// 章八爪模块 · 模块中心 · 作者 长鱼茶汤茗
import { Button, Switch, Tag } from "antd";
import { useNavigate } from "react-router";

import { PageHeader, WorkspacePage } from "@/components/layout/workspace-page";

import { ZBZ_NAME, ZBZ_RELEASES, ZBZ_UPSTREAM } from "../brand";
import { isZbzModuleEnabled, useZbzModules, ZBZ_MODULES } from "../modules";
import { openExternal } from "../open-external";

export default function ZbzModulesPage() {
    const navigate = useNavigate();
    const enabled = useZbzModules((s) => s.enabled);
    const setEnabled = useZbzModules((s) => s.setEnabled);

    return (
        <WorkspacePage>
            <PageHeader title="模块中心" description={`${ZBZ_NAME}自己开发的功能都在这里，可单独开关；和上游 ${ZBZ_UPSTREAM} 的代码分开存放，跟随作者更新时互不干扰。`} actions={<Button onClick={() => openExternal(ZBZ_RELEASES)}>下载新版</Button>} />
            <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                {ZBZ_MODULES.map((m) => {
                    const on = isZbzModuleEnabled(m, enabled);
                    const Icon = m.icon;
                    return (
                        <section key={m.id} className="flex flex-col gap-3 rounded-[var(--r-md)] border border-foreground/10 bg-foreground/[0.02] p-4">
                            <div className="flex items-center gap-3">
                                <Icon className="size-5 shrink-0" strokeWidth={1.7} />
                                <h2 className="min-w-0 flex-1 truncate text-base font-semibold">{m.name}</h2>
                                <Tag>{m.kind}</Tag>
                                <Switch size="small" checked={on} disabled={m.locked} onChange={(v) => setEnabled(m.id, v)} />
                            </div>
                            <p className="text-sm leading-6 text-foreground/70">{m.description}</p>
                            <p className="text-xs leading-5 text-foreground/45">v{m.version} · 作者 {m.author}{m.locked ? " · 核心模块，常开" : ""}</p>
                            <p className="break-all text-xs leading-5 text-foreground/35">代码位置：{m.where}</p>
                            {m.route && on ? <div><Button size="small" type="primary" onClick={() => navigate(m.route as string)}>打开</Button></div> : null}
                        </section>
                    );
                })}
            </div>
        </WorkspacePage>
    );
}
