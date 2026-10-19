// 章八爪模块 · 充值中心后端（余额查询）· 作者 长鱼茶汤茗
// 只在本机查询；Key 由本机前端交给本机后端，直接发给对应平台，不经过任何第三方，不落盘、不打日志。
package zbz

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"infinite-canvas/backend/internal/outbound"

	"github.com/gin-gonic/gin"
)

type balanceReq struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

type balanceResp struct {
	Provider  string  `json:"provider"`
	Supported bool    `json:"supported"`
	Balance   float64 `json:"balance"`
	Currency  string  `json:"currency"`
	Detail    string  `json:"detail,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type adapter struct {
	id       string
	match    func(host string) bool
	endpoint string
	parse    func(b []byte) (float64, string, string, error)
}

func num(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

var adapters = []adapter{
	{"deepseek", func(h string) bool { return strings.HasSuffix(h, "deepseek.com") }, "https://api.deepseek.com/user/balance",
		func(b []byte) (float64, string, string, error) {
			var r struct {
				BalanceInfos []struct {
					Currency     string `json:"currency"`
					TotalBalance string `json:"total_balance"`
				} `json:"balance_infos"`
			}
			if err := json.Unmarshal(b, &r); err != nil || len(r.BalanceInfos) == 0 {
				return 0, "", "", errBad
			}
			return num(r.BalanceInfos[0].TotalBalance), r.BalanceInfos[0].Currency, "", nil
		}},
	{"moonshot", func(h string) bool { return strings.HasSuffix(h, "moonshot.cn") || strings.HasSuffix(h, "moonshot.ai") }, "https://api.moonshot.cn/v1/users/me/balance",
		func(b []byte) (float64, string, string, error) {
			var r struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(b, &r); err != nil || r.Data == nil {
				return 0, "", "", errBad
			}
			return num(r.Data["available_balance"]), "CNY", "", nil
		}},
	{"siliconflow", func(h string) bool { return strings.Contains(h, "siliconflow") }, "https://api.siliconflow.cn/v1/user/info",
		func(b []byte) (float64, string, string, error) {
			var r struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(b, &r); err != nil || r.Data == nil {
				return 0, "", "", errBad
			}
			return num(r.Data["totalBalance"]), "CNY", "", nil
		}},
	{"openrouter", func(h string) bool { return strings.HasSuffix(h, "openrouter.ai") }, "https://openrouter.ai/api/v1/credits",
		func(b []byte) (float64, string, string, error) {
			var r struct {
				Data struct {
					Total float64 `json:"total_credits"`
					Used  float64 `json:"total_usage"`
				} `json:"data"`
			}
			if err := json.Unmarshal(b, &r); err != nil {
				return 0, "", "", errBad
			}
			return r.Data.Total - r.Data.Used, "USD", "", nil
		}},
}

type badErr struct{}

func (badErr) Error() string { return "平台返回的格式无法识别" }

var errBad = badErr{}

func handleBalance(c *gin.Context) {
	var in balanceReq
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.BaseURL) == "" {
		fail(c, http.StatusBadRequest, "参数格式错误")
		return
	}
	u, err := url.Parse(strings.TrimSpace(in.BaseURL))
	if err != nil || u.Host == "" {
		fail(c, http.StatusBadRequest, "渠道地址无效")
		return
	}
	host := strings.ToLower(u.Hostname())
	for _, a := range adapters {
		if !a.match(host) {
			continue
		}
		out := balanceResp{Provider: a.id, Supported: true}
		if strings.TrimSpace(in.APIKey) == "" {
			out.Error = "这个渠道还没填 API Key"
			ok(c, out)
			return
		}
		req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, a.endpoint, nil)
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(in.APIKey))
		req.Header.Set("Accept", "application/json")
		client := outbound.OutboundHTTPClient(8 * time.Second)
		resp, err := client.Do(req)
		if err != nil {
			out.Error = "查询失败：网络不通"
			ok(c, out)
			return
		}
		defer resp.Body.Close()
		var buf strings.Builder
		_, _ = ioCopyLimit(&buf, resp)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			out.Error = "Key 无效或没有查询余额的权限"
			ok(c, out)
			return
		}
		if resp.StatusCode/100 != 2 {
			out.Error = "查询失败：" + resp.Status
			ok(c, out)
			return
		}
		bal, cur, detail, perr := a.parse([]byte(buf.String()))
		if perr != nil {
			out.Error = perr.Error()
		} else {
			out.Balance, out.Currency, out.Detail = bal, cur, detail
		}
		ok(c, out)
		return
	}
	ok(c, balanceResp{Provider: host, Supported: false, Error: "这个平台没有公开的余额接口，请到官网控制台查看"})
}
