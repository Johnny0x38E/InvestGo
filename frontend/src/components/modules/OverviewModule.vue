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
