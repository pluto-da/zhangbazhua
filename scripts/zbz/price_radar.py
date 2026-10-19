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
    s = re.sub(r"[-_@](20\d{2}-?\d{2}-?\d{2}|\d{6}|\d{4})$", "", s)  # 日期后缀
    s = re.sub(r"^(us|eu|apac|global)\.", "", s)
    s = re.sub(r"^(anthropic|meta|amazon|mistral|cohere|ai21)\.", "", s)
    s = re.sub(r"-v\d+:\d+$", "", s)
    s = re.sub(r"(?<=\d)\.(?=\d)", "-", s)  # 4.5 → 4-5，与官方命名对齐
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


VIDEO_RE = re.compile(r"veo|sora|kling|seedance|hailuo|vidu|runway|luma|pika|happyhorse|video|t2v|i2v|r2v|s2v|jimeng|wanx?[\d.\-]*-(t2v|i2v|kf2v)", re.I)
IMAGE_RE = re.compile(r"image|flux|midjourney|^mj[-_]|seedream|imagen|dall-?e|kolors|recraft|ideogram|sdxl|stable-diffusion|qwen-image|gpt-4o-image", re.I)
DUR_RE = re.compile(r"(\d{1,2})\s*(s|秒|sec)\b", re.I)


def kind_of(name, desc=""):
    if VIDEO_RE.search(name) and not re.search(r"understand|vl|vision|embed|caption", name, re.I):
        return "video"
    if IMAGE_RE.search(name) and not re.search(r"vision|vl\b|understand|embed", name, re.I):
        return "image"
    return "text"


def from_litellm_media(d):
    for key, m in d.items():
        if key == "sample_spec" or not isinstance(m, dict):
            continue
        mode = m.get("mode")
        prov = (m.get("litellm_provider") or key.split("/")[0]).lower()
        base = dict(model=key, name=key.split("/")[-1], provider=prov, providerName=prov, source="LiteLLM",
                    url="https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json",
                    china=bool(CHINA.search(prov)), context=0, inputUsd=0.0, outputUsd=0.0, cacheReadUsd=None)
        if mode == "video_generation":
            per = m.get("output_cost_per_second_720p") or m.get("output_cost_per_second") or m.get("output_cost_per_video_per_second")
            if not per:
                continue
            res = "720p" if m.get("output_cost_per_second_720p") else ""
            yield dict(base, modalities=["video"], unit="second", unitUsd=float(per), res=res,
                       res480=m.get("output_cost_per_second_480p"), res1080=m.get("output_cost_per_second_1080p"))
        elif mode == "image_generation":
            per = m.get("output_cost_per_image")
            if not per:
                continue
            yield dict(base, modalities=["image"], unit="image", unitUsd=float(per))


def _get(url, timeout=20):
    req = urllib.request.Request(url, headers=UA)
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read().decode("utf-8"))


def pick_group(enable_groups, group_ratio):
    """优先 default；否则取可用分组中倍率最低的。返回 (分组, 倍率, 最低分组, 最低倍率)。"""
    cands = [(g, float(group_ratio[g])) for g in (enable_groups or []) if g in group_ratio]
    if not cands:
        r = float(group_ratio.get("default", 1) or 1)
        return "default", r, "default", r
    low = min(cands, key=lambda x: x[1])
    for g, r in cands:
        if g == "default":
            return g, r, low[0], low[1]
    return low[0], low[1], low[0], low[1]


def newapi_offers(relay, status, pricing, fx):
    """把 New API 的 /api/status + /api/pricing 换算成实付价。返回 offer 列表（美元等值 = 实付人民币 ÷ 汇率）。"""
    price = status.get("price")
    try:
        price = float(price)
    except (TypeError, ValueError):
        price = 0
    # 充值价明显异常（未配置或超出 0.1～10 元/美元额度）时按 1 元估算，并标注“充值价未知”
    assumed = not (0.1 <= price <= 10)
    if assumed:
        price = 1.0
    group_ratio = pricing.get("group_ratio") or {}
    out = []
    for r in pricing.get("data") or []:
        name = r.get("model_name") or ""
        if not name:
            continue
        g, gr, lg, lgr = pick_group(r.get("enable_groups"), group_ratio)
        desc = r.get("description") or ""
        kind = kind_of(name, desc)
        base = dict(model=name, name=name, provider="relay:" + relay["id"], providerName=relay["name"], source="中转站",
                    url=relay.get("homepage") or relay["base"], china=True, context=0, cacheReadUsd=None,
                    modalities=[kind], group=g, groupRatio=gr, minGroup=lg, minGroupRatio=lgr,
                    rechargePrice=price, rechargeAssumed=assumed, site=relay["base"], inputUsd=0.0, outputUsd=0.0)
        if int(r.get("quota_type") or 0) == 0:
            ratio = float(r.get("model_ratio") or 0)
            if ratio <= 0 or kind != "text":
                continue
            cny_in = ratio * 2 * gr * price
            cny_out = cny_in * float(r.get("completion_ratio") or 1)
            out.append(dict(base, unit="token", inputUsd=cny_in / fx, outputUsd=cny_out / fx, cnyReal=round(cny_in, 6)))
        else:
            mp = float(r.get("model_price") or 0)
            if mp <= 0:
                continue
            cny = mp * gr * price
            o = dict(base, cnyCall=round(cny, 6))
            if kind == "video":
                m = DUR_RE.search(name) or DUR_RE.search(desc)
                secs = int(m.group(1)) if m else 0
                if 1 <= secs <= 60:
                    out.append(dict(o, unit="second", unitUsd=cny / secs / fx, assumedSeconds=secs, cnyReal=round(cny / secs, 6)))
                else:
                    out.append(dict(o, unit="call", unitUsd=cny / fx, cnyReal=round(cny, 6)))
            elif kind == "image":
                out.append(dict(o, unit="image", unitUsd=cny / fx, cnyReal=round(cny, 6)))
            else:
                out.append(dict(o, unit="call", unitUsd=cny / fx, cnyReal=round(cny, 6)))
    return out


def from_relays(fx, path):
    from concurrent.futures import ThreadPoolExecutor
    reg = json.load(open(path, encoding="utf-8"))["relays"]

    def one(relay):
        if relay.get("pricingPublic") is False:
            return relay, [], {"name": relay["name"], "url": relay["base"], "ok": False, "kind": "relay", "error": "该站价格需登录查看"}
        try:
            st = (_get(relay["base"] + "/api/status") or {}).get("data") or {}
            pr = _get(relay["base"] + "/api/pricing", 40)
            if not isinstance(pr, dict) or not pr.get("data"):
                raise ValueError("未公开价格表")
            if "group_price" in pr["data"][0] and "model_ratio" not in pr["data"][0]:
                raise ValueError("新版分组价格格式，口径未核实，暂不解析")
            rows = newapi_offers(relay, st, pr, fx)
            return relay, rows, {"name": relay["name"], "url": relay["base"], "ok": True, "kind": "relay", "count": len(rows), "rechargePrice": st.get("price")}
        except Exception as e:
            return relay, [], {"name": relay["name"], "url": relay["base"], "ok": False, "kind": "relay", "error": str(e)[:160]}

    with ThreadPoolExecutor(8) as ex:
        res = list(ex.map(one, reg))
    offers, states = [], []
    for relay, rows, st in res:
        offers.extend(rows); states.append(st)
    return offers, states


def unit_price(x):
    if x.get("unit", "token") == "token":
        return (3 * x["inputUsd"] + x["outputUsd"]) / 4
    return x.get("unitUsd") or 0


UNIT_RANK = {"token": 0, "second": 1, "image": 2, "call": 3}


def main():
    out_path = sys.argv[1] if len(sys.argv) > 1 else "prices.json"
    prev = None
    if "--previous" in sys.argv:
        try:
            prev = json.load(open(sys.argv[sys.argv.index("--previous") + 1]))
        except Exception:
            prev = None
    sources, offers = [], []
    fx, fx_note = 7.1, "fallback"
    try:
        fx = float(fetch(FX_URL, 20)["rates"]["CNY"]); fx_note = "open.er-api.com"
    except Exception:
        if prev and prev.get("usdCny"):
            fx, fx_note = prev["usdCny"], "previous"
    parsers = {"models.dev": from_models_dev, "OpenRouter": from_openrouter, "LiteLLM": from_litellm}
    priority = {"OpenRouter": 0, "models.dev": 1, "LiteLLM": 2, "中转站": 3}
    for name, url in SRC.items():
        try:
            raw = fetch(url)
            rows = list(parsers[name](raw))
            if name == "LiteLLM":
                media = list(from_litellm_media(raw))
                rows.extend(media)
            offers.extend(rows)
            sources.append({"name": name, "url": url, "ok": True, "count": len(rows)})
        except Exception as e:  # 单源失败不影响其他源
            sources.append({"name": name, "url": url, "ok": False, "error": str(e)[:200]})
    relay_ok = relay_fail = 0
    try:
        import os
        rows, states = from_relays(fx, os.path.join(os.path.dirname(os.path.abspath(__file__)), "relays.json"))
        offers.extend(rows); sources.extend(states)
        relay_ok = sum(1 for x in states if x["ok"]); relay_fail = len(states) - relay_ok
    except Exception as e:
        sources.append({"name": "中转站名单", "ok": False, "kind": "relay", "error": str(e)[:200]})

    models, anomalies, zero = {}, [], 0
    for o in offers:
        o.setdefault("unit", "token")
        if o["unit"] == "token":
            if o["inputUsd"] < 0 or o["outputUsd"] < 0:
                anomalies.append({"model": o["model"], "provider": o["provider"], "reason": "负价"}); continue
            if o["inputUsd"] == 0 and o["outputUsd"] == 0:
                zero += 1; continue  # 零价多为未标价或限时免费，未确认，不参与比价
        elif not o.get("unitUsd") or o["unitUsd"] <= 0:
            zero += 1; continue
        key = canonical(o["model"])
        if not key:
            continue
        m = models.setdefault(key, {"id": key, "name": o["name"], "modalities": set(), "context": 0, "offers": {}})
        m["modalities"].update(o["modalities"] or [])
        m["context"] = max(m["context"], int(o["context"] or 0))
        k = o["provider"] + ("" if o["unit"] == "token" else "#" + o["unit"])
        old = m["offers"].get(k)
        if old is None or priority[o["source"]] < priority[old["source"]] or (o["source"] == old["source"] == "中转站" and unit_price(o) < unit_price(old)):
            keep = ("provider", "providerName", "source", "inputUsd", "outputUsd", "cacheReadUsd", "url", "china", "unit", "unitUsd", "res", "res480", "res1080",
                    "assumedSeconds", "cnyReal", "cnyCall", "group", "groupRatio", "minGroup", "minGroupRatio", "rechargePrice", "rechargeAssumed", "site")
            m["offers"][k] = {x: o[x] for x in keep if x in o and o[x] not in (None, "")}
            m["offers"][k].setdefault("cacheReadUsd", None)
            m["offers"][k]["tier"] = "中转站" if o["source"] == "中转站" else tier(o["provider"])
            m["offers"][k]["rawModel"] = o["model"]

    out_models = []
    for m in models.values():
        offs = list(m["offers"].values())
        kept = []
        for unit in {x["unit"] for x in offs}:
            group = [x for x in offs if x["unit"] == unit]
            nz = [unit_price(x) for x in group if unit_price(x) > 0]
            med = statistics.median(nz) if nz else 0
            for x in group:
                b = unit_price(x)
                if med and (b > med * 100 or (b > 0 and b < med / 100)):
                    anomalies.append({"model": m["id"], "provider": x["provider"], "reason": "偏离中位数 100 倍"}); continue
                if x.get("cacheReadUsd") is None:
                    x.pop("cacheReadUsd", None)
                kept.append(x)
        if not kept:
            continue
        mods = set(m["modalities"])
        if any(x["unit"] in ("second",) for x in kept) or "video" in mods:
            mods.add("video")
        primary = "second" if "video" in mods and any(x["unit"] == "second" for x in kept) else min((x["unit"] for x in kept), key=lambda u: UNIT_RANK[u])
        kept.sort(key=lambda x: (0 if x["unit"] == primary else 1 + UNIT_RANK[x["unit"]], unit_price(x)))
        out_models.append({"id": m["id"], "name": m["name"], "modalities": sorted(mods), "context": m["context"], "primaryUnit": primary, "offers": kept})
    out_models.sort(key=lambda m: m["id"])

    if len(out_models) < 1000 and prev:
        print(f"模型数只有 {len(out_models)}，低于 1000，保留上次数据", file=sys.stderr)
        sys.exit(2)
    data = {
        "generatedAt": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "usdCny": round(fx, 4), "fx": fx_note, "unit": "token: USD per 1M tokens；second/image/call: unitUsd 为美元等值单价；中转站已按 额度×分组倍率×充值价 换算为实付",
        "sources": sources, "models": out_models, "anomalies": anomalies[:500],
        "stats": {"models": len(out_models), "offers": sum(len(m["offers"]) for m in out_models), "zeroPriceDropped": zero,
                  "relaysOk": relay_ok, "relaysFailed": relay_fail,
                  "videoModels": sum(1 for m in out_models if "video" in m["modalities"]),
                  "imageModels": sum(1 for m in out_models if "image" in m["modalities"])},
    }
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, separators=(",", ":"))
    print(json.dumps({"models": len(out_models), "offers": data["stats"]["offers"], "fx": fx, "sources": sources, "anomalies": len(anomalies)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
