// 章八爪模块 · 前端接口 · 作者 长鱼茶汤茗
import { http } from "@/services/api/request";

export type PriceOffer = {
    provider: string;
    providerName: string;
    source: string;
    tier: string;
    inputUsd: number;
    outputUsd: number;
    cacheReadUsd?: number;
    url: string;
    china: boolean;
};

export type PriceModel = {
    id: string;
    name: string;
    modalities: string[];
    context: number;
    offers: PriceOffer[];
};

export type PricesFile = {
    generatedAt: string;
    usdCny: number;
    fx?: string;
    sources: Array<{ name: string; url: string; ok: boolean; count?: number }>;
    models: PriceModel[];
    degraded?: boolean;
};

export type PricesResponse = { prices: PricesFile; source: string; fetchedAt: string; stale: boolean; note: string };

export function fetchPrices(refresh = false) {
    return http.get<PricesResponse>(`/zbz/prices${refresh ? "?refresh=1" : ""}`, { timeout: 90_000 });
}

export type BalanceResult = { provider: string; supported: boolean; balance: number; currency: string; detail?: string; error?: string };

export function queryBalance(baseUrl: string, apiKey: string) {
    return http.post<BalanceResult>("/zbz/balance", { baseUrl, apiKey }, { timeout: 15_000 });
}

export type LedgerEntry = { id: string; platform: string; amount: number; currency: string; note: string; at: string };

export function listLedger() {
    return http.get<{ entries: LedgerEntry[] }>("/zbz/ledger");
}

export function addLedger(entry: Omit<LedgerEntry, "id" | "at"> & { at?: string }) {
    return http.post<LedgerEntry>("/zbz/ledger", entry);
}

export function deleteLedger(id: string) {
    return http.delete<{ deleted: string }>(`/zbz/ledger/${encodeURIComponent(id)}`);
}
