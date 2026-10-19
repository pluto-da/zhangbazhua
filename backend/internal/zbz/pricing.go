// 章八爪模块 · 模型比价雷达后端 · 作者 长鱼茶汤茗
package zbz

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"infinite-canvas/backend/internal/outbound"

	"github.com/gin-gonic/gin"
)

// 价格数据由仓库 pluto-da/zhangbazhua 的 zbz-price-radar 工作流每天生成，发布在 data 分支。
var priceSources = []string{
	"https://cdn.jsdelivr.net/gh/pluto-da/zhangbazhua@data/prices.json",
	"https://raw.githubusercontent.com/pluto-da/zhangbazhua/data/prices.json",
	"https://fastly.jsdelivr.net/gh/pluto-da/zhangbazhua@data/prices.json",
}

const modelsDevURL = "https://models.dev/api.json"

type pricing struct {
	mu      sync.Mutex
	cache   string
	data    json.RawMessage
	fetched time.Time
	source  string
}

func newPricing(dir string) *pricing {
	p := &pricing{cache: filepath.Join(dir, "prices.json")}
	if b, err := os.ReadFile(p.cache); err == nil && json.Valid(b) {
		p.data = b
		if st, err := os.Stat(p.cache); err == nil {
			p.fetched = st.ModTime()
		}
		p.source = "cache"
	}
	return p
}

func (p *pricing) handle(c *gin.Context) {
	force := c.Query("refresh") == "1"
	p.mu.Lock()
	defer p.mu.Unlock()
	if !force && p.data != nil && time.Since(p.fetched) < 6*time.Hour && p.source != "cache" {
		p.reply(c, false, "")
		return
	}
	data, src, err := fetchPrices()
	if err == nil {
		p.data, p.fetched, p.source = data, time.Now(), src
		_ = os.WriteFile(p.cache, data, 0o644)
		p.reply(c, false, "")
		return
	}
	if p.data != nil {
		p.reply(c, true, "价格数据暂时拉取失败，显示的是上次保存的数据")
		return
	}
	fail(c, http.StatusBadGateway, "价格数据拉取失败，请检查网络（需要能访问 jsDelivr 或 GitHub）后重试")
}

func (p *pricing) reply(c *gin.Context, stale bool, note string) {
	ok(c, gin.H{"prices": p.data, "source": p.source, "fetchedAt": p.fetched.Format(time.RFC3339), "stale": stale, "note": note})
}

func get(url string, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	outbound.ApplyDefaultOutboundHeaders(req)
	resp, err := outbound.OutboundHTTPClient(timeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func fetchPrices() (json.RawMessage, string, error) {
	for _, u := range priceSources {
		b, err := get(u, 20*time.Second)
		if err == nil && json.Valid(b) && strings.Contains(string(b[:min(len(b), 4096)]), "generatedAt") {
			return b, u, nil
		}
	}
	// 降级：直接读 models.dev（只有这一个来源，没有汇总）。
	b, err := get(modelsDevURL, 40*time.Second)
	if err != nil {
		return nil, "", err
	}
	out, err := fromModelsDev(b)
	if err != nil {
		return nil, "", err
	}
	return out, modelsDevURL, nil
}

type offer struct {
	Provider     string   `json:"provider"`
	ProviderName string   `json:"providerName"`
	Source       string   `json:"source"`
	Tier         string   `json:"tier"`
	Input        float64  `json:"inputUsd"`
	Output       float64  `json:"outputUsd"`
	CacheRead    *float64 `json:"cacheReadUsd,omitempty"`
	URL          string   `json:"url"`
	China        bool     `json:"china"`
}

type modelRow struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Modalities []string `json:"modalities"`
	Context    int      `json:"context"`
	Offers     []offer  `json:"offers"`
}

// fromModelsDev 把 models.dev 的原始结构压成和 prices.json 相同的形状（降级用，汇率按 7.1）。
func fromModelsDev(b []byte) (json.RawMessage, error) {
	var raw map[string]struct {
		Name   string `json:"name"`
		Doc    string `json:"doc"`
		API    string `json:"api"`
		Models map[string]struct {
			Name string `json:"name"`
			Cost *struct {
				Input     float64  `json:"input"`
				Output    float64  `json:"output"`
				CacheRead *float64 `json:"cache_read"`
			} `json:"cost"`
			Limit struct {
				Context int `json:"context"`
			} `json:"limit"`
			Modalities struct {
				Input  []string `json:"input"`
				Output []string `json:"output"`
			} `json:"modalities"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	rows := map[string]*modelRow{}
	for pid, prov := range raw {
		for mid, m := range prov.Models {
			if m.Cost == nil || m.Cost.Input < 0 || m.Cost.Output < 0 || (m.Cost.Input == 0 && m.Cost.Output == 0) {
				continue
			}
			key := canonical(mid)
			r := rows[key]
			if r == nil {
				r = &modelRow{ID: key, Name: m.Name, Context: m.Limit.Context, Modalities: m.Modalities.Output}
				rows[key] = r
			}
			r.Offers = append(r.Offers, offer{Provider: pid, ProviderName: prov.Name, Source: "models.dev", Tier: "汇总", Input: m.Cost.Input, Output: m.Cost.Output, CacheRead: m.Cost.CacheRead, URL: prov.Doc, China: strings.HasSuffix(pid, "-cn") || strings.Contains(prov.API, ".cn")})
		}
	}
	list := make([]*modelRow, 0, len(rows))
	for _, r := range rows {
		sort.Slice(r.Offers, func(i, j int) bool { return blend(r.Offers[i]) < blend(r.Offers[j]) })
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return json.Marshal(gin.H{"generatedAt": time.Now().UTC().Format(time.RFC3339), "usdCny": 7.1, "fx": "fallback", "sources": []gin.H{{"name": "models.dev", "url": modelsDevURL, "ok": true}}, "models": list, "degraded": true})
}

func blend(o offer) float64 { return (3*o.Input + o.Output) / 4 }

var (
	reSuffix  = regexp.MustCompile(`:(free|beta|extended|thinking|nitro|floor)$`)
	reDate    = regexp.MustCompile(`[-_@](20\d{2}-?\d{2}-?\d{2}|\d{6}|\d{4})$`)
	reDotNum  = regexp.MustCompile(`(\d)\.(\d)`)
	reRegion  = regexp.MustCompile(`^(us|eu|apac|global)\.`)
	reVendor  = regexp.MustCompile(`^(anthropic|meta|amazon|mistral|cohere|ai21)\.`)
	reVersion = regexp.MustCompile(`-v\d+:\d+$`)
)

// canonical 与 scripts/zbz/price_radar.py 的 canonical 保持同一套规则。
func canonical(id string) string {
	s := strings.ToLower(strings.TrimSpace(id))
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = reSuffix.ReplaceAllString(s, "")
	s = reDate.ReplaceAllString(s, "")
	s = reRegion.ReplaceAllString(s, "")
	s = reVendor.ReplaceAllString(s, "")
	s = reVersion.ReplaceAllString(s, "")
	// 4.5 → 4-5，与官方命名对齐（重复一次处理 1.2.3 这类连续情况）
	s = reDotNum.ReplaceAllString(s, "$1-$2")
	s = reDotNum.ReplaceAllString(s, "$1-$2")
	return s
}
