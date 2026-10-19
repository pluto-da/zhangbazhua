// 章八爪模块 · 模型比价雷达 · 作者 长鱼茶汤茗
import { useQuery } from "@tanstack/react-query";
import { Alert, Button, Checkbox, Input, Segmented, Table, Tag } from "antd";
import { useMemo, useState } from "react";

import { PageHeader, WorkspacePage } from "@/components/layout/workspace-page";
import { useConfigStore } from "@/stores/use-config-store";

import { fetchPrices, type PriceModel, type PriceOffer } from "../api";
import { openExternal } from "../open-external";

type Modality = "全部" | "文本" | "图片" | "视频" | "语音";
const MODALITY_KEY: Record<Exclude<Modality, "全部">, string> = { 文本: "text", 图片: "image", 视频: "video", 语音: "audio" };

type RadarRow = PriceModel & { best: PriceOffer; worst: PriceOffer; offersShown: PriceOffer[]; mine: boolean };

function blend(o: PriceOffer, ratio: number) {
    return (ratio * o.inputUsd + o.outputUsd) / (ratio + 1);
}

function cny(usd: number, rate: number) {
    const v = usd * rate;
    if (v === 0) return "免费";
    return v < 0.1 ? `¥${v.toFixed(3)}` : `¥${v.toFixed(2)}`;
}

function hostOf(url: string) {
    try {
        return new URL(url).hostname.replace(/^www\./, "");
    } catch {
        return "";
    }
}

export default function PriceRadarPage() {
    const [query, setQuery] = useState("");
    const [modality, setModality] = useState<Modality>("全部");
    const [chinaOnly, setChinaOnly] = useState(false);
    const [officialOnly, setOfficialOnly] = useState(false);
    const [ratio, setRatio] = useState(3);
    const channels = useConfigStore((s) => s.config.channels);

    const q = useQuery({ queryKey: ["zbz-prices"], queryFn: () => fetchPrices(false), staleTime: 6 * 3600_000, retry: 1 });
    const data = q.data?.prices;
    const rate = data?.usdCny || 7.1;

    const myHosts = useMemo(() => new Set((channels || []).map((c) => hostOf(c.baseUrl)).filter(Boolean)), [channels]);
    const myModels = useMemo(() => new Set((channels || []).flatMap((c) => c.models || []).map((m) => m.toLowerCase().split("/").pop() as string)), [channels]);

    const rows = useMemo((): RadarRow[] => {
        if (!data) return [];
        const needle = query.trim().toLowerCase();
        const out: RadarRow[] = [];
        for (const m of data.models) {
            if (needle && !m.id.includes(needle) && !(m.name || "").toLowerCase().includes(needle)) continue;
            if (modality !== "全部" && !(m.modalities || []).includes(MODALITY_KEY[modality])) continue;
            let offers = m.offers.filter((o) => o.inputUsd >= 0 && o.outputUsd >= 0);
            if (chinaOnly) offers = offers.filter((o) => o.china);
            if (officialOnly) offers = offers.filter((o) => o.tier === "官方/云厂商");
            if (!offers.length) continue;
            offers = [...offers].sort((a, b) => blend(a, ratio) - blend(b, ratio));
            out.push({ ...m, best: offers[0], worst: offers[offers.length - 1], offersShown: offers, mine: myModels.has(m.id) });
        }
        out.sort((a, b) => Number(b.mine) - Number(a.mine) || (needle ? blend(a.best, ratio) - blend(b.best, ratio) : b.offersShown.length - a.offersShown.length));
        return out.slice(0, 300);
    }, [data, query, modality, chinaOnly, officialOnly, ratio, myModels]);

    return (
        <WorkspacePage>
            <PageHeader
                title="模型比价雷达"
                description="同一个模型，各家平台的调用价格（每百万 tokens，人民币）。价格每天自动汇总，以官方页面为准。"
                actions={<Button loading={q.isFetching} onClick={() => void fetchPrices(true).then(() => q.refetch())}>刷新数据</Button>}
            />
            {q.isError ? <Alert className="mt-3" type="error" showIcon title="价格数据拉取失败" description="需要能访问 jsDelivr、GitHub 或 models.dev。检查网络或代理后点“刷新数据”。" /> : null}
            {q.data?.stale ? <Alert className="mt-3" type="warning" showIcon title={q.data.note} /> : null}
            {data?.degraded ? <Alert className="mt-3" type="info" showIcon title="云端汇总暂不可用，当前只用 models.dev 一个来源，汇率按 7.1 估算。" /> : null}
            <div className="mt-4 flex flex-wrap items-center gap-3">
                <Input.Search allowClear placeholder="搜模型，比如 deepseek、qwen、glm、kimi、claude" className="max-w-md" onSearch={setQuery} onChange={(e) => setQuery(e.target.value)} />
                <Segmented<Modality> options={["全部", "文本", "图片", "视频", "语音"]} value={modality} onChange={setModality} />
                <Checkbox checked={chinaOnly} onChange={(e) => setChinaOnly(e.target.checked)}>只看国内直连</Checkbox>
                <Checkbox checked={officialOnly} onChange={(e) => setOfficialOnly(e.target.checked)}>只看官方/云厂商</Checkbox>
                <span className="text-sm text-foreground/60">综合价权重 输入:输出 =</span>
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
                            rowKey={(o) => `${o.provider}-${o.source}`}
                            pagination={false}
                            dataSource={m.offersShown}
                            columns={[
                                { title: "平台", render: (_, o) => <span>{o.providerName || o.provider}{myHosts.has(hostOf(o.url)) ? <Tag className="ml-2" color="blue">你在用</Tag> : null}{o.china ? <Tag className="ml-1">国内</Tag> : null}</span> },
                                { title: "输入", render: (_, o) => cny(o.inputUsd, rate) },
                                { title: "输出", render: (_, o) => cny(o.outputUsd, rate) },
                                { title: "缓存命中", render: (_, o) => (o.cacheReadUsd != null ? cny(o.cacheReadUsd, rate) : "—") },
                                { title: "综合", render: (_, o) => cny(blend(o, ratio), rate) },
                                { title: "类型", render: (_, o) => <Tag color={o.tier === "官方/云厂商" ? "green" : undefined}>{o.tier}</Tag> },
                                { title: "来源", render: (_, o) => <Tag>{o.source}</Tag> },
                                { title: "", render: (_, o) => (o.url ? <Button size="small" type="link" onClick={() => openExternal(o.url)}>价格页</Button> : null) },
                            ]}
                        />
                    ),
                }}
                columns={[
                    { title: "模型", render: (_, m) => <span className="font-medium">{m.id}{m.mine ? <Tag className="ml-2" color="blue">你在用</Tag> : null}</span> },
                    { title: "最低价渠道", render: (_, m) => m.best.providerName || m.best.provider },
                    { title: "输入", render: (_, m) => cny(m.best.inputUsd, rate) },
                    { title: "输出", render: (_, m) => cny(m.best.outputUsd, rate) },
                    { title: "综合", render: (_, m) => <b>{cny(blend(m.best, ratio), rate)}</b> },
                    { title: "比最贵省", render: (_, m) => { const w = blend(m.worst, ratio); const b = blend(m.best, ratio); return w > 0 && m.offersShown.length > 1 ? `${Math.round((1 - b / w) * 100)}%` : "—"; } },
                    { title: "渠道数", render: (_, m) => m.offersShown.length },
                ]}
            />
            <p className="mt-3 text-xs leading-5 text-foreground/45">
                数据来源：{(data?.sources || []).map((s) => `${s.name}${s.ok ? "" : "（本次失败）"}`).join("、") || "—"}；汇总时间 {data?.generatedAt ? new Date(data.generatedAt).toLocaleString() : "—"}；美元汇率 {rate}。价格仅供比较，实际以各平台官方页面为准；第三方中转站不收录。
            </p>
        </WorkspacePage>
    );
}
