#!/usr/bin/env python3
"""章八爪模块 · 模型比价雷达数据管线 · 作者 长鱼茶汤茗
汇总三个公开价格库，按模型标准名合并，输出 prices.json（每百万 tokens 美元价 + 当日美元兑人民币汇率）。
来源：
  models.dev   https://models.dev/api.json   （GitHub sst/models.dev）
  OpenRouter   https://openrouter.ai/api/v1/models
  LiteLLM      https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json
用法：python scripts/zbz/price_radar.py out/prices.json [--previous old.json]
"""
import json, re, statistics, sys, time, urllib.request
from datetime import datetime, timezone

UA = {"User-Agent": "ZhangBaZhua-PriceRadar/1.0 (+https://github.com/pluto-da/zhangbazhua)"}
SRC = {
    "models.dev": "https://models.dev/api.json",
    "OpenRouter": "https://openrouter.ai/api/v1/models",
    "LiteLLM": "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json",
}
FX_URL = "https://open.er-api.com/v6/latest/USD"
# 订阅套餐类价格不是按量计费，不参与比价
SKIP_PROVIDER = re.compile(r"coding-plan|token-plan|step-plan|subscription", re.I)
AGGREGATOR = re.compile(r"openrouter|siliconflow|aihubmix|requesty|zenmux|nano-gpt|aimlapi|together|fireworks|deepinfra|novita|groq|orcarouter|unorouter|lowrouter|bothub|vercel|helicone|302ai|ppio", re.I)
CHINA = re.compile(r"(-cn$|^deepseek$|^zhipuai$|^moonshotai-cn$|^siliconflow-cn$|^alibaba-cn$|^volcengine$|^xiaomi$|^tencent|^minimax-cn$|^baidu|^stepfun|^sensenova|^moark|^ppio|^modelscope)", re.I)


def fetch(url, timeout=60):
    req = urllib.request.Request(url, headers=UA)
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read().decode("utf-8"))


def canonical(mid: str) -> str:
    s = mid.strip().lower()
    s = s.split("/")[-1]
    s = re.sub(r":(free|beta|extended|thinking|nitro|floor)$", "", s)
    s = re.sub(r"[-_@](20\d{2}-?\d{2}-?\d{2}|\d{4})$", "", s)  # 日期后缀
    s = re.sub(r"^(us|eu|apac|global)\.", "", s)
    s = re.sub(r"^(anthropic|meta|amazon|mistral|cohere|ai21)\.", "", s)
    s = re.sub(r"-v\d+:\d+$", "", s)
    return s


OFFICIAL = re.compile(r"^(openai|anthropic|google|google-vertex|google-vertex-anthropic|vertex_ai.*|gemini|azure|azure_ai|amazon-bedrock|bedrock.*|deepseek|alibaba|alibaba-cn|dashscope|zhipuai|zai|moonshotai|moonshotai-cn|moonshot|xiaomi|volcengine|tencent.*|minimax|minimax-cn|mistral|xai|cohere|baidu.*|stepfun.*|sensenova|oci|databricks|nvidia|cerebras|groq|siliconflow|siliconflow-cn|openrouter)$", re.I)


def tier(provider: str) -> str:
    return "官方/云厂商" if OFFICIAL.search(provider) else "第三方/聚合"


def from_models_dev(d):
    for pid, prov in d.items():
        if SKIP_PROVIDER.search(pid):
            continue
        doc = prov.get("doc") or ""
        api = prov.get("api") or ""
        for mid, m in (prov.get("models") or {}).items():
            c = m.get("cost")
            if not c or c.get("input") is None or c.get("output") is None:
                continue
            yield dict(model=mid, name=m.get("name") or mid, provider=pid.lower(), providerName=prov.get("name") or pid,
                       source="models.dev", inputUsd=float(c["input"]), outputUsd=float(c["output"]),
                       cacheReadUsd=c.get("cache_read"), url=doc or "https://models.dev",
                       china=bool(CHINA.search(pid) or ".cn" in api),
                       modalities=(m.get("modalities") or {}).get("output") or ["text"],
                       context=(m.get("limit") or {}).get("context") or 0)


def from_openrouter(d):
    for m in d.get("data", []):
        p = m.get("pricing") or {}
        try:
            i, o = float(p.get("prompt", -1)), float(p.get("completion", -1))
        except ValueError:
            continue
        if i < 0 or o < 0:
            continue
        cr = p.get("input_cache_read")
        arch = m.get("architecture") or {}
        yield dict(model=m["id"], name=m.get("name") or m["id"], provider="openrouter", providerName="OpenRouter",
                   source="OpenRouter", inputUsd=i * 1e6, outputUsd=o * 1e6,
                   cacheReadUsd=float(cr) * 1e6 if cr not in (None, "") else None,
                   url=f"https://openrouter.ai/{m['id']}", china=False,
                   modalities=arch.get("output_modalities") or ["text"], context=m.get("context_length") or 0)


def from_litellm(d):
    for key, m in d.items():
        if key == "sample_spec" or not isinstance(m, dict):
            continue
        if m.get("mode") not in ("chat", "completion", "responses"):
            continue
        i, o = m.get("input_cost_per_token"), m.get("output_cost_per_token")
        if i is None or o is None:
            continue
        prov = (m.get("litellm_provider") or key.split("/")[0]).lower()
        if SKIP_PROVIDER.search(prov):
            continue
        cr = m.get("cache_read_input_token_cost")
        yield dict(model=key, name=key.split("/")[-1], provider=prov, providerName=prov, source="LiteLLM",
                   inputUsd=float(i) * 1e6, outputUsd=float(o) * 1e6, cacheReadUsd=float(cr) * 1e6 if cr else None,
                   url="https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json",
                   china=bool(CHINA.search(prov)), modalities=["text"], context=m.get("max_input_tokens") or 0)


def main():
    out_path = sys.argv[1] if len(sys.argv) > 1 else "prices.json"
    prev = None
    if "--previous" in sys.argv:
        try:
            prev = json.load(open(sys.argv[sys.argv.index("--previous") + 1]))
        except Exception:
            prev = None
    sources, offers = [], []
    parsers = {"models.dev": from_models_dev, "OpenRouter": from_openrouter, "LiteLLM": from_litellm}
    priority = {"OpenRouter": 0, "models.dev": 1, "LiteLLM": 2}
    for name, url in SRC.items():
        try:
            rows = list(parsers[name](fetch(url)))
            offers.extend(rows)
            sources.append({"name": name, "url": url, "ok": True, "count": len(rows)})
        except Exception as e:  # 单源失败不影响其他源
            sources.append({"name": name, "url": url, "ok": False, "error": str(e)[:200]})
    fx, fx_note = 7.1, "fallback"
    try:
        fx = float(fetch(FX_URL, 20)["rates"]["CNY"]); fx_note = "open.er-api.com"
    except Exception:
        if prev and prev.get("usdCny"):
            fx, fx_note = prev["usdCny"], "previous"

    models, anomalies, zero = {}, [], 0
    for o in offers:
        if o["inputUsd"] < 0 or o["outputUsd"] < 0:
            anomalies.append({"model": o["model"], "provider": o["provider"], "reason": "负价"}); continue
        if o["inputUsd"] == 0 and o["outputUsd"] == 0:
            zero += 1; continue  # 零价多为未标价或限时免费，未确认，不参与比价
        key = canonical(o["model"])
        if not key:
            continue
        m = models.setdefault(key, {"id": key, "name": o["name"], "modalities": set(), "context": 0, "offers": {}})
        m["modalities"].update(o["modalities"] or [])
        m["context"] = max(m["context"], int(o["context"] or 0))
        k = o["provider"]
        old = m["offers"].get(k)
        if old is None or priority[o["source"]] < priority[old["source"]]:
            m["offers"][k] = {x: o[x] for x in ("provider", "providerName", "source", "inputUsd", "outputUsd", "cacheReadUsd", "url", "china")}
            m["offers"][k]["tier"] = tier(k)

    out_models = []
    for m in models.values():
        offs = list(m["offers"].values())
        blends = [(3 * x["inputUsd"] + x["outputUsd"]) / 4 for x in offs]
        nz = [b for b in blends if b > 0]
        med = statistics.median(nz) if nz else 0
        kept = []
        for x, b in zip(offs, blends):
            if med and (b > med * 100 or (b > 0 and b < med / 100)):
                anomalies.append({"model": m["id"], "provider": x["provider"], "reason": "偏离中位数 100 倍"}); continue
            if x["cacheReadUsd"] is None:
                x.pop("cacheReadUsd")
            kept.append(x)
        if not kept:
            continue
        kept.sort(key=lambda x: (3 * x["inputUsd"] + x["outputUsd"]) / 4)
        out_models.append({"id": m["id"], "name": m["name"], "modalities": sorted(m["modalities"]), "context": m["context"], "offers": kept})
    out_models.sort(key=lambda m: m["id"])

    if len(out_models) < 1000 and prev:
        print(f"模型数只有 {len(out_models)}，低于 1000，保留上次数据", file=sys.stderr)
        sys.exit(2)
    data = {
        "generatedAt": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "usdCny": round(fx, 4), "fx": fx_note, "unit": "USD per 1M tokens",
        "sources": sources, "models": out_models, "anomalies": anomalies[:500],
        "stats": {"models": len(out_models), "offers": sum(len(m["offers"]) for m in out_models), "zeroPriceDropped": zero},
    }
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, separators=(",", ":"))
    print(json.dumps({"models": len(out_models), "offers": data["stats"]["offers"], "fx": fx, "sources": sources, "anomalies": len(anomalies)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
