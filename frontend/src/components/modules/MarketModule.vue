<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Select from "primevue/select";

import PriceChart from "../PriceChart.vue";
import { historyIntervals } from "../../constants";
import { formatMoney, formatPercent, formatUnitPrice, intervalWindowLabel } from "../../format";
import type { HistoryInterval, HistorySeries, MarketMetricCard, OptionItem, WatchlistItem } from "../../types";

const props = defineProps<{
    selectedItem: WatchlistItem | null;
    selectedItemId: string;
    historyInterval: HistoryInterval;
    historyItemOptions: OptionItem<string>[];
    historySeries: HistorySeries | null;
    historyLoading: boolean;
    historyError: string;
}>();

const emit = defineEmits<{
    (event: "refresh"): void;
    (event: "update:selectedItemId", value: string): void;
    (event: "select-interval", value: HistoryInterval): void;
}>();

const selectedItemProxy = computed({
    get: () => props.selectedItemId,
    set: (value: string) => emit("update:selectedItemId", value),
});

const marketCards = computed<MarketMetricCard[]>(() => {
    const item = props.selectedItem;
    const series = props.historySeries;
    if (!item) {
        return [];
    }

    return [
        {
            label: `${item.name || item.symbol} · 最新`,
            value: formatUnitPrice(series?.endPrice || item.currentPrice || 0, item.currency),
            sub: `${item.market} · ${item.symbol}`,
            tone: "neutral",
        },
        {
            label: "区间变化",
            value: formatMoney(series?.change ?? item.change ?? 0, true),
            sub: formatPercent(series?.changePercent ?? item.changePercent ?? 0),
            tone: (series?.change ?? item.change ?? 0) >= 0 ? "rise" : "fall",
        },
        {
            label: "区间高点",
            value: formatUnitPrice(series?.high ?? item.dayHigh ?? 0, item.currency),
            sub: intervalWindowLabel(props.historyInterval),
            tone: "neutral",
        },
        {
            label: "区间低点",
            value: formatUnitPrice(series?.low ?? item.dayLow ?? 0, item.currency),
            sub: series?.source || item.quoteSource || "-",
            tone: "neutral",
        },
    ];
});
</script>

<template>
    <section class="module-content market-module">
        <div class="panel-header market-header">
            <div class="market-heading">
                <div>
                    <h3 class="title">行情</h3>
                </div>
                <div class="interval-row">
                    <button
                        v-for="entry in historyIntervals"
                        :key="entry.value"
                        class="interval-pill"
                        :class="{ active: historyInterval === entry.value }"
                        type="button"
                        @click="$emit('select-interval', entry.value)"
                    >
                        {{ entry.label }}
                    </button>
                </div>
            </div>
            <div class="toolbar-row">
                <Button text icon="pi pi-refresh" label="刷新" @click="$emit('refresh')" />
                <Select v-model="selectedItemProxy" :options="historyItemOptions" option-label="label" option-value="value" class="compact-select" />
            </div>
        </div>

        <div class="market-board">
            <div class="market-main">
                <PriceChart :series="historySeries" :loading="historyLoading" :error="historyError" />
            </div>

            <aside class="market-aside">
                <div class="market-metrics">
                    <article v-for="card in marketCards" :key="card.label" class="metric-card" :class="`tone-${card.tone}`">
                        <span class="metric-label">{{ card.label }}</span>
                        <strong class="metric-value">{{ card.value }}</strong>
                        <span class="metric-sub">{{ card.sub }}</span>
                    </article>
                </div>
            </aside>
        </div>
    </section>
</template>
