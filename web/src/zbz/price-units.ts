// 章八爪模块 · 计费单位与中转站风险的共用工具 · 作者 长鱼茶汤茗
export type Unit = "token" | "second" | "image" | "call";

export type UnitOffer = {
    unit?: Unit | string;
    inputUsd: number;
    outputUsd: number;
    unitUsd?: number;
    tier: string;
    group?: string;
    groupRatio?: number;
    minGroup?: string;
    minGroupRatio?: number;
    rechargePrice?: number;
    rechargeAssumed?: boolean;
    assumedSeconds?: number;
    res?: string;
};

/** 与后端 canonical 同一套规则（简化版）：小写、去平台前缀、去 :free 等后缀、数字间的点换横线。 */
export function canonicalId(id: string) {
    let s = id.trim().toLowerCase();
    s = s.split("/").pop() || "";
    s = s.replace(/:(free|beta|extended|thinking|nitro|floor)$/, "");
    s = s.replace(/[-_@](20\d{2}-?\d{2}-?\d{2}|\d{6}|\d{4})$/, "");
    s = s.replace(/(\d)\.(?=\d)/g, "$1-");
    return s;
}

export function unitOf(o: UnitOffer): Unit {
    return (o.unit as Unit) || "token";
}

/** 同一单位内可比较的单价（美元等值）：token 用输入:输出加权综合价。 */
export function unitPrice(o: UnitOffer, ratio = 3) {
    if (unitOf(o) === "token") return (ratio * o.inputUsd + o.outputUsd) / (ratio + 1);
    return o.unitUsd || 0;
}

export function yuan(usd: number, rate: number) {
    const v = usd * rate;
    if (v === 0) return "免费";
    if (v < 0.01) return `¥${v.toFixed(4)}`;
    return v < 0.1 ? `¥${v.toFixed(3)}` : `¥${v.toFixed(2)}`;
}

export const UNIT_LABEL: Record<Unit, string> = { token: "/百万 tokens", second: "/秒", image: "/张", call: "/次" };

/** 主价格文案：视频带 5 秒成本，按次带“每次”。 */
export function priceText(o: UnitOffer, rate: number, ratio = 3) {
    const u = unitOf(o);
    const p = unitPrice(o, ratio);
    if (u === "second") return `${yuan(p, rate)}/秒 · 5 秒约 ${yuan(p * 5, rate)}`;
    if (u === "token") return `${yuan(p, rate)}/百万 tokens`;
    return `${yuan(p, rate)}${UNIT_LABEL[u]}`;
}

export const RISK_TEXT = "中转站价格更低，但可能来自非官方渠道，存在稳定性、合规与数据安全风险；重要项目建议用官方或云厂商，充值从小额开始。";

/** 中转站风险徽章。officialMin 为同单位官方/云厂商最低单价（美元等值），用于识别“远低于官方”。 */
export function riskBadges(o: UnitOffer, officialMin?: number, ratio = 3): Array<{ text: string; color?: string }> {
    if (o.tier !== "中转站") return [];
    const out: Array<{ text: string; color?: string }> = [];
    const g = o.group || "";
    if (/官转|官方/.test(g)) out.push({ text: "官方转发", color: "green" });
    if (/逆|特价|限时|体验|免费/.test(g)) out.push({ text: "低价分组", color: "orange" });
    if (o.rechargeAssumed) out.push({ text: "充值价未知", color: "orange" });
    if (officialMin && unitPrice(o, ratio) < officialMin * 0.2) out.push({ text: "远低于官方，多为非官方渠道", color: "red" });
    return out;
}

export function relayDetail(o: UnitOffer) {
    if (o.tier !== "中转站") return "";
    const parts: string[] = [];
    if (o.group) parts.push(`分组 ${o.group}${o.groupRatio != null ? ` ×${o.groupRatio}` : ""}`);
    if (o.rechargePrice) parts.push(`${o.rechargePrice} 元 = 1 美元额度`);
    if (o.assumedSeconds) parts.push(`按每条 ${o.assumedSeconds} 秒折算`);
    if (o.minGroup && o.minGroup !== o.group && o.minGroupRatio != null) parts.push(`最低分组 ${o.minGroup} ×${o.minGroupRatio}`);
    return parts.join(" · ");
}
