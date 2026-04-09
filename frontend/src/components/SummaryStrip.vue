<script setup lang="ts">
import { computed } from "vue";

import { formatMoney, formatPercent } from "../format";
import type { DashboardSummary, SummaryCard } from "../types";

const props = defineProps<{
    dashboard: DashboardSummary | null;
    itemCount: number;
    livePriceCount: number;
}>();

const cards = computed<SummaryCard[]>(() => {
    const value = props.dashboard;
    return [
        {
            label: "组合成本",
            value: formatMoney(value?.totalCost ?? 0),
            sub: `${value?.itemCount ?? 0} 个观察标的`,
            tone: "neutral",
        },
        {
            label: "当前市值",
            value: formatMoney(value?.totalValue ?? 0),
            sub: `${props.livePriceCount}/${props.itemCount} 个已同步`,
            tone: "neutral",
        },
        {
            label: "未实现盈亏",
            value: formatMoney(value?.totalPnL ?? 0, true),
            sub: formatPercent(value?.totalPnLPct ?? 0),
            tone: (value?.totalPnL ?? 0) >= 0 ? "rise" : "fall",
        },
        {
            label: "触发提醒",
            value: String(value?.triggeredAlerts ?? 0),
            sub: `盈利 ${value?.winCount ?? 0} / 亏损 ${value?.lossCount ?? 0}`,
            tone: (value?.triggeredAlerts ?? 0) > 0 ? "warn" : "neutral",
        },
    ];
});
</script>

<template>
    <section class="summary-strip">
        <article v-for="card in cards" :key="card.label" class="summary-card" :class="`tone-${card.tone}`">
            <span class="summary-label">{{ card.label }}</span>
            <strong class="summary-value">{{ card.value }}</strong>
            <span class="summary-sub">{{ card.sub }}</span>
        </article>
    </section>
</template>
