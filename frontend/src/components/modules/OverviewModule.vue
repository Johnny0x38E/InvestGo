<script setup lang="ts">
import SummaryStrip from "../SummaryStrip.vue";
import { formatDateTime } from "../../format";
import { useI18n } from "../../i18n";
import type { DashboardSummary, RuntimeStatus } from "../../types";

defineProps<{
    dashboard: DashboardSummary | null;
    itemCount: number;
    livePriceCount: number;
    runtime: RuntimeStatus;
    generatedAt: string;
}>();

const { t } = useI18n();
</script>

<template>
    <section class="module-content overview-module">
        <div class="panel-header panel-header-stack">
            <div>
                <h3 class="title">{{ t("modules.overview") }}</h3>
                <p class="panel-subtitle">{{ t("overview.description") }}</p>
            </div>
        </div>

        <SummaryStrip :dashboard="dashboard" :item-count="itemCount" :live-price-count="livePriceCount" />

        <div class="overview-meta-grid">
            <article class="overview-note-card">
                <span class="overview-note-label">{{ t("overview.cards.quoteSource") }}</span>
                <strong>{{ runtime.quoteSource || "-" }}</strong>
                <span>{{ t("overview.cards.quoteSourceSub") }}</span>
            </article>
            <article class="overview-note-card">
                <span class="overview-note-label">{{ t("overview.cards.syncStatus") }}</span>
                <strong>{{ runtime.livePriceCount }}/{{ itemCount }}</strong>
                <span>{{ t("overview.cards.syncStatusSub") }}</span>
            </article>
            <article class="overview-note-card">
                <span class="overview-note-label">{{ t("overview.cards.updatedAt") }}</span>
                <strong>{{ formatDateTime(generatedAt) }}</strong>
                <span>{{ t("overview.cards.updatedAtSub") }}</span>
            </article>
        </div>
    </section>
</template>

<style scoped>
.overview-meta-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 8px;
}

.overview-note-card {
    min-height: 118px;
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: linear-gradient(180deg, color-mix(in srgb, var(--panel-soft) 92%, var(--accent-soft)) 0%, var(--panel-strong) 100%);
    padding: 14px;
    display: grid;
    align-content: start;
    gap: 6px;
}

.overview-note-card strong {
    font: 500 15px/1.2 var(--font-display);
}

.overview-note-label {
    font-size: 10px;
    color: var(--muted);
    letter-spacing: 0.08em;
    text-transform: uppercase;
}

.overview-note-card span:last-child {
    font-size: 11px;
    color: var(--muted);
    line-height: 1.6;
}

@media (max-width: 1180px) {
    .overview-meta-grid {
        grid-template-columns: 1fr;
    }
}
</style>
