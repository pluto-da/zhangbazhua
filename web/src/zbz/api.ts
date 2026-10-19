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

export type LiveOffer = {
    provider: string;
    providerName: string;
    tier: string;
    source: string;
    url: string;
    china: boolean;
    rawModel?: string;
    inputUsd: number;
    outputUsd: number;
    baseInputUsd: number;
    baseOutputUsd: number;
    timed: boolean;
    nextChangeAt?: string;
    nextBlendUsd?: number;
};

export type LiveModel = {
    id: string;
    best: LiveOffer | null;
    offers: LiveOffer[];
    offerCount: number;
    baseBestBlendUsd: number;
    yesterdayBlendUsd: number | null;
    changePct: number | null;
    isNewLow: boolean;
};

export type LiveResponse = {
    generatedAt: string;
    mode: "live" | "daily" | "cache";
    usdCny: number;
    sources: Array<{ name: string; ok: boolean; fetchedAt?: string; error?: string }>;
    models: LiveModel[];
};

export function fetchLive(models: string[], officialOnly: boolean) {
    const q = new URLSearchParams({ models: models.join(","), official: officialOnly ? "1" : "0" });
    return http.get<LiveResponse>(`/zbz/live?${q.toString()}`, { timeout: 90_000 });
}
