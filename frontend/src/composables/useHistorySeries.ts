import { computed, onBeforeUnmount, ref, watch, type ComputedRef, type Ref } from "vue";

import { ApiAbortError, api } from "../api";
import type { HistoryInterval, HistorySeries, ModuleKey, OptionItem, StatusTone, WatchlistItem } from "../types";

type StatusReporter = (message: string, tone: StatusTone) => void;

// 历史走势按标的和周期做本地缓存，避免用户切换标签时重复请求同一段数据。
export function useHistorySeries(items: Ref<WatchlistItem[]>, selectedItem: ComputedRef<WatchlistItem | null>, activeModule: Ref<ModuleKey>, setStatus: StatusReporter) {
    const historyInterval = ref<HistoryInterval>("day");
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

    function cancelInflightHistory(resetLoading = false): void {
        inflightController?.abort(new ApiAbortError("aborted"));
        inflightController = null;
        if (resetLoading) {
            historyLoading.value = false;
        }
    }

    async function loadHistory(silent = false): Promise<void> {
        const item = selectedItem.value;
        if (!item) {
            cancelInflightHistory(true);
            historySeries.value = null;
            historyError.value = "";
            return;
        }

        const key = `${item.id}:${historyInterval.value}`;
        if (historyCache.has(key)) {
            cancelInflightHistory(true);
            historySeries.value = historyCache.get(key) ?? null;
            historyError.value = "";
            return;
        }

        cancelInflightHistory();
        const controller = new AbortController();
        inflightController = controller;
        historyLoading.value = true;
        historyError.value = "";
        if (!silent) {
            setStatus("正在加载市场走势…", "success");
        }

        try {
            const series = await api<HistorySeries>(`/api/history?itemId=${encodeURIComponent(item.id)}&interval=${encodeURIComponent(historyInterval.value)}`, {
                signal: controller.signal,
                timeoutMs: 12000,
            });
            if (inflightController !== controller) {
                return;
            }
            historyCache.set(key, series);
            historySeries.value = series;
            historyError.value = "";
            if (!silent) {
                setStatus("市场走势已更新。", "success");
            }
        } catch (error) {
            if (error instanceof ApiAbortError) {
                return;
            }
            historyError.value = error instanceof Error ? error.message : "走势加载失败";
            historySeries.value = null;
            setStatus(historyError.value, "error");
        } finally {
            if (inflightController === controller) {
                inflightController = null;
                historyLoading.value = false;
            }
        }
    }

    function clearHistoryCache(): void {
        cancelInflightHistory(true);
        historyCache.clear();
        if (activeModule.value === "market") {
            void loadHistory(true);
        }
    }

    function selectHistoryInterval(next: HistoryInterval): void {
        if (historyInterval.value === next) {
            return;
        }
        historyInterval.value = next;
        void loadHistory(true);
    }

    watch(activeModule, (nextModule) => {
        if (nextModule !== "market") {
            cancelInflightHistory(true);
        }
    });

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
