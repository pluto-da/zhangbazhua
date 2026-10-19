package zbz

import (
	"os"
	"testing"
	"time"
)

// 规则取自 OpenRouter deepseek/deepseek-v4.1-flash-1017 的真实 overrides（2026-10-19 实测）。
const dsOverrides = `{"data":[{"id":"deepseek/deepseek-v4.1-flash-1017","pricing":{"prompt":"0.0000003","completion":"0.0000012","overrides":[
{"utc_days":["saturday","sunday"],"prompt":"0.00000015","completion":"0.0000006"},
{"utc_days":["monday","tuesday","wednesday","thursday","friday"],"utc_start":0,"utc_end":100,"prompt":"0.00000015","completion":"0.0000006"},
{"utc_days":["monday","tuesday","wednesday","thursday","friday"],"utc_start":100,"utc_end":400,"prompt":"0.0000003","completion":"0.0000012"},
{"utc_days":["monday","tuesday","wednesday","thursday","friday"],"utc_start":400,"utc_end":600,"prompt":"0.00000015","completion":"0.0000006"},
{"utc_days":["monday","tuesday","wednesday","thursday","friday"],"utc_start":600,"utc_end":1000,"prompt":"0.0000003","completion":"0.0000012"},
{"utc_days":["monday","tuesday","wednesday","thursday","friday"],"utc_start":1000,"utc_end":0,"prompt":"0.00000015","completion":"0.0000006"},
{"min_prompt_tokens":100000,"prompt":"0.000009","completion":"0.000009"}]}},
{"id":"x/free-model:free","pricing":{"prompt":"0","completion":"0"}}]}`

func dsOffer(t *testing.T) liveOffer {
	t.Helper()
	m, err := parseOpenRouter([]byte(dsOverrides))
	if err != nil {
		t.Fatal(err)
	}
	list := m["deepseek-v4.1-flash"]
	if len(list) != 1 || !list[0].Timed || len(list[0].rules) != 6 {
		t.Fatalf("解析分时规则失败: %+v", list)
	}
	return list[0]
}

func near(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

func TestTimedPricing(t *testing.T) {
	o := dsOffer(t)
	cases := []struct {
		at      string
		in, out float64
	}{
		{"2026-10-24T12:00:00Z", 0.15, 0.6}, // 周六
		{"2026-10-20T02:00:00Z", 0.3, 1.2},  // 周二 高价段
		{"2026-10-20T23:30:00Z", 0.15, 0.6}, // 周二 1000-0(2400) 低价段
		{"2026-10-20T05:00:00Z", 0.15, 0.6}, // 周二 400-600 低价段
	}
	for _, c := range cases {
		tm, _ := time.Parse(time.RFC3339, c.at)
		in, out := effectivePrice(o.BaseInputUsd, o.BaseOutputUsd, o.rules, tm)
		if !near(in, c.in) || !near(out, c.out) {
			t.Errorf("%s: got %v/%v want %v/%v", c.at, in, out, c.in, c.out)
		}
	}
}

func TestNextChange(t *testing.T) {
	o := dsOffer(t)
	tm, _ := time.Parse(time.RFC3339, "2026-10-20T02:00:00Z")
	at, b, ok := nextChange(o.BaseInputUsd, o.BaseOutputUsd, o.rules, tm)
	if !ok || at.UTC().Format("15:04") != "04:00" || !near(b, blendUsd(0.15, 0.6)) {
		t.Fatalf("next change got %v %v %v", at, b, ok)
	}
}

func TestTokenTierIgnoredAndZeroDropped(t *testing.T) {
	m, _ := parseOpenRouter([]byte(dsOverrides))
	for _, r := range m["deepseek-v4.1-flash"][0].rules {
		if r.TokenTier {
			t.Fatal("min_prompt_tokens 规则不应进入时段规则")
		}
	}
	lm := buildModel("free-model", m["free-model"], time.Now(), false)
	if lm.Best != nil {
		t.Fatal("零价不应成为最低价")
	}
}

func TestParseWanted(t *testing.T) {
	got := parseWanted("DeepSeek/DeepSeek-V4.1-Flash-1017, <script>, qwen3-max,,qwen3-max")
	if len(got) != 2 || got[0] != "deepseek-v4.1-flash" || got[1] != "qwen3-max" {
		t.Fatalf("got %v", got)
	}
}

func TestHistory(t *testing.T) {
	dir := t.TempDir()
	l := newLive(dir, &pricing{})
	_ = os.WriteFile(l.hist, []byte(`{"2026-10-18":{"m":2},"2026-10-19":{"m":1}}`), 0o644)
	now, _ := time.Parse(time.RFC3339, "2026-10-20T04:00:00Z")
	ms := []liveModel{{ID: "m", Best: &liveOffer{}, BaseBestBlendUsd: 0.9}}
	l.applyHistory(ms, now)
	if ms[0].YesterdayBlendUsd == nil || *ms[0].YesterdayBlendUsd != 1 || *ms[0].ChangePct != -10 || !ms[0].IsNewLow {
		t.Fatalf("history got %+v", ms[0])
	}
}
