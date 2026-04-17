<script setup lang="ts">
import { computed } from "vue";

import { formatMoney, formatPercent } from "../format";
import { useI18n } from "../i18n";
import type { DashboardSummary, SummaryCard } from "../types";

const props = defineProps<{
    dashboard: DashboardSummary | null;
    itemCount: number;
    livePriceCount: number;
}>();

const { t } = useI18n();

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
            label: t("summary.totalCost"),
            value: formatMoney(value?.totalCost ?? 0),
            sub: t("summary.itemsSub", { count: value?.itemCount ?? 0 }),
            tone: "neutral",
            currency,
        },
        {
            label: t("summary.currentValue"),
            value: formatMoney(value?.totalValue ?? 0),
            sub: t("summary.syncedSub", { live: props.livePriceCount, total: props.itemCount }),
            tone: "neutral",
            currency,
        },
        {
            label: t("summary.positionPnL"),
            value: formatMoney(value?.totalPnL ?? 0, true),
            sub: formatPercent(value?.totalPnLPct ?? 0),
            tone: (value?.totalPnL ?? 0) >= 0 ? "rise" : "fall",
            currency,
        },
        {
            label: t("summary.triggeredAlerts"),
            value: String(value?.triggeredAlerts ?? 0),
            sub: t("summary.winLossSub", { win: value?.winCount ?? 0, loss: value?.lossCount ?? 0 }),
            tone: (value?.triggeredAlerts ?? 0) > 0 ? "warn" : "neutral",
        },
    ];
});
</script>

<template>
    <section class="summary-strip">
        <article v-for="card in cards" :key="card.label" class="summary-card">
            <span class="summary-label">{{ card.label }}</span>
            <strong class="summary-value" :class="card.tone !== 'neutral' ? `tone-${card.tone}` : ''">
                <span v-if="card.currency" class="summary-currency">{{ card.currency }}</span>
                <span class="summary-number">{{ card.value }}</span>
            </strong>
            <span class="summary-sub" :class="card.tone === 'rise' || card.tone === 'fall' ? `tone-${card.tone}` : ''">{{ card.sub }}</span>
        </article>
    </section>
</template>

<style scoped>
.summary-strip {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 8px;
    padding: 0;
}

.summary-card {
    min-height: 82px;
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: linear-gradient(180deg, color-mix(in srgb, var(--panel-soft) 48%, var(--panel-strong)) 0%, var(--panel-strong) 100%);
    padding: 11px 12px 10px;
    display: grid;
    gap: 6px;
    box-shadow: var(--shadow-soft);
}

.summary-label {
    font-size: 11px;
    color: var(--muted);
}

.summary-value {
    display: inline-flex;
    align-items: baseline;
    gap: 0.42rem;
    font: 500 clamp(15px, 1vw, 18px) / 1.1 var(--font-display);
    letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
}

.summary-currency {
    font: inherit;
    line-height: inherit;
    color: inherit;
}

.summary-number {
    min-width: 0;
}

.summary-sub {
    font-size: 11px;
    color: var(--muted);
    line-height: 1.35;
}

.tone-rise {
    color: var(--rise);
}

.tone-fall {
    color: var(--fall);
}

.tone-warn {
    color: var(--warn);
}

@media (max-width: 1180px) {
    .summary-strip {
        grid-template-columns: 1fr;
    }
}
</style>
