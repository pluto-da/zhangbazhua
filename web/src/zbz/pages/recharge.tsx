// 章八爪模块 · 充值中心 · 作者 长鱼茶汤茗
// 统一看余额、统一去充值、本地记账。不代收代付，不保存任何支付信息；Key 只在本机前后端之间传递。
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, App, Button, Input, InputNumber, Modal, Popconfirm, Table, Tag } from "antd";
import { useMemo, useState } from "react";

import { PageHeader, WorkspacePage } from "@/components/layout/workspace-page";
import { useConfigStore } from "@/stores/use-config-store";

import { addLedger, deleteLedger, listLedger, queryBalance, type BalanceResult } from "../api";
import { openExternal } from "../open-external";

type Platform = { id: string; name: string; hosts: RegExp; console: string; recharge: string; note?: string };

// 官方控制台 / 充值页。只做跳转，充值在官方页面由你本人完成。
const PLATFORMS: Platform[] = [
    { id: "deepseek", name: "DeepSeek", hosts: /deepseek\.com/, console: "https://platform.deepseek.com/usage", recharge: "https://platform.deepseek.com/top_up" },
    { id: "moonshot", name: "月之暗面 Kimi", hosts: /moonshot\.(cn|ai)/, console: "https://platform.moonshot.cn/console/account", recharge: "https://platform.moonshot.cn/console/pay" },
    { id: "siliconflow", name: "硅基流动", hosts: /siliconflow/, console: "https://cloud.siliconflow.cn/account/ak", recharge: "https://cloud.siliconflow.cn/expensebill", note: "聚合渠道" },
    { id: "openrouter", name: "OpenRouter", hosts: /openrouter\.ai/, console: "https://openrouter.ai/settings/credits", recharge: "https://openrouter.ai/settings/credits", note: "聚合渠道" },
    { id: "bailian", name: "阿里云百炼", hosts: /dashscope|aliyuncs/, console: "https://bailian.console.aliyun.com/", recharge: "https://usercenter2.aliyun.com/finance/fund-management/recharge" },
    { id: "zhipu", name: "智谱 BigModel", hosts: /bigmodel\.cn/, console: "https://open.bigmodel.cn/finance/overview", recharge: "https://open.bigmodel.cn/finance/pay" },
    { id: "mimo", name: "小米 MiMo", hosts: /xiaomimimo/, console: "https://platform.xiaomimimo.com/", recharge: "https://platform.xiaomimimo.com/" },
    { id: "volcengine", name: "火山方舟", hosts: /volces\.com|volcengine/, console: "https://console.volcengine.com/ark", recharge: "https://console.volcengine.com/finance/fund/recharge" },
];

const LOW_KEY = "zbz:recharge:low";

function hostOf(url: string) {
    try {
        return new URL(url).hostname;
    } catch {
        return "";
    }
}

export default function RechargePage() {
    const { message } = App.useApp();
    const qc = useQueryClient();
    const channels = useConfigStore((s) => s.config.channels);
    const [balances, setBalances] = useState<Record<string, BalanceResult & { loading?: boolean }>>({});
    const [low, setLow] = useState<number>(() => Number(localStorage.getItem(LOW_KEY) || 10));
    const [recording, setRecording] = useState<string | null>(null);
    const [amount, setAmount] = useState<number | null>(null);
    const [note, setNote] = useState("");

    const rows = useMemo(() => PLATFORMS.map((p) => {
        const channel = (channels || []).find((c) => p.hosts.test(hostOf(c.baseUrl)) && c.apiKey);
        return { ...p, channel };
    }), [channels]);

    const ledger = useQuery({ queryKey: ["zbz-ledger"], queryFn: listLedger });
    const monthTotal = useMemo(() => {
        const now = new Date();
        return (ledger.data?.entries || []).filter((e) => { const d = new Date(e.at); return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && e.currency === "CNY"; }).reduce((s, e) => s + e.amount, 0);
    }, [ledger.data]);

    async function refreshAll() {
        const targets = rows.filter((r) => r.channel);
        if (!targets.length) {
            message.info("还没有配置这些平台的渠道 Key，先到“模型配置”里添加");
            return;
        }
        setBalances((b) => {
            const next = { ...b };
            for (const t of targets) next[t.id] = { provider: t.id, supported: true, balance: 0, currency: "", ...(b[t.id] || {}), loading: true };
            return next;
        });
        await Promise.all(targets.map(async (t) => {
            try {
                const r = await queryBalance(t.channel!.baseUrl, t.channel!.apiKey);
                setBalances((b) => ({ ...b, [t.id]: r }));
            } catch (e) {
                setBalances((b) => ({ ...b, [t.id]: { provider: t.id, supported: true, balance: 0, currency: "", error: e instanceof Error ? e.message : "查询失败" } }));
            }
        }));
    }

    const lowCount = Object.values(balances).filter((b) => !b.error && b.supported && b.currency === "CNY" && b.balance < low).length;
    const totalCny = Object.values(balances).filter((b) => !b.error && b.supported).reduce((s, b) => s + (b.currency === "USD" ? b.balance * 7.1 : b.balance), 0);

    return (
        <WorkspacePage>
            <PageHeader title="充值中心" description="各平台余额一处看，充值一处进，账本记在本机。充值在官方页面由你本人完成，软件不经手钱、不保存任何支付信息。" actions={<Button type="primary" onClick={() => void refreshAll()}>查询全部余额</Button>} />
            <div className="mt-4 grid gap-3 sm:grid-cols-3">
                <div className="rounded-[var(--r-md)] border border-foreground/10 p-4"><div className="text-xs text-foreground/50">可查询平台总余额（约）</div><div className="mt-1 text-2xl font-semibold">¥{totalCny.toFixed(2)}</div></div>
                <div className="rounded-[var(--r-md)] border border-foreground/10 p-4"><div className="text-xs text-foreground/50">本月已记充值</div><div className="mt-1 text-2xl font-semibold">¥{monthTotal.toFixed(2)}</div></div>
                <div className="rounded-[var(--r-md)] border border-foreground/10 p-4"><div className="text-xs text-foreground/50">余额不足的平台（低于 ¥{low}）</div><div className="mt-1 flex items-center gap-2 text-2xl font-semibold">{lowCount}<InputNumber size="small" min={0} value={low} onChange={(v) => { const n = Number(v || 0); setLow(n); localStorage.setItem(LOW_KEY, String(n)); }} addonBefore="提醒线 ¥" /></div></div>
            </div>
            <Alert className="mt-4" type="info" showIcon title="想只充一个地方？" description="用聚合渠道：OpenRouter 一次充值可调用数百个国外模型，硅基流动一次充值可调用大量国产与开源模型。先到“模型比价雷达”看看你常用的模型在聚合渠道贵不贵，再决定。" />
            <Table
                className="mt-4"
                size="small"
                rowKey="id"
                pagination={false}
                dataSource={rows}
                columns={[
                    { title: "平台", render: (_, r) => <span className="font-medium">{r.name}{r.note ? <Tag className="ml-2" color="purple">{r.note}</Tag> : null}</span> },
                    { title: "渠道", render: (_, r) => (r.channel ? <Tag color="blue">{r.channel.name || "已配置"}</Tag> : <span className="text-foreground/40">未配置</span>) },
                    {
                        title: "余额",
                        render: (_, r) => {
                            const b = balances[r.id] as (BalanceResult & { loading?: boolean }) | undefined;
                            if (!r.channel) return "—";
                            if (!b) return <span className="text-foreground/40">点“查询全部余额”</span>;
                            if (b.loading) return "查询中…";
                            if (!b.supported || b.error) return <span className="text-foreground/50">{b.error || "需到官网查看"}</span>;
                            const lowNow = b.currency === "CNY" && b.balance < low;
                            return <span className={lowNow ? "font-semibold text-red-500" : "font-semibold"}>{b.currency === "USD" ? "$" : "¥"}{b.balance.toFixed(2)}{lowNow ? " · 该充了" : ""}</span>;
                        },
                    },
                    {
                        title: "操作",
                        render: (_, r) => (
                            <div className="flex flex-wrap gap-2">
                                <Button size="small" type="primary" onClick={() => openExternal(r.recharge)}>去充值</Button>
                                <Button size="small" onClick={() => openExternal(r.console)}>控制台</Button>
                                <Button size="small" onClick={() => { setRecording(r.name); setAmount(null); setNote(""); }}>记一笔</Button>
                            </div>
                        ),
                    },
                ]}
            />
            <h2 className="mt-6 text-base font-semibold">充值账本</h2>
            <Table
                className="mt-2"
                size="small"
                rowKey="id"
                loading={ledger.isLoading}
                dataSource={[...(ledger.data?.entries || [])].reverse()}
                pagination={{ pageSize: 10, showSizeChanger: false }}
                locale={{ emptyText: "还没有记录。在官方页面充完值后，点对应平台的“记一笔”。" }}
                columns={[
                    { title: "时间", render: (_, e) => new Date(e.at).toLocaleString() },
                    { title: "平台", dataIndex: "platform" },
                    { title: "金额", render: (_, e) => `${e.currency === "USD" ? "$" : "¥"}${e.amount.toFixed(2)}` },
                    { title: "备注", dataIndex: "note" },
                    { title: "", render: (_, e) => <Popconfirm title="删除这条记录？" onConfirm={() => void deleteLedger(e.id).then(() => qc.invalidateQueries({ queryKey: ["zbz-ledger"] }))}><Button size="small" type="link" danger>删除</Button></Popconfirm> },
                ]}
            />
            <Modal
                open={recording !== null}
                title={`记一笔：${recording || ""}`}
                okText="保存"
                cancelText="取消"
                onCancel={() => setRecording(null)}
                onOk={async () => {
                    if (!recording || !amount || amount <= 0) {
                        message.warning("请填写大于 0 的金额");
                        return;
                    }
                    try {
                        await addLedger({ platform: recording, amount, currency: recording === "OpenRouter" ? "USD" : "CNY", note });
                        setRecording(null);
                        await qc.invalidateQueries({ queryKey: ["zbz-ledger"] });
                        message.success("已记账");
                    } catch (e) {
                        message.error(e instanceof Error ? e.message : "保存失败");
                    }
                }}
            >
                <div className="flex flex-col gap-3">
                    <InputNumber className="w-full" min={0} precision={2} value={amount} onChange={(v) => setAmount(v === null ? null : Number(v))} addonBefore={recording === "OpenRouter" ? "$" : "¥"} placeholder="充值金额" />
                    <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder="备注（可选），比如：做第 3 集配音前充的" />
                </div>
            </Modal>
        </WorkspacePage>
    );
}
