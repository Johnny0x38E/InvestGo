import { computed, onBeforeUnmount, ref, watch, type ComputedRef, type Ref } from "vue";

import { ApiAbortError, api } from "../api";
import { translate } from "../i18n";
import type { HistoryInterval, HistorySeries, ModuleKey, StatusTone, WatchlistItem } from "../types";

type StatusReporter = (message: string, tone: StatusTone) => void;

// Cache historical data locally by item and interval to avoid re-fetching the same data when users switch intervals or items.
interface CachedHistorySeries {
    series: HistorySeries;
    expiresAt: number; // Date.now() ms
}
const HISTORY_CACHE_MAX_SIZE = 60; // max items x intervals

export function useHistorySeries(items: Ref<WatchlistItem[]>, selectedItem: ComputedRef<WatchlistItem | null>, activeModule: Ref<ModuleKey>, setStatus: StatusReporter) {
    const historyInterval = ref<HistoryInterval>("1d");
    const historySeries = ref<HistorySeries | null>(null);
    const historyLoading = ref(false);
    const historyError = ref("");
    const historyCache = new Map<string, CachedHistorySeries>();
    let inflightController: AbortController | null = null;


    // Cancel any in-flight history request.
    function cancelInflightHistory(resetLoading = false): void {
        inflightController?.abort(new ApiAbortError("aborted"));
        inflightController = null;
        if (resetLoading) {
            historyLoading.value = false;
        }
    }

    // Load chart data for the current item and interval; forceRefresh bypasses the cache to fetch fresh data.
    async function loadHistory(silent = false, forceRefresh = false): Promise<void> {
        const item = selectedItem.value;
        if (!item) {
            cancelInflightHistory(true);
            historySeries.value = null;
            historyError.value = "";
            return;
        }

        const key = `${item.id}:${historyInterval.value}`;
        const keepCurrentSeries = silent && Boolean(historySeries.value);
        // During silent refresh, prefer keeping the current chart to avoid a blank flash when switching items or intervals.
        const cached = historyCache.get(key);
        if (!forceRefresh && cached && Date.now() < cached.expiresAt) {
            cancelInflightHistory(true);
            // The series is served from the in-process memory cache, so mark it as
            // a cache hit regardless of the original backend Cached value (which
            // was false when the data was first fetched live from the provider).
            historySeries.value = { ...cached.series, cached: true };
            historyError.value = "";
            return;
        }

        cancelInflightHistory();
        const controller = new AbortController();
        inflightController = controller;
        if (!keepCurrentSeries) {
            historyLoading.value = true;
        }
        historyError.value = "";
        if (!silent) {
            setStatus(translate("history.loading"), "success");
        }

        try {
            const params = new URLSearchParams({
                itemId: item.id,
                interval: historyInterval.value,
            });
            if (forceRefresh) {
                params.set("force", "1");
            }
            const series = await api<HistorySeries>(`/api/history?${params.toString()}`, {
                signal: controller.signal,
                timeoutMs: 12000,
            });
            // When the response arrives, the controller may have been replaced by a newer request; discard stale results.
            if (inflightController !== controller) {
                return;
            }
            const expiresAt = series.cacheExpiresAt
                ? new Date(series.cacheExpiresAt).getTime()
                : Date.now() + 5 * 60 * 1000; // 5-min fallback
            historyCache.set(key, { series, expiresAt });
            // Evict oldest entries if over limit
            if (historyCache.size > HISTORY_CACHE_MAX_SIZE) {
                const oldest = historyCache.keys().next().value;
                if (oldest !== undefined) historyCache.delete(oldest);
            }
            historySeries.value = series;
            historyError.value = "";
            if (!silent) {
                setStatus(translate("history.updated"), "success");
            }
        } catch (error) {
            if (error instanceof ApiAbortError) {
                return;
            }
            // Silent refresh failures should not blank out the chart the user is
            // already reading. Keep the previous series until an explicit load
            // replaces it with a fresh result.
            if (keepCurrentSeries) {
                return;
            }
            historyError.value = error instanceof Error ? error.message : translate("history.loadFailed");
            historySeries.value = null;
            setStatus(historyError.value, "error");
        } finally {
            if (inflightController === controller) {
                inflightController = null;
                historyLoading.value = false;
            }
        }
    }

    // Clear the history cache and force-reload the current chart after items are added, deleted, or updated.
    function clearHistoryCache(): void {
        cancelInflightHistory(true);
        historyCache.clear();
        if (activeModule.value === "watchlist") {
            void loadHistory(true, true);
        }
    }

    // Switch the chart interval.
    function selectHistoryInterval(next: HistoryInterval): void {
        if (historyInterval.value === next) {
            return;
        }
        historyInterval.value = next;
        void loadHistory(true);
    }

    watch(
        () => [activeModule.value, selectedItem.value?.id ?? "", historyInterval.value] as const,
        () => {
            if (activeModule.value !== "watchlist" || !selectedItem.value) {
                // When leaving the watchlist module, cancel the request directly to avoid unnecessary background updates.
                cancelInflightHistory(true);
                return;
            }
            void loadHistory(true);
        },
        { immediate: true },
    );

    onBeforeUnmount(() => {
        cancelInflightHistory(true);
    });

    return {
        historyInterval,
        historySeries,
        historyLoading,
        historyError,
        loadHistory,
        clearHistoryCache,
        selectHistoryInterval,
    };
}
