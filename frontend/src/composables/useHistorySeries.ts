import { computed, onBeforeUnmount, ref, watch, type ComputedRef, type Ref } from "vue";

import { ApiAbortError, api } from "../api";
import { translate } from "../i18n";
import type { HistoryInterval, HistorySeries, ModuleKey, OptionItem, StatusTone, WatchlistItem } from "../types";

type StatusReporter = (message: string, tone: StatusTone) => void;

// Cache historical data locally by item and interval to avoid re-fetching the same data when users switch intervals or items.
export function useHistorySeries(items: Ref<WatchlistItem[]>, selectedItem: ComputedRef<WatchlistItem | null>, activeModule: Ref<ModuleKey>, setStatus: StatusReporter) {
    const historyInterval = ref<HistoryInterval>("1h");
    const historySeries = ref<HistorySeries | null>(null);
    const historyLoading = ref(false);
    const historyError = ref("");
    const historyCache = new Map<string, HistorySeries>();
    let inflightController: AbortController | null = null;

    const historyItemOptions = computed<OptionItem<string>[]>(() =>
        items.value.map((item) => ({
            label: `${item.name || item.symbol} · ${item.symbol}`,
            value: item.id,
        })),
    );

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
        if (!forceRefresh && historyCache.has(key)) {
            cancelInflightHistory(true);
            historySeries.value = historyCache.get(key) ?? null;
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
            const series = await api<HistorySeries>(`/api/history?itemId=${encodeURIComponent(item.id)}&interval=${encodeURIComponent(historyInterval.value)}`, {
                signal: controller.signal,
                timeoutMs: 12000,
            });
            // When the response arrives, the controller may have been replaced by a newer request; discard stale results.
            if (inflightController !== controller) {
                return;
            }
            historyCache.set(key, series);
            historySeries.value = series;
            historyError.value = "";
            if (!silent) {
                setStatus(translate("history.updated"), "success");
            }
        } catch (error) {
            if (error instanceof ApiAbortError) {
                return;
            }
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
        if (activeModule.value === "market") {
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
            if (activeModule.value !== "market" || !selectedItem.value) {
                // When leaving the market module, cancel the request directly to avoid unnecessary background updates.
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
        historyItemOptions,
        loadHistory,
        clearHistoryCache,
        selectHistoryInterval,
    };
}
