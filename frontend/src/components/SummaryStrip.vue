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
    const currencySymbol = (code: string): string => {
        switch (code) {
            case "CNY":
                return "¥";
            case "HKD":
                return "HK$";
            case "USD":
                return "$";
            default:
                return "";
        }
    };
    const currency = currencySymbol(value?.displayCurrency || "");
    return [
        {
            label: "组合成本",
            value: formatMoney(value?.totalCost ?? 0),
            sub: `${value?.itemCount ?? 0} 个标的`,
            tone: "neutral",
            currency,
        },
        {
            label: "当前资产",
            value: formatMoney(value?.totalValue ?? 0),
            sub: `${props.livePriceCount}/${props.itemCount} 个已同步`,
            tone: "neutral",
            currency,
        },
        {
            label: "未实现盈亏",
            value: formatMoney(value?.totalPnL ?? 0, true),
            sub: formatPercent(value?.totalPnLPct ?? 0),
            tone: (value?.totalPnL ?? 0) >= 0 ? "rise" : "fall",
            currency,
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
        <article v-for="card in cards" :key="card.label" class="summary-card" :data-tone="card.tone">
            <span class="summary-label">{{ card.label }}</span>
            <strong class="summary-value">
                <span v-if="card.currency" class="summary-currency">{{ card.currency }}</span
                >{{ card.value }}
            </strong>
            <span class="summary-sub">{{ card.sub }}</span>
        </article>
    </section>
</template>
