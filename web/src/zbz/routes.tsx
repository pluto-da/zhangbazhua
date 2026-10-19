// 章八爪模块 · 路由集合 · 作者 长鱼茶汤茗
import { lazy, Suspense, type ReactNode } from "react";

import { WorkspaceRouteLoader } from "@/components/ui/aceternity/full-screen-loader";

const ModulesPage = lazy(() => import("./pages/modules-page"));
const PriceRadarPage = lazy(() => import("./pages/price-radar"));
const RechargePage = lazy(() => import("./pages/recharge"));
const LivePricesPage = lazy(() => import("./pages/live-prices"));

function deferred(element: ReactNode) {
    return <Suspense fallback={<WorkspaceRouteLoader />}>{element}</Suspense>;
}

export const zbzRoutes = [
    { path: "/zbz", element: deferred(<ModulesPage />) },
    { path: "/zbz/prices", element: deferred(<PriceRadarPage />) },
    { path: "/zbz/recharge", element: deferred(<RechargePage />) },
    { path: "/zbz/live", element: deferred(<LivePricesPage />) },
];
