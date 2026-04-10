import { computed, onBeforeUnmount, ref, watch, type ComputedRef, type Ref } from "vue";

import { ApiAbortError, api } from "../api";
import type { HistoryInterval, HistorySeries, ModuleKey, OptionItem, StatusTone, WatchlistItem } from "../types";

type StatusReporter = (message: string, tone: StatusTone) => void;

// 历史走势按标的和范围做本地缓存，避免用户切换区间或标的时重复请求同一段数据。
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

    // 取消正在进行的历史请求。
    function cancelInflightHistory(resetLoading = false): void {
        inflightController?.abort(new ApiAbortError("aborted"));
        inflightController = null;
        if (resetLoading) {
            historyLoading.value = false;
        }
    }

    // 按当前标的和范围加载图表数据，forceRefresh 用于绕过缓存刷新最新数据。
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
        // 静默刷新时优先保留当前图表，避免切换标的或区间时闪空。
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
            setStatus("正在加载市场走势…", "success");
        }

        try {
            const series = await api<HistorySeries>(`/api/history?itemId=${encodeURIComponent(item.id)}&interval=${encodeURIComponent(historyInterval.value)}`, {
                signal: controller.signal,
                timeoutMs: 12000,
            });
            // 请求返回时 controller 可能已经被新的请求替换，需要丢弃过期结果。
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
            if (keepCurrentSeries) {
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

    // 清空历史缓存，在标的增删改后强制重拉当前图表。
    function clearHistoryCache(): void {
        cancelInflightHistory(true);
        historyCache.clear();
        if (activeModule.value === "market") {
            void loadHistory(true, true);
        }
    }

    // 切换图表区间。
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
                // 离开市场模块时直接取消请求，避免无意义的后台更新。
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
