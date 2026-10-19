// 章八爪模块 · 我的中转站（New API / One API）· 作者 长鱼茶汤茗
// 只保存站名和地址，不保存任何密钥。价格 = 额度价 × 分组倍率 × 充值价。
package zbz

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	videoKindRe = regexp.MustCompile(`(?i)veo|sora|kling|seedance|hailuo|vidu|runway|luma|pika|happyhorse|video|t2v|i2v|r2v|s2v|jimeng`)
	imageKindRe = regexp.MustCompile(`(?i)image|flux|midjourney|^mj[-_]|seedream|imagen|dall-?e|kolors|recraft|ideogram|sdxl|stable-diffusion`)
	notGenRe    = regexp.MustCompile(`(?i)understand|vision|embed|caption|\bvl\b`)
	durRe       = regexp.MustCompile(`(?i)(\d{1,2})\s*(s|秒|sec)\b`)
	hostRe      = regexp.MustCompile(`^[a-z0-9.-]{1,253}$`)
)

func kindOf(name string) string {
	if notGenRe.MatchString(name) {
		return "text"
	}
	if videoKindRe.MatchString(name) {
		return "video"
	}
	if imageKindRe.MatchString(name) {
		return "image"
	}
	return "text"
}

type relaySite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Base string `json:"base"`
}

type newAPIStatus struct {
	Data struct {
		SystemName string `json:"system_name"`
		Price      any    `json:"price"`
	} `json:"data"`
}

type newAPIPricing struct {
	Data []struct {
		ModelName       string   `json:"model_name"`
		Description     string   `json:"description"`
		QuotaType       any      `json:"quota_type"`
		ModelRatio      float64  `json:"model_ratio"`
		ModelPrice      float64  `json:"model_price"`
		CompletionRatio float64  `json:"completion_ratio"`
		EnableGroups    []string `json:"enable_groups"`
		GroupPrice      any      `json:"group_price"`
	} `json:"data"`
	GroupRatio map[string]float64 `json:"group_ratio"`
}

func anyFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

func pickGroup(enable []string, ratios map[string]float64) (string, float64, string, float64) {
	type gr struct {
		g string
		r float64
	}
	var c []gr
	for _, g := range enable {
		if r, ok := ratios[g]; ok {
			c = append(c, gr{g, r})
		}
	}
	if len(c) == 0 {
		r := 1.0
		if v, ok := ratios["default"]; ok && v > 0 {
			r = v
		}
		return "default", r, "default", r
	}
	low := c[0]
	for _, x := range c {
		if x.r < low.r {
			low = x
		}
	}
	for _, x := range c {
		if x.g == "default" {
			return x.g, x.r, low.g, low.r
		}
	}
	return low.g, low.r, low.g, low.r
}

// newAPIOffers 与 scripts/zbz/price_radar.py 的 newapi_offers 口径一致。返回 canonical → offers。
func newAPIOffers(site relaySite, st newAPIStatus, pr newAPIPricing, usdCny float64) (map[string][]liveOffer, error) {
	if len(pr.Data) == 0 {
		return nil, errors.New("该站没有公开价格表")
	}
	if pr.Data[0].GroupPrice != nil && pr.Data[0].ModelRatio == 0 && pr.Data[0].ModelPrice == 0 {
		return nil, errors.New("该站使用新版分组价格格式，口径未核实，暂不支持")
	}
	price := anyFloat(st.Data.Price)
	assumed := price < 0.1 || price > 10
	if assumed {
		price = 1
	}
	out := map[string][]liveOffer{}
	for _, r := range pr.Data {
		if r.ModelName == "" {
			continue
		}
		g, gr, lg, lgr := pickGroup(r.EnableGroups, pr.GroupRatio)
		kind := kindOf(r.ModelName)
		o := liveOffer{Provider: "relay:" + site.ID, ProviderName: site.Name, Tier: "中转站", Source: "我的中转站 实时", URL: site.Base, China: true, RawModel: r.ModelName,
			Group: g, GroupRatio: gr, MinGroup: lg, MinGroupRatio: lgr, RechargePrice: price, RechargeGuess: assumed, Site: site.Base}
		if int(anyFloat(r.QuotaType)) == 0 {
			if r.ModelRatio <= 0 || kind != "text" {
				continue
			}
			cr := r.CompletionRatio
			if cr <= 0 {
				cr = 1
			}
			cnyIn := r.ModelRatio * 2 * gr * price
			o.Unit, o.CnyReal = "token", cnyIn
			o.BaseInputUsd, o.BaseOutputUsd = cnyIn/usdCny, cnyIn*cr/usdCny
		} else {
			if r.ModelPrice <= 0 {
				continue
			}
			cny := r.ModelPrice * gr * price
			switch kind {
			case "video":
				m := durRe.FindStringSubmatch(r.ModelName)
				if m == nil {
					m = durRe.FindStringSubmatch(r.Description)
				}
				secs := 0
				if m != nil {
					secs, _ = strconv.Atoi(m[1])
				}
				if secs >= 1 && secs <= 60 {
					o.Unit, o.AssumedSecs, o.CnyReal, o.UnitUsd = "second", secs, cny/float64(secs), cny/float64(secs)/usdCny
				} else {
					o.Unit, o.CnyReal, o.UnitUsd = "call", cny, cny/usdCny
				}
			case "image":
				o.Unit, o.CnyReal, o.UnitUsd = "image", cny, cny/usdCny
			default:
				o.Unit, o.CnyReal, o.UnitUsd = "call", cny, cny/usdCny
			}
		}
		key := canonical(r.ModelName)
		if prev, ok := firstUnit(out[key], o.Unit); ok && priceBase(prev) <= priceBase(o) {
			continue
		}
		out[key] = replaceProvider(out[key], o)
	}
	return out, nil
}

func firstUnit(list []liveOffer, unit string) (liveOffer, bool) {
	for _, o := range list {
		if unitOf(o) == unit {
			return o, true
		}
	}
	return liveOffer{}, false
}

// normalizeBase 校验并规范站点地址：只允许 https、禁止内网与本机地址（防 SSRF）。
func normalizeBase(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return "", errors.New("只支持 https 地址")
	}
	host := strings.ToLower(u.Hostname())
	if !hostRe.MatchString(host) || !strings.Contains(host, ".") || host == "localhost" {
		return "", errors.New("地址不合法")
	}
	if ip := net.ParseIP(host); ip != nil {
		return "", errors.New("请填写域名，不支持直接填 IP")
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", errors.New("域名解析失败")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsLinkLocalMulticast() {
			return "", errors.New("不支持内网或本机地址")
		}
	}
	return "https://" + host, nil
}

type relayCache struct {
	offers  map[string][]liveOffer
	fetched time.Time
	err     string
	name    string
}

type myRelays struct {
	mu    sync.Mutex
	file  string
	cache map[string]*relayCache
}

func newMyRelays(dir string) *myRelays {
	return &myRelays{file: filepath.Join(dir, "my-relays.json"), cache: map[string]*relayCache{}}
}

func (m *myRelays) load() []relaySite {
	var list []relaySite
	if b, err := os.ReadFile(m.file); err == nil {
		_ = json.Unmarshal(b, &list)
	}
	return list
}

func (m *myRelays) save(list []relaySite) error {
	b, _ := json.MarshalIndent(list, "", "  ")
	tmp := m.file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.file)
}

func fetchRelay(site relaySite, usdCny float64) (string, map[string][]liveOffer, error) {
	var st newAPIStatus
	b, err := get(site.Base+"/api/status", 15*time.Second)
	if err != nil {
		return "", nil, fmt.Errorf("连不上该站：%v", err)
	}
	if json.Unmarshal(b, &st) != nil {
		return "", nil, errors.New("该站不是 New API / One API 系统，暂不支持")
	}
	b, err = get(site.Base+"/api/pricing", 30*time.Second)
	if err != nil {
		return st.Data.SystemName, nil, errors.New("该站关闭了公开价格，无法比价")
	}
	var pr newAPIPricing
	if json.Unmarshal(b, &pr) != nil || len(pr.Data) == 0 {
		return st.Data.SystemName, nil, errors.New("该站关闭了公开价格，无法比价")
	}
	offers, err := newAPIOffers(site, st, pr, usdCny)
	return st.Data.SystemName, offers, err
}

// offers 返回我的中转站的报价（每站缓存 30 分钟）与各站状态。
func (m *myRelays) offers(usdCny float64) (map[string][]liveOffer, []sourceState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string][]liveOffer{}
	var states []sourceState
	for _, s := range m.load() {
		c := m.cache[s.ID]
		if c == nil || time.Since(c.fetched) > 30*time.Minute {
			_, offers, err := fetchRelay(s, usdCny)
			c = &relayCache{offers: offers, fetched: time.Now(), name: s.Name}
			if err != nil {
				c.err = err.Error()
			}
			m.cache[s.ID] = c
		}
		states = append(states, sourceState{Name: "我的中转站 " + s.Name, OK: c.err == "", FetchedAt: c.fetched.UTC().Format(time.RFC3339), Error: c.err})
		for k, v := range c.offers {
			out[k] = append(out[k], v...)
		}
	}
	return out, states
}

func (m *myRelays) list(c *gin.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.load()
	if list == nil {
		list = []relaySite{}
	}
	ok(c, list)
}

func (m *myRelays) check(c *gin.Context) {
	base, err := normalizeBase(c.Query("base"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	name, offers, err := fetchRelay(relaySite{ID: "check", Name: "检查", Base: base}, 7.1)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	n := 0
	for _, v := range offers {
		n += len(v)
	}
	var st newAPIStatus
	if b, e := get(base+"/api/status", 10*time.Second); e == nil {
		_ = json.Unmarshal(b, &st)
	}
	ok(c, gin.H{"base": base, "name": name, "offers": n, "rechargePrice": anyFloat(st.Data.Price)})
}

func (m *myRelays) add(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Base string `json:"base"`
	}
	if c.ShouldBindJSON(&req) != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	base, err := normalizeBase(req.Base)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimPrefix(base, "https://")
	}
	if len([]rune(name)) > 40 {
		name = string([]rune(name)[:40])
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.load()
	for _, s := range list {
		if s.Base == base {
			fail(c, http.StatusBadRequest, "这个站已经添加过了")
			return
		}
	}
	if len(list) >= 20 {
		fail(c, http.StatusBadRequest, "最多添加 20 个中转站")
		return
	}
	site := relaySite{ID: strconv.FormatInt(time.Now().UnixNano(), 36), Name: name, Base: base}
	list = append(list, site)
	if err := m.save(list); err != nil {
		fail(c, http.StatusInternalServerError, "保存失败")
		return
	}
	ok(c, site)
}

func (m *myRelays) remove(c *gin.Context) {
	id := c.Param("id")
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.load()
	out := list[:0]
	for _, s := range list {
		if s.ID != id {
			out = append(out, s)
		}
	}
	_ = m.save(out)
	delete(m.cache, id)
	ok(c, gin.H{"removed": id})
}
