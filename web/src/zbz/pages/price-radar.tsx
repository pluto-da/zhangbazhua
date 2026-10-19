// 章八爪模块 · 模型比价雷达 · 作者 长鱼茶汤茗
// v2.0：纳入视频/图片（按秒、按张、按次）与中转站（额度 × 分组倍率 × 充值价 = 实付）。
import { useQuery } from "@tanstack/react-query";
import { Alert, Button, Checkbox, Input, Segmented, Table, Tag } from "antd";
import { useMemo, useState } from "react";

import { PageHeader, WorkspacePage } from "@/components/layout/workspace-page";
import { useConfigStore } from "@/stores/use-config-store";

import { fetchPrices, type PriceModel, type PriceOffer } from "../api";
import { openExternal } from "../open-external";
import { RISK_TEXT, canonicalId, priceText, relayDetail, riskBadges, unitOf, unitPrice, yuan, type Unit } from "../price-units";

type Modality = "全部" | "文本" | "图片" | "视频" | "语音";
const MODALITY_KEY: Record<Exclude<Modality, "全部">, string> = { 文本: "text", 图片: "image", 视频: "video", 语音: "audio" };
type TierFilter = "全部" | "官方/云厂商" | "第三方/聚合" | "中转站";
const UNIT_RANK: Record<Unit, number> = { second: 0, token: 1, image: 2, call: 3 };

type RadarRow = PriceModel & { unit: Unit; best: PriceOffer; worst: PriceOffer; same: PriceOffer[]; offersShown: PriceOffer[]; officialMin?: number; mine: boolean };

function hostOf(url: string) {
    try {
        return new URL(url).hostname.replace(/^www\./, "");
    } catch {
        return "";
    }
}

function tierColor(t: string) {
    return t === "官方/云厂商" ? "green" : t === "中转站" ? "purple" : undefined;
}

export default function PriceRadarPage() {
    const [query, setQuery] = useState("");
    const [modality, setModality] = useState<Modality>("全部");
    const [tierFilter, setTierFilter] = useState<TierFilter>("全部");
    const [chinaOnly, setChinaOnly] = useState(false);
    const [ratio, setRatio] = useState(3);
    const channels = useConfigStore((s) => s.config.channels);

    const q = useQuery({ queryKey: ["zbz-prices"], queryFn: () => fetchPrices(false), staleTime: 3 * 3600_000, retry: 1 });
    const data = q.data?.prices;
    const rate = data?.usdCny || 7.1;

    const myHosts = useMemo(() => new Set((channels || []).map((c) => hostOf(c.baseUrl)).filter(Boolean)), [channels]);
    const myModels = useMemo(() => new Set((channels || []).flatMap((c) => c.models || []).map(canonicalId)), [channels]);

    const rows = useMemo((): RadarRow[] => {
        if (!data) return [];
        const needle = canonicalId(query);
        const out: RadarRow[] = [];
        for (const m of data.models) {
            if (needle && !m.id.includes(needle) && !(m.name || "").toLowerCase().includes(query.trim().toLowerCase())) continue;
            if (modality !== "全部" && !(m.modalities || []).includes(MODALITY_KEY[modality])) continue;
            let offers = m.offers.filter((o) => unitPrice(o, ratio) > 0);
            if (chinaOnly) offers = offers.filter((o) => o.china);
            if (tierFilter !== "全部") offers = offers.filter((o) => o.tier === tierFilter);
            if (!offers.length) continue;
            const unit = offers.map(unitOf).sort((a, b) => UNIT_RANK[a] - UNIT_RANK[b])[0];
            const same = offers.filter((o) => unitOf(o) === unit).sort((a, b) => unitPrice(a, ratio) - unitPrice(b, ratio));
            const officials = m.offers.filter((o) => o.tier === "官方/云厂商" && unitOf(o) === unit).map((o) => unitPrice(o, ratio));
            const sorted = [...offers].sort((a, b) => (unitOf(a) === unitOf(b) ? unitPrice(a, ratio) - unitPrice(b, ratio) : UNIT_RANK[unitOf(a)] - UNIT_RANK[unitOf(b)]));
            out.push({ ...m, unit, best: same[0], worst: same[same.length - 1], same, offersShown: sorted, officialMin: officials.length ? Math.min(...officials) : undefined, mine: myModels.has(m.id) });
        }
        out.sort((a, b) => Number(b.mine) - Number(a.mine) || (needle ? unitPrice(a.best, ratio) - unitPrice(b.best, ratio) : b.offersShown.length - a.offersShown.length));
        return out.slice(0, 300);
    }, [data, query, modality, chinaOnly, tierFilter, ratio, myModels]);

    const relaySources = (data?.sources || []).filter((s) => s.kind === "relay");

    return (
        <WorkspacePage>
            <PageHeader
                title="模型比价雷达"
                description="同一个模型在官方、云厂商、聚合平台和中转站的实付价格（人民币）。文本按每百万 tokens，视频按每秒，图片按每张。数据每 3 小时汇总一次，以各平台页面为准。"
                actions={<Button loading={q.isFetching} onClick={() => void fetchPrices(true).then(() => q.refetch())}>刷新数据</Button>}
            />
            {q.isError ? <Alert className="mt-3" type="error" showIcon title="价格数据拉取失败" description="需要能访问 jsDelivr、GitHub 或 models.dev。检查网络或代理后点“刷新数据”。" /> : null}
            {q.data?.stale ? <Alert className="mt-3" type="warning" showIcon title={q.data.note} /> : null}
            {data?.degraded ? <Alert className="mt-3" type="info" showIcon title="云端汇总暂不可用，当前只用 models.dev 一个来源，汇率按 7.1 估算。" /> : null}
            <Alert className="mt-3" type="info" showIcon title="中转站价格 = 额度价 × 分组倍率 × 充值价，已换算成实付人民币。" description={RISK_TEXT} />
            <div className="mt-4 flex flex-wrap items-center gap-3">
                <Input.Search allowClear placeholder="搜模型，比如 seedance、kling、wan、veo、sora、deepseek、qwen" className="max-w-md" onSearch={setQuery} onChange={(e) => setQuery(e.target.value)} />
                <Segmented<Modality> options={["全部", "文本", "图片", "视频", "语音"]} value={modality} onChange={setModality} />
                <Segmented<TierFilter> options={["全部", "官方/云厂商", "第三方/聚合", "中转站"]} value={tierFilter} onChange={setTierFilter} />
                <Checkbox checked={chinaOnly} onChange={(e) => setChinaOnly(e.target.checked)}>只看国内直连</Checkbox>
                <span className="text-sm text-foreground/60">文本综合价 输入:输出 =</span>
                <Segmented<number> options={[{ label: "1:1", value: 1 }, { label: "3:1", value: 3 }, { label: "10:1", value: 10 }]} value={ratio} onChange={setRatio} />
            </div>
            <Table
                className="mt-4"
                size="small"
                loading={q.isLoading}
                rowKey="id"
                dataSource={rows}
                pagination={{ pageSize: 30, showSizeChanger: false }}
                expandable={{
                    expandedRowRender: (m) => (
                        <Table
                            size="small"
                            rowKey={(o) => `${o.provider}-${o.source}-${unitOf(o)}`}
                            pagination={false}
                            dataSource={m.offersShown}
                            columns={[
                                { title: "平台", render: (_, o) => <span>{o.providerName || o.provider}{myHosts.has(hostOf(o.url)) ? <Tag className="ml-2" color="blue">你在用</Tag> : null}{o.china ? <Tag className="ml-1">国内</Tag> : null}</span> },
                                { title: "价格", render: (_, o) => (unitOf(o) === "token" ? <span>输入 {yuan(o.inputUsd, rate)} · 输出 {yuan(o.outputUsd, rate)} · 综合 <b>{yuan(unitPrice(o, ratio), rate)}</b></span> : <b>{priceText(o, rate, ratio)}</b>) },
                                { title: "类型", render: (_, o) => <span><Tag color={tierColor(o.tier)}>{o.tier}</Tag>{riskBadges(o, unitOf(o) === m.unit ? m.officialMin : undefined, ratio).map((b) => <Tag key={b.text} color={b.color}>{b.text}</Tag>)}</span> },
                                { title: "说明", render: (_, o) => <span className="text-xs text-foreground/55">{relayDetail(o) || o.source}{o.rawModel && canonicalId(o.rawModel) !== o.rawModel.toLowerCase() ? ` · 原名 ${o.rawModel}` : ""}</span> },
                                { title: "", render: (_, o) => (o.url ? <Button size="small" type="link" onClick={() => openExternal(o.url)}>{o.tier === "中转站" ? "官网" : "价格页"}</Button> : null) },
                            ]}
                        />
                    ),
                }}
                columns={[
                    { title: "模型", render: (_, m) => <span className="font-medium">{m.id}{m.mine ? <Tag className="ml-2" color="blue">你在用</Tag> : null}{(m.modalities || []).includes("video") ? <Tag className="ml-1" color="magenta">视频</Tag> : (m.modalities || []).includes("image") ? <Tag className="ml-1" color="cyan">图片</Tag> : null}</span> },
                    { title: "最低价渠道", render: (_, m) => <span>{m.best.providerName || m.best.provider} <Tag color={tierColor(m.best.tier)}>{m.best.tier}</Tag></span> },
                    { title: "最低价", render: (_, m) => <b>{priceText(m.best, rate, ratio)}</b> },
                    { title: "官方最低", render: (_, m) => (m.officialMin ? yuan(m.officialMin, rate) : "—") },
                    { title: "比最贵省", render: (_, m) => { const w = unitPrice(m.worst, ratio); const b = unitPrice(m.best, ratio); return w > 0 && m.same.length > 1 ? `${Math.round((1 - b / w) * 100)}%` : "—"; } },
                    { title: "渠道数", render: (_, m) => m.offersShown.length },
                ]}
            />
            <p className="mt-3 text-xs leading-5 text-foreground/45">
                数据来源：{(data?.sources || []).filter((s) => s.kind !== "relay").map((s) => `${s.name}${s.ok ? "" : "（本次失败）"}`).join("、") || "—"}；中转站 {relaySources.filter((s) => s.ok).length}/{relaySources.length} 家拉取成功{relaySources.some((s) => !s.ok) ? `（未收录：${relaySources.filter((s) => !s.ok).map((s) => `${s.name}·${s.error || "失败"}`).join("，")}）` : ""}；汇总时间 {data?.generatedAt ? new Date(data.generatedAt).toLocaleString() : "—"}；美元汇率 {rate}。不同计费单位不混排；按次计费又看不出时长的视频单列为“/次”。
            </p>
        </WorkspacePage>
    );
}
