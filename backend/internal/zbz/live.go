// 章八爪模块 · 实时最低价后端 · 作者 长鱼茶汤茗
// 实时 = 打开即拉取 OpenRouter（10 分钟缓存）与 models.dev（30 分钟缓存）+ 按此刻套用分时价 + 对比本地每日快照。
package zbz

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const openRouterURL = "https://openrouter.ai/api/v1/models"

var (
	officialRe = regexp.MustCompile(`^(openai|anthropic|google|google-vertex|google-vertex-anthropic|vertex_ai.*|gemini|azure|azure_ai|amazon-bedrock|bedrock.*|deepseek|alibaba|alibaba-cn|dashscope|zhipuai|zai|moonshotai|moonshotai-cn|moonshot|xiaomi|volcengine|tencent.*|minimax|minimax-cn|mistral|xai|cohere|baidu.*|stepfun.*|sensenova|oci|databricks|nvidia|cerebras|groq|siliconflow|siliconflow-cn|openrouter)$`)
	planRe     = regexp.MustCompile(`(?i)coding-plan|token-plan|step-plan|subscription`)
	chinaRe    = regexp.MustCompile(`(?i)(-cn$|^deepseek$|^zhipuai$|^moonshotai-cn$|^siliconflow-cn$|^alibaba-cn$|^volcengine$|^xiaomi$|^tencent|^minimax-cn$|^baidu|^stepfun|^sensenova|^moark|^modelscope)`)
	modelParam = regexp.MustCompile(`^[a-z0-9._:\-/]{1,120}$`)
	beijing    = time.FixedZone("CST", 8*3600)
)

func tierOf(provider string) string {
	if officialRe.MatchString(strings.ToLower(provider)) {
		return "官方/云厂商"
	}
	return "第三方/聚合"
}

// priceRule 是 OpenRouter pricing.overrides 里的一条时段规则。
type priceRule struct {
	Days      []string
	Start     int // HHMM
	End       int // HHMM，0 视为 2400
	Input     float64
	Output    float64
	TokenTier bool // min_prompt_tokens 阶梯价，不属于时段价
}

func (r priceRule) active(t time.Time) bool {
	if r.TokenTier || len(r.Days) == 0 {
		return false
	}
	day := strings.ToLower(t.UTC().Weekday().String())
	hit := false
	for _, d := range r.Days {
		if strings.ToLower(d) == day {
			hit = true
			break
		}
	}
	if !hit {
		return false
	}
	hhmm := t.UTC().Hour()*100 + t.UTC().Minute()
	end := r.End
	if end == 0 {
		end = 2400
	}
	return hhmm >= r.Start && hhmm < end
}

// effectivePrice 返回 t 时刻的生效价（每百万 tokens 美元）。
func effectivePrice(baseIn, baseOut float64, rules []priceRule, t time.Time) (float64, float64) {
	for _, r := range rules {
		if r.active(t) {
			return r.Input, r.Output
		}
	}
	return baseIn, baseOut
}

// nextChange 从 t 起按 15 分钟步长向后扫描 7 天，返回第一次生效价改变的时刻与变价后综合价。
func nextChange(baseIn, baseOut float64, rules []priceRule, t time.Time) (time.Time, float64, bool) {
	if len(rules) == 0 {
		return time.Time{}, 0, false
	}
	ci, co := effectivePrice(baseIn, baseOut, rules, t)
	cur := blendUsd(ci, co)
	start := t.UTC().Truncate(15 * time.Minute)
	for step := 1; step <= 7*24*4; step++ {
		at := start.Add(time.Duration(step) * 15 * time.Minute)
		i, o := effectivePrice(baseIn, baseOut, rules, at)
		if b := blendUsd(i, o); math.Abs(b-cur) > 1e-12 {
			return at, b, true
		}
	}
	return time.Time{}, 0, false
}

func blendUsd(in, out float64) float64 { return (3*in + out) / 4 }

type liveOffer struct {
	Provider      string   `json:"provider"`
	ProviderName  string   `json:"providerName"`
	Tier          string   `json:"tier"`
	Source        string   `json:"source"`
	URL           string   `json:"url"`
	China         bool     `json:"china"`
	RawModel      string   `json:"rawModel,omitempty"`
	InputUsd      float64  `json:"inputUsd"`
	OutputUsd     float64  `json:"outputUsd"`
	BaseInputUsd  float64  `json:"baseInputUsd"`
	BaseOutputUsd float64  `json:"baseOutputUsd"`
	Unit          string   `json:"unit"`
	UnitUsd       float64  `json:"unitUsd,omitempty"`
	Res           string   `json:"res,omitempty"`
	AssumedSecs   int      `json:"assumedSeconds,omitempty"`
	CnyReal       float64  `json:"cnyReal,omitempty"`
	Group         string   `json:"group,omitempty"`
	GroupRatio    float64  `json:"groupRatio,omitempty"`
	MinGroup      string   `json:"minGroup,omitempty"`
	MinGroupRatio float64  `json:"minGroupRatio,omitempty"`
	RechargePrice float64  `json:"rechargePrice,omitempty"`
	RechargeGuess bool     `json:"rechargeAssumed,omitempty"`
	Site          string   `json:"site,omitempty"`
	Timed         bool     `json:"timed"`
	NextChangeAt  string   `json:"nextChangeAt,omitempty"`
	NextBlendUsd  *float64 `json:"nextBlendUsd,omitempty"`
	rules         []priceRule
}

type liveModel struct {
	ID                string      `json:"id"`
	Best              *liveOffer  `json:"best"`
	Offers            []liveOffer `json:"offers"`
	OfferCount        int         `json:"offerCount"`
	BaseBestBlendUsd  float64     `json:"baseBestBlendUsd"`
	Unit              string      `json:"unit"`
	Modalities        []string    `json:"modalities"`
	YesterdayBlendUsd *float64    `json:"yesterdayBlendUsd"`
	ChangePct         *float64    `json:"changePct"`
	IsNewLow          bool        `json:"isNewLow"`
}

type sourceState struct {
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	FetchedAt string `json:"fetchedAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

type cachedSource struct {
	offers  map[string][]liveOffer // canonical -> offers
	fetched time.Time
	err     string
}

type live struct {
	mu         sync.Mutex
	pricing    *pricing
	or         cachedSource
	md         cachedSource
	hist       string
	relays     *myRelays
	modalities map[string][]string
}

func newLive(dir string, p *pricing) *live {
	return &live{pricing: p, hist: filepath.Join(dir, "price-history.json"), relays: newMyRelays(dir), modalities: map[string][]string{}}
}

func (l *live) refresh(src *cachedSource, ttl time.Duration, fetch func() (map[string][]liveOffer, error)) {
	if src.offers != nil && time.Since(src.fetched) < ttl {
		return
	}
	offers, err := fetch()
	if err != nil {
		src.err = err.Error()
		return
	}
	src.offers, src.fetched, src.err = offers, time.Now(), ""
}

func (s cachedSource) state(name string) sourceState {
	st := sourceState{Name: name, OK: s.offers != nil && s.err == "", Error: s.err}
	if !s.fetched.IsZero() {
		st.FetchedAt = s.fetched.UTC().Format(time.RFC3339)
	}
	return st
}

func parseUsdPerToken(v any) (float64, bool) {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case float64:
		return t * 1e6, true
	default:
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f * 1e6, true
}

func parseOpenRouter(b []byte) (map[string][]liveOffer, error) {
	var raw struct {
		Data []struct {
			ID      string         `json:"id"`
			Pricing map[string]any `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := map[string][]liveOffer{}
	for _, m := range raw.Data {
		in, ok1 := parseUsdPerToken(m.Pricing["prompt"])
		outp, ok2 := parseUsdPerToken(m.Pricing["completion"])
		if !ok1 || !ok2 || in < 0 || outp < 0 {
			continue
		}
		o := liveOffer{Provider: "openrouter", ProviderName: "OpenRouter", Tier: tierOf("openrouter"), Source: "OpenRouter 实时", URL: "https://openrouter.ai/" + m.ID, RawModel: m.ID, BaseInputUsd: in, BaseOutputUsd: outp, Unit: "token"}
		if list, ok := m.Pricing["overrides"].([]any); ok {
			for _, item := range list {
				r, ok := item.(map[string]any)
				if !ok {
					continue
				}
				rule := priceRule{}
				if _, has := r["min_prompt_tokens"]; has {
					rule.TokenTier = true
				}
				if days, ok := r["utc_days"].([]any); ok {
					for _, d := range days {
						if ds, ok := d.(string); ok {
							rule.Days = append(rule.Days, ds)
						}
					}
				}
				if v, ok := r["utc_start"].(float64); ok {
					rule.Start = int(v)
				}
				if v, ok := r["utc_end"].(float64); ok {
					rule.End = int(v)
				}
				ri, ok1 := parseUsdPerToken(r["prompt"])
				ro, ok2 := parseUsdPerToken(r["completion"])
				if !ok1 || !ok2 || rule.TokenTier || len(rule.Days) == 0 {
					continue
				}
				rule.Input, rule.Output = ri, ro
				o.rules = append(o.rules, rule)
			}
		}
		o.Timed = len(o.rules) > 0
		key := canonical(m.ID)
		// 同一标准名在 OpenRouter 有多个变体（如 :free）时保留常规价最低的付费变体
		if prev, ok := firstByProvider(out[key], "openrouter"); ok && blendUsd(prev.BaseInputUsd, prev.BaseOutputUsd) > 0 && blendUsd(prev.BaseInputUsd, prev.BaseOutputUsd) <= blendUsd(in, outp) {
			continue
		}
		out[key] = replaceProvider(out[key], o)
	}
	return out, nil
}

func parseModelsDevLive(b []byte) (map[string][]liveOffer, error) {
	var raw map[string]struct {
		Name   string `json:"name"`
		Doc    string `json:"doc"`
		API    string `json:"api"`
		Models map[string]struct {
			Cost *struct {
				Input  float64 `json:"input"`
				Output float64 `json:"output"`
			} `json:"cost"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := map[string][]liveOffer{}
	for pid, prov := range raw {
		if planRe.MatchString(pid) {
			continue
		}
		for mid, m := range prov.Models {
			if m.Cost == nil {
				continue
			}
			url := prov.Doc
			if url == "" {
				url = "https://models.dev"
			}
			o := liveOffer{Provider: strings.ToLower(pid), ProviderName: prov.Name, Tier: tierOf(pid), Source: "models.dev 实时", URL: url, RawModel: mid, China: chinaRe.MatchString(pid) || strings.Contains(prov.API, ".cn"), BaseInputUsd: m.Cost.Input, BaseOutputUsd: m.Cost.Output, Unit: "token"}
			key := canonical(mid)
			if _, ok := firstByProvider(out[key], o.Provider); ok {
				continue
			}
			out[key] = append(out[key], o)
		}
	}
	return out, nil
}

func firstByProvider(list []liveOffer, provider string) (liveOffer, bool) {
	for _, o := range list {
		if o.Provider == provider && (o.Unit == "" || o.Unit == "token") {
			return o, true
		}
	}
	return liveOffer{}, false
}

func replaceProvider(list []liveOffer, o liveOffer) []liveOffer {
	for i := range list {
		if list[i].Provider == o.Provider && unitOf(list[i]) == unitOf(o) {
			list[i] = o
			return list
		}
	}
	return append(list, o)
}

// baseline 从日报 prices.json 取出关注模型的报价。
func (l *live) baseline(wanted map[string]bool) (map[string][]liveOffer, float64, bool) {
	l.pricing.mu.Lock()
	data := l.pricing.data
	l.pricing.mu.Unlock()
	if data == nil {
		return nil, 0, false
	}
	var f struct {
		UsdCny float64 `json:"usdCny"`
		Models []struct {
			ID         string   `json:"id"`
			Modalities []string `json:"modalities"`
			Offers     []struct {
				liveOffer
				InputUsd  float64 `json:"inputUsd"`
				OutputUsd float64 `json:"outputUsd"`
				RawModel  string  `json:"rawModel"`
			} `json:"offers"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, 0, false
	}
	out := map[string][]liveOffer{}
	for _, m := range f.Models {
		id := canonical(m.ID)
		if !wanted[id] {
			continue
		}
		l.modalities[id] = m.Modalities
		for _, o := range m.Offers {
			lo := o.liveOffer
			lo.Provider = strings.ToLower(lo.Provider)
			if lo.Tier == "" {
				lo.Tier = tierOf(lo.Provider)
			}
			lo.Source += " 日报"
			lo.BaseInputUsd, lo.BaseOutputUsd, lo.RawModel = o.InputUsd, o.OutputUsd, o.RawModel
			if lo.Unit == "" {
				lo.Unit = "token"
			}
			out[id] = append(out[id], lo)
		}
	}
	return out, f.UsdCny, true
}

func parseWanted(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		s := strings.ToLower(strings.TrimSpace(part))
		if s == "" || !modelParam.MatchString(s) {
			continue
		}
		s = canonical(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) >= 40 {
			break
		}
	}
	return out
}

func (l *live) handle(c *gin.Context) {
	wantedList := parseWanted(c.Query("models"))
	officialOnly := c.Query("official") == "1"
	wanted := map[string]bool{}
	for _, w := range wantedList {
		wanted[w] = true
	}
	// 确保日报缓存已加载（复用比价雷达的缓存与拉取逻辑）
	l.pricing.mu.Lock()
	needDaily := l.pricing.data == nil || time.Since(l.pricing.fetched) > 12*time.Hour
	l.pricing.mu.Unlock()
	if needDaily {
		if data, src, err := fetchPrices(); err == nil {
			l.pricing.mu.Lock()
			l.pricing.data, l.pricing.fetched, l.pricing.source = data, time.Now(), src
			_ = os.WriteFile(l.pricing.cache, data, 0o644)
			l.pricing.mu.Unlock()
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.refresh(&l.or, 10*time.Minute, func() (map[string][]liveOffer, error) {
		b, err := get(openRouterURL, 20*time.Second)
		if err != nil {
			return nil, err
		}
		return parseOpenRouter(b)
	})
	l.refresh(&l.md, 30*time.Minute, func() (map[string][]liveOffer, error) {
		b, err := get(modelsDevURL, 40*time.Second)
		if err != nil {
			return nil, err
		}
		return parseModelsDevLive(b)
	})
	base, usdCny, haveDaily := l.baseline(wanted)
	if usdCny <= 0 {
		usdCny = 7.1
	}
	liveOK := l.or.offers != nil || l.md.offers != nil
	mode := "live"
	if !liveOK {
		l.pricing.mu.Lock()
		src := l.pricing.source
		l.pricing.mu.Unlock()
		switch {
		case haveDaily && src != "cache":
			mode = "daily"
		case haveDaily:
			mode = "cache"
		default:
			fail(c, http.StatusBadGateway, "实时价格和日报都拉取失败，请检查网络（需要能访问 OpenRouter、models.dev 或 jsDelivr）后重试")
			return
		}
	}

	myOffers, myStates := l.relays.offers(usdCny)
	now := time.Now()
	models := make([]liveModel, 0, len(wantedList))
	for _, id := range wantedList {
		merged := append([]liveOffer(nil), base[id]...)
		for _, o := range l.md.offers[id] {
			merged = replaceProvider(merged, o)
		}
		for _, o := range l.or.offers[id] {
			merged = replaceProvider(merged, o)
		}
		for _, o := range myOffers[id] {
			merged = replaceProvider(merged, o)
		}
		lm := buildModel(id, merged, now, officialOnly)
		lm.Modalities = l.modalities[id]
		if lm.Modalities == nil {
			lm.Modalities = []string{}
		}
		models = append(models, lm)
	}
	l.applyHistory(models, now)
	daily := sourceState{Name: "日报 prices.json", OK: haveDaily}
	l.pricing.mu.Lock()
	if !l.pricing.fetched.IsZero() {
		daily.FetchedAt = l.pricing.fetched.UTC().Format(time.RFC3339)
	}
	l.pricing.mu.Unlock()
	ok(c, gin.H{
		"generatedAt": now.UTC().Format(time.RFC3339),
		"mode":        mode,
		"usdCny":      usdCny,
		"sources":     append([]sourceState{l.or.state("OpenRouter 实时"), l.md.state("models.dev 实时"), daily}, myStates...),
		"models":      models,
	})
}

func unitOf(o liveOffer) string {
	if o.Unit == "" {
		return "token"
	}
	return o.Unit
}

// priceNow 是同一计费单位内可比较的此刻单价（token 为 3:1 综合价，其他为 unitUsd）。
func priceNow(o liveOffer) float64 {
	if unitOf(o) == "token" {
		return blendUsd(o.InputUsd, o.OutputUsd)
	}
	return o.UnitUsd
}

func priceBase(o liveOffer) float64 {
	if unitOf(o) == "token" {
		return blendUsd(o.BaseInputUsd, o.BaseOutputUsd)
	}
	return o.UnitUsd
}

var unitRank = map[string]int{"second": 0, "token": 1, "image": 2, "call": 3}

func buildModel(id string, offers []liveOffer, now time.Time, officialOnly bool) liveModel {
	kept := make([]liveOffer, 0, len(offers))
	for _, o := range offers {
		u := unitOf(o)
		if u == "token" {
			if o.BaseInputUsd < 0 || o.BaseOutputUsd < 0 || (o.BaseInputUsd == 0 && o.BaseOutputUsd == 0) {
				continue // 零价多为未标价或限时免费，未确认，不参与
			}
		} else if o.UnitUsd <= 0 {
			continue
		}
		if officialOnly && o.Tier != "官方/云厂商" {
			continue
		}
		o.Unit = u
		o.InputUsd, o.OutputUsd = effectivePrice(o.BaseInputUsd, o.BaseOutputUsd, o.rules, now)
		if u == "token" {
			if at, b, okc := nextChange(o.BaseInputUsd, o.BaseOutputUsd, o.rules, now); okc {
				o.NextChangeAt = at.UTC().Format(time.RFC3339)
				bb := b
				o.NextBlendUsd = &bb
			}
		}
		kept = append(kept, o)
	}
	// 同一单位内，偏离中位数 100 倍的异常价剔除
	byUnit := map[string][]float64{}
	for _, o := range kept {
		byUnit[o.Unit] = append(byUnit[o.Unit], priceBase(o))
	}
	med := map[string]float64{}
	for u, v := range byUnit {
		if len(v) >= 3 {
			sorted := append([]float64(nil), v...)
			sort.Float64s(sorted)
			med[u] = sorted[len(sorted)/2]
		}
	}
	filtered := kept[:0]
	for _, o := range kept {
		if m := med[o.Unit]; m > 0 && (priceBase(o) > m*100 || priceBase(o) < m/100) {
			continue
		}
		filtered = append(filtered, o)
	}
	kept = filtered
	m := liveModel{ID: id, OfferCount: len(kept), Offers: []liveOffer{}, Modalities: []string{}}
	if len(kept) == 0 {
		return m
	}
	primary := ""
	for _, o := range kept {
		if primary == "" || unitRank[o.Unit] < unitRank[primary] {
			primary = o.Unit
		}
	}
	m.Unit = primary
	sort.SliceStable(kept, func(i, j int) bool {
		pi, pj := kept[i].Unit == primary, kept[j].Unit == primary
		if pi != pj {
			return pi
		}
		if kept[i].Unit != kept[j].Unit {
			return unitRank[kept[i].Unit] < unitRank[kept[j].Unit]
		}
		return priceNow(kept[i]) < priceNow(kept[j])
	})
	best := kept[0]
	m.Best = &best
	baseMin := math.Inf(1)
	for _, o := range kept {
		if o.Unit == primary {
			baseMin = math.Min(baseMin, priceBase(o))
		}
	}
	m.BaseBestBlendUsd = baseMin
	if len(kept) > 6 {
		kept = kept[:6]
	}
	m.Offers = kept
	return m
}

// applyHistory 写入今天（北京时间）的快照，并算出相对昨天的涨跌与 30 天新低。快照比较的是常规价最低值，避免分时价造成假涨跌。
func (l *live) applyHistory(models []liveModel, now time.Time) {
	hist := map[string]map[string]float64{}
	if b, err := os.ReadFile(l.hist); err == nil {
		_ = json.Unmarshal(b, &hist)
	}
	today := now.In(beijing).Format("2006-01-02")
	if hist[today] == nil {
		hist[today] = map[string]float64{}
	}
	dates := make([]string, 0, len(hist))
	for d := range hist {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	changed := false
	for i := range models {
		m := &models[i]
		if m.Best == nil || m.BaseBestBlendUsd <= 0 {
			continue
		}
		if _, ok := hist[today][m.ID]; !ok {
			hist[today][m.ID] = m.BaseBestBlendUsd
			changed = true
		} else if m.BaseBestBlendUsd < hist[today][m.ID] {
			hist[today][m.ID] = m.BaseBestBlendUsd
			changed = true
		}
		prevMin := math.Inf(1)
		for j := len(dates) - 1; j >= 0; j-- {
			d := dates[j]
			if d >= today {
				continue
			}
			v, ok := hist[d][m.ID]
			if !ok {
				continue
			}
			if m.YesterdayBlendUsd == nil {
				y := v
				m.YesterdayBlendUsd = &y
				pct := (m.BaseBestBlendUsd - v) / v * 100
				pct = math.Round(pct*10) / 10
				m.ChangePct = &pct
			}
			prevMin = math.Min(prevMin, v)
		}
		m.IsNewLow = !math.IsInf(prevMin, 1) && m.BaseBestBlendUsd < prevMin-1e-12
	}
	// 只保留 30 天
	cutoff := now.In(beijing).AddDate(0, 0, -30).Format("2006-01-02")
	for d := range hist {
		if d < cutoff {
			delete(hist, d)
			changed = true
		}
	}
	if changed {
		if b, err := json.Marshal(hist); err == nil {
			tmp := l.hist + ".tmp"
			if os.WriteFile(tmp, b, 0o644) == nil {
				_ = os.Rename(tmp, l.hist)
			}
		}
	}
}
