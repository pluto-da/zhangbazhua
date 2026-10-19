// 章八爪模块 · 模块注册表与开关 · 作者 长鱼茶汤茗
// 以后新增的自有功能都在这里登记一行，侧栏「章八爪」分组和模块中心会自动出现它。
import { BadgeDollarSign, Boxes, Feather, Palette, Plug2, Wallet, type LucideIcon } from "lucide-react";
import { create } from "zustand";
import { persist } from "zustand/middleware";

import { ZBZ_AUTHOR } from "./brand";

export type ZbzModuleKind = "系统" | "工具" | "技能" | "渠道";

export type ZbzModule = {
    id: string;
    name: string;
    kind: ZbzModuleKind;
    icon: LucideIcon;
    route?: string;
    version: string;
    author: string;
    description: string;
    where: string;
    defaultEnabled: boolean;
    locked?: boolean;
};

export const ZBZ_MODULES: ZbzModule[] = [
    {
        id: "price-radar", name: "模型比价雷达", kind: "工具", icon: BadgeDollarSign, route: "/zbz/prices", version: "1.0.0", author: ZBZ_AUTHOR, defaultEnabled: true,
        description: "每天汇总 models.dev、OpenRouter、LiteLLM 三大公开价格库，找出同一个模型最便宜的调用渠道，并和你正在用的渠道对比。",
        where: "web/src/zbz/pages/price-radar.tsx · backend/internal/zbz/pricing.go · scripts/zbz/price_radar.py",
    },
    {
        id: "recharge", name: "充值中心", kind: "工具", icon: Wallet, route: "/zbz/recharge", version: "1.0.0", author: ZBZ_AUTHOR, defaultEnabled: true,
        description: "一处查看各平台余额、一处打开官方充值页、本地记账；推荐用聚合渠道实现“只充一个地方”。不代收代付，不保存任何支付信息。",
        where: "web/src/zbz/pages/recharge.tsx · backend/internal/zbz/billing.go、ledger.go",
    },
    {
        id: "drama-writer", name: "结构主义短剧编剧", kind: "技能", icon: Feather, version: "1.0.0", author: ZBZ_AUTHOR, defaultEnabled: true, locked: true,
        description: "内置技能：先建核心二元对立、符号方阵、行动元、镜像配对和核心符号表，再写分集与对白。在画布助手里直接用。",
        where: "backend/internal/skills/seed/skills.json · backend/internal/prompts/agent-system-policy.md",
    },
    {
        id: "mimo", name: "小米 MiMo 渠道", kind: "渠道", icon: Plug2, version: "1.0.0", author: ZBZ_AUTHOR, defaultEnabled: true, locked: true,
        description: "模型配置里可直接添加小米 MiMo（OpenAI 兼容），支持 mimo-v2.6 系列文本模型与 TTS。",
        where: "plugin-packages/xiaomi-mimo-chat · generate-catalog.mjs",
    },
    {
        id: "brand", name: "品牌与外观", kind: "系统", icon: Palette, version: "1.0.0", author: ZBZ_AUTHOR, defaultEnabled: true, locked: true,
        description: "软件名、图标、窗口标题；打包时自动把界面里的产品名统一成章八爪（scripts/zbz/rebrand.py），并审计无遗留。",
        where: "scripts/zbz/rebrand.py · assets/app-icon.png · web/public/beef-logo.png",
    },
];

export const ZBZ_CENTER = { id: "center", name: "模块中心", icon: Boxes, route: "/zbz" };

type ZbzModuleStore = {
    enabled: Record<string, boolean>;
    setEnabled: (id: string, value: boolean) => void;
};

export const useZbzModules = create<ZbzModuleStore>()(
    persist(
        (set) => ({
            enabled: {},
            setEnabled: (id, value) => set((state) => ({ enabled: { ...state.enabled, [id]: value } })),
        }),
        { name: "zbz:modules" },
    ),
);

export function isZbzModuleEnabled(module: ZbzModule, enabled: Record<string, boolean>) {
    if (module.locked) return true;
    return enabled[module.id] ?? module.defaultEnabled;
}

/** 侧栏「章八爪」分组：模块中心 + 已开启且有页面的模块。 */
export function zbzNavGroup() {
    const enabled = useZbzModules.getState().enabled;
    const items = [
        { id: "zbz:center", title: ZBZ_CENTER.name, icon: ZBZ_CENTER.icon, to: ZBZ_CENTER.route },
        ...ZBZ_MODULES.filter((m) => m.route && isZbzModuleEnabled(m, enabled)).map((m) => ({ id: `zbz:${m.id}`, title: m.name, icon: m.icon, to: m.route as string })),
    ];
    return { heading: "章八爪", items };
}
