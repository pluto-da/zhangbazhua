// 章八爪模块 · 实时最低价 · 作者 长鱼茶汤茗
// 实时 = 打开即拉取 + 页面可见时每 5 分钟自动刷新 + 分时价按此刻计算；涨跌对比本机每日快照。
import { useQuery } from "@tanstack/react-query";
import { Alert, AutoComplete, Button, Empty, Spin, Switch, Tag } from "antd";
import { ArrowDown, ArrowUp, RotateCcw, X } from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { create } from "zustand";
import { persist } from "zustand/middleware";

import { PageHeader, WorkspacePage } from "@/components/layout/workspace-page";
import { useConfigStore } from "@/stores/use-config-store";

import { fetchLive, fetchPrices, type LiveModel, type LiveOffer } from "../api";
import { openExternal } from "../open-external";

const DEFAULT_WATCH = ["deepseek-v4-flash", "qwen3-max", "glm-4.6", "kimi-k2", "claude-sonnet-4.5", "gpt-5", "gemini-2.5-pro"];
const REFRESH_MS = 5 * 60_000;

type WatchStore = {
    list: string[] | null;
    officialOnly: boolean;
    setList: (list: string[] | null) => void;
    setOfficialOnly: (v: boolean) => void;
};

const useWatch = create<WatchStore>()(
    persist(
        (set) => ({
            list: null,
            officialOnly: false,
            setList: (list) => set({ list }),
            setOfficialOnly: (officialOnly) => set({ officialOnly }),
        }),
        { name: "zbz:live-watch" },
    ),
);

function simpleCanonical(id: string) {
    return id.trim().toLowerCase().split("/").pop() || "";
}

function siteOf(url: string) {
    try {
        const parts = new URL(url).hostname.split(".");
        return parts.slice(-2).join(".");
    } catch {
        return "";
    }
}

function blend(i: number, o: number) {
    return (3 * i + o) / 4;
}

function yuan(usd: number, rate: number) {
    const v = usd * rate;
    return v < 0.1 ? `¥${v.toFixed(3)}` : `¥${v.toFixed(2)}`;
}

function bjTime(iso: string) {
    return new Date(iso).toLocaleTimeString("zh-CN", { timeZone: "Asia/Shanghai", hour: "2-digit", minute: "2-digit", hour12: false });
}

function untilText(iso: string, now: number) {
    const ms = new Date(iso).getTime() - now;
    if (ms <= 0) return "即将变价";
    const h = Math.floor(ms / 3_600_000);
    const m = Math.floor((ms % 3_600_000) / 60_000);
    return h > 0 ? `${h} 小时 ${m} 分` : `${m} 分`;
}

function agoText(iso: string | undefined, now: number) {
    if (!iso) return "—";
    const m = Math.max(0, Math.floor((now - new Date(iso).getTime()) / 60_000));
    return m === 0 ? "刚刚" : `${m} 分钟前`;
}

function TimedLine({ offer, rate, now }: { offer: LiveOffer; rate: number; now: number }) {
    if (!offer.timed || !offer.nextChangeAt || offer.nextBlendUsd == null) return null;
    const cur = blend(offer.inputUsd, offer.outputUsd);
    const base = blend(offer.baseInputUsd, offer.baseOutputUsd);
    if (cur < base - 1e-12) {
        const label = cur < base * 0.6 ? "半价时段" : "优惠时段";
        return <p className="text-xs font-medium text-emerald-500">{label} · 还剩 {untilText(offer.nextChangeAt, now)}</p>;
    }
    if (offer.nextBlendUsd < cur) {
        return <p className="text-xs text-amber-500">北京时间 {bjTime(offer.nextChangeAt)} 起降到 {yuan(offer.nextBlendUsd, rate)}</p>;
    }
    return <p className="text-xs text-foreground/50">北京时间 {bjTime(offer.nextChangeAt)} 起变为 {yuan(offer.nextBlendUsd, rate)}</p>;
}

function ModelCard({ m, rate, now, mySites, onRemove }: { m: LiveModel; rate: number; now: number; mySites: Set<string> | undefined; onRemove: () => void }) {
    const best = m.best;
    let mine: ReactNode = null;
    if (best && mySites && mySites.size) {
        const used = m.offers.find((o) => mySites.has(siteOf(o.url)));
        if (used === best || (used && used.provider === best.provider)) mine = <p className="text-xs text-emerald-500">你用的已经是最低价</p>;
        else if (used) {
            const u = blend(used.inputUsd, used.outputUsd);
            const b = blend(best.inputUsd, best.outputUsd);
            mine = <p className="text-xs text-red-500">你在用的渠道比最低贵 {Math.round((u / b - 1) * 100)}%，换到 {best.providerName || best.provider} 每百万 tokens 省 {yuan(u - b, rate)}</p>;
        } else mine = <p className="text-xs text-foreground/45">你配置的渠道不在最便宜的前 5 名里</p>;
    }
    return (
        <section className="flex flex-col gap-2 rounded-[var(--r-md)] border border-foreground/10 bg-foreground/[0.02] p-4">
            <div className="flex items-start gap-2">
                <h2 className="min-w-0 flex-1 truncate text-sm font-semibold" title={m.id}>{m.id}</h2>
                {m.isNewLow ? <Tag color="green">新低</Tag> : null}
                <button type="button" aria-label="取消关注" className="text-foreground/40 hover:text-foreground" onClick={onRemove}><X className="size-4" /></button>
            </div>
            {best ? (
                <>
                    <div className="flex items-baseline gap-2">
                        <span className="text-2xl font-semibold">{yuan(blend(best.inputUsd, best.outputUsd), rate)}</span>
                        <span className="text-xs text-foreground/50">/ 百万 tokens 综合</span>
                    </div>
                    <p className="text-xs text-foreground/60">输入 {yuan(best.inputUsd, rate)} · 输出 {yuan(best.outputUsd, rate)}</p>
                    <p className="text-sm">{best.providerName || best.provider} <Tag className="ml-1" color={best.tier === "官方/云厂商" ? "green" : undefined}>{best.tier}</Tag>{best.china ? <Tag>国内</Tag> : null}</p>
                    {m.changePct == null ? (
                        <p className="text-xs text-foreground/45">首日，明天起显示涨跌</p>
                    ) : m.changePct === 0 ? (
                        <p className="text-xs text-foreground/50">和昨天持平</p>
                    ) : m.changePct < 0 ? (
                        <p className="flex items-center gap-1 text-xs text-emerald-500"><ArrowDown className="size-3" />比昨天 {Math.abs(m.changePct)}%</p>
                    ) : (
                        <p className="flex items-center gap-1 text-xs text-red-500"><ArrowUp className="size-3" />比昨天 {m.changePct}%</p>
                    )}
                    <TimedLine offer={best} rate={rate} now={now} />
                    {mine}
                    {m.offers.length > 1 ? (
                        <p className="text-xs leading-5 text-foreground/45">其次：{m.offers.slice(1, 3).map((o) => `${o.providerName || o.provider} ${yuan(blend(o.inputUsd, o.outputUsd), rate)}`).join("，")} · 共 {m.offerCount} 家</p>
                    ) : null}
                    <div><Button size="small" type="link" className="!px-0" onClick={() => openExternal(best.url)}>价格页</Button></div>
                </>
            ) : (
                <p className="text-sm text-foreground/50">暂无可比价格（可能拼写不同，换个名字搜搜）</p>
            )}
        </section>
    );
}

export default function LivePricesPage() {
    const channels = useConfigStore((s) => s.config.channels);
    const { list, officialOnly, setList, setOfficialOnly } = useWatch();
    const [input, setInput] = useState("");
    const [now, setNow] = useState(() => Date.now());

    const defaults = useMemo(() => {
        const mine = (channels || []).flatMap((c) => c.models || []).map(simpleCanonical).filter(Boolean);
        return Array.from(new Set([...mine.slice(0, 12), ...DEFAULT_WATCH]));
    }, [channels]);
    const watch = list ?? defaults;
    const mySites = useMemo(() => new Set((channels || []).filter((c) => c.apiKey || c.credentialRef).map((c) => siteOf(c.baseUrl)).filter(Boolean)), [channels]);

    const [visible, setVisible] = useState(() => document.visibilityState === "visible");
    useEffect(() => {
        const onVis = () => setVisible(document.visibilityState === "visible");
        document.addEventListener("visibilitychange", onVis);
        const t = window.setInterval(() => setNow(Date.now()), 30_000);
        return () => {
            document.removeEventListener("visibilitychange", onVis);
            window.clearInterval(t);
        };
    }, []);

    const live = useQuery({
        queryKey: ["zbz-live", watch.join(","), officialOnly],
        queryFn: () => fetchLive(watch, officialOnly),
        enabled: watch.length > 0,
        refetchInterval: visible ? REFRESH_MS : false,
        refetchOnWindowFocus: true,
        retry: 1,
    });
    const catalog = useQuery({ queryKey: ["zbz-prices"], queryFn: () => fetchPrices(false), staleTime: 6 * 3600_000, retry: 1 });
    const options = useMemo(() => {
        const needle = input.trim().toLowerCase();
        if (!needle || !catalog.data) return [];
        return catalog.data.prices.models.filter((m) => m.id.includes(needle)).slice(0, 12).map((m) => ({ value: m.id, label: `${m.id}（${m.offers.length} 家）` }));
    }, [input, catalog.data]);

    const add = (id: string) => {
        const v = simpleCanonical(id);
        if (!v || watch.includes(v)) return;
        setList([...watch, v].slice(0, 40));
        setInput("");
    };
    const remove = (id: string) => setList(watch.filter((w) => w !== id));
    const data = live.data;
    const rate = data?.usdCny || 7.1;

    return (
        <WorkspacePage>
            <PageHeader
                title="实时最低价"
                description="你盯的模型此刻在哪里调用最便宜。打开即拉取最新价格，分时段计价按此刻算，页面开着时每 5 分钟自动刷新；价格以官方页面为准。"
                actions={
                    <div className="flex flex-wrap items-center gap-3">
                        <span className="flex items-center gap-2 text-sm"><Switch size="small" checked={officialOnly} onChange={setOfficialOnly} />只看官方/云厂商</span>
                        <Button loading={live.isFetching} onClick={() => void live.refetch()}>立即刷新</Button>
                        <span className="text-xs text-foreground/50">更新于 {agoText(data?.generatedAt, now)}</span>
                    </div>
                }
            />
            {live.isError ? <Alert className="mt-3" type="error" showIcon title="价格拉取失败" description="需要能访问 OpenRouter、models.dev 或 jsDelivr。检查网络或代理后点“立即刷新”。" /> : null}
            {data?.mode === "daily" ? <Alert className="mt-3" type="warning" showIcon title="实时源暂时不可用，显示今日日报（不含分时价）" /> : null}
            {data?.mode === "cache" ? <Alert className="mt-3" type="warning" showIcon title={`网络不通，显示本机缓存数据（${data.sources.find((s) => s.name.startsWith("日报"))?.fetchedAt?.slice(0, 10) || "日期未知"}）`} /> : null}
            <div className="mt-4 flex flex-wrap items-center gap-2">
                <AutoComplete className="w-80 max-w-full" value={input} options={options} onChange={setInput} onSelect={(v: string) => add(v)} placeholder="搜模型加入关注，比如 deepseek、qwen、glm" onKeyDown={(e) => { if (e.key === "Enter" && input.trim() && !options.length) add(input); }} />
                <Button icon={<RotateCcw className="size-3.5" />} onClick={() => setList(null)}>恢复默认</Button>
                <span className="text-xs text-foreground/45">已关注 {watch.length} 个，最多 40 个</span>
            </div>
            {watch.length === 0 ? (
                <Empty className="mt-10" description="还没有关注的模型。搜一个加进来，或点“恢复默认”。" />
            ) : live.isLoading ? (
                <div className="mt-10 flex justify-center"><Spin /></div>
            ) : (
                <div className="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
                    {(data?.models || []).map((m) => <ModelCard key={m.id} m={m} rate={rate} now={now} mySites={mySites} onRemove={() => remove(m.id)} />)}
                </div>
            )}
            <p className="mt-4 text-xs leading-5 text-foreground/45">
                数据来源：{(data?.sources || []).map((s) => `${s.name}${s.ok ? "" : "（失败）"}`).join("、") || "—"}；美元汇率 {rate}。综合价按输入:输出 = 3:1 计算；涨跌按各平台常规价最低值与本机昨日快照比较。零价未确认、异常价与第三方中转站不参与。
            </p>
        </WorkspacePage>
    );
}
