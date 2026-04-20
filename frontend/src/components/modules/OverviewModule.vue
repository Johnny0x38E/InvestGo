<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import Chart from "primevue/chart";
import Button from "primevue/button";
import Skeleton from "primevue/skeleton";

import SummaryStrip from "../SummaryStrip.vue";
import { api, ApiAbortError } from "../../api";
import { formatDateTime, formatMoney, formatNumber, resolvedLocale } from "../../format";
import { useI18n } from "../../i18n";
import type { DashboardSummary, OverviewAnalytics } from "../../types";

const props = defineProps<{
    dashboard: DashboardSummary | null;
    itemCount: number;
    livePriceCount: number;
    generatedAt: string;
}>();

defineEmits<{
    (event: "refresh"): void;
}>();

const { t } = useI18n();

const analytics = ref<OverviewAnalytics | null>(null);
const loading = ref(false);
const error = ref("");
let inflightController: AbortController | null = null;

const documentStyle = ref<CSSStyleDeclaration | null>(null);

onMounted(() => {
    documentStyle.value = getComputedStyle(document.documentElement);
});

// ---------------------------------------------------------------------------
// Categorical chart palette — 8 perceptually distinct hues, tuned per mode.
// Independent of the UI theme colour so multi-asset charts stay readable.
// ---------------------------------------------------------------------------
const CHART_PALETTE_LIGHT = [
    "#2a62aa", // blue    hue 210
    "#0f8860", // teal    hue 165
    "#b07610", // amber   hue  42
    "#6438ac", // violet  hue 268
    "#bc2e4c", // rose    hue 346
    "#267820", // green   hue 110
    "#bc4c18", // orange  hue  24
    "#9c2888", // fuchsia hue 302
];
const CHART_PALETTE_DARK = [
    "#5ea4ec", // blue
    "#35c89e", // teal
    "#f2b238", // amber
    "#aa7ee8", // violet
    "#f06e8e", // rose
    "#68cc50", // green
    "#f2904e", // orange
    "#d468be", // fuchsia
];

/** Read the 8 chart colours from live CSS variables (respects theme switches). */
function resolveChartPalette(): string[] {
    const ds = documentStyle.value;
    if (!ds) return CHART_PALETTE_DARK;
    const sample = ds.getPropertyValue("--chart-1").trim();
    if (sample) {
        return [1, 2, 3, 4, 5, 6, 7, 8].map((i) => ds.getPropertyValue(`--chart-${i}`).trim() || CHART_PALETTE_DARK[i - 1]);
    }
    // Fallback when CSS variables are not yet injected: detect mode via --app-bg.
    const bg = ds.getPropertyValue("--app-bg").trim();
    const isDark = bg.startsWith("#0") || bg.includes("15, 17") || bg.includes("15,17");
    return isDark ? CHART_PALETTE_DARK : CHART_PALETTE_LIGHT;
}

// Single palette shared by both the breakdown doughnut and trend charts.
const chartPalette = computed(() => resolveChartPalette());

const trendTotalColor = computed(() => {
    if (!documentStyle.value) return "#24476f";
    return documentStyle.value.getPropertyValue("--accent-strong").trim() || "#24476f";
});

const breakdownTotal = computed(() => analytics.value?.breakdown.reduce((sum, slice) => sum + slice.value, 0) ?? 0);
const cacheSummary = computed(() => (analytics.value?.cached ? t("common.cacheHit") : t("common.cacheMiss")));
const hasOverviewData = computed(() => {
    const dashboard = props.dashboard;
    if (!dashboard) {
        return false;
    }

    return dashboard.totalCost > 0 || dashboard.totalValue > 0;
});

const doughnutData = computed(() => {
    const breakdown = analytics.value?.breakdown ?? [];
    return {
        labels: breakdown.map((slice) => slice.name || slice.symbol),
        datasets: [
            {
                data: breakdown.map((slice) => slice.value),
                backgroundColor: breakdown.map((_, index) => chartPalette.value[index % chartPalette.value.length]),
                borderWidth: 0,
                hoverOffset: 12,
                cutout: "70%",
            },
        ],
    };
});

const doughnutOptions = computed(() => {
    return {
        responsive: true,
        maintainAspectRatio: false,
        layout: {
            padding: 16,
        },
        animation: {
            duration: 220,
        },
        plugins: {
            legend: {
                display: false,
            },
            tooltip: {
                backgroundColor: "rgba(10, 16, 30, 0.96)",
                padding: 10,
                cornerRadius: 8,
                boxWidth: 8,
                boxHeight: 8,
                usePointStyle: true,
                titleFont: { size: 12, weight: "600" },
                bodyFont: { size: 12 },
                borderColor: "rgba(255,255,255,0.08)",
                borderWidth: 1,
                callbacks: {
                    labelPointStyle() {
                        return { pointStyle: "circle", rotation: 0 };
                    },
                    labelColor(context: { dataset: { backgroundColor: string[] }; dataIndex: number }) {
                        const colors = context.dataset.backgroundColor as string[];
                        return { borderColor: colors[context.dataIndex], backgroundColor: colors[context.dataIndex], borderWidth: 0 };
                    },
                    label(context: { parsed: number }) {
                        const value = context.parsed ?? 0;
                        const weight = breakdownTotal.value > 0 ? (value / breakdownTotal.value) * 100 : 0;
                        return ` ${formatMoney(value)}  ${formatNumber(weight, 1)}%`;
                    },
                },
            },
        },
    };
});

const trendData = computed(() => {
    const trend = analytics.value?.trend;
    if (!trend || trend.dates.length === 0 || trend.series.length === 0) {
        return null;
    }

    const labels = trend.dates.map((value) =>
        new Intl.DateTimeFormat(resolvedLocale(), {
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
        }).format(new Date(value)),
    );

    const totalValues = labels.map((_, index) => trend.series.reduce((sum, series) => sum + (series.values[index] ?? 0), 0));

    return {
        labels,
        datasets: [
            {
                label: t("overview.charts.trend.totalLine"),
                data: totalValues,
                borderColor: trendTotalColor.value,
                backgroundColor: `${trendTotalColor.value}18`,
                fill: true,
                tension: 0.34,
                pointRadius: 0,
                pointHoverRadius: 4,
                pointHitRadius: 10,
                borderWidth: 2.4,
                order: 0,
                yAxisID: "y",
            },
            ...trend.series.map((series, index) => {
                const color = chartPalette.value[index % chartPalette.value.length];
                return {
                    label: series.name || series.symbol,
                    data: series.values,
                    borderColor: color,
                    backgroundColor: color,
                    fill: false,
                    tension: 0.28,
                    pointRadius: 0,
                    pointHoverRadius: 3,
                    pointHitRadius: 10,
                    borderWidth: 1.4,
                    borderDash: [6, 4],
                    order: 1,
                    yAxisID: "y1",
                };
            }),
        ],
    };
});

const trendOptions = computed(() => {
    return {
        responsive: true,
        maintainAspectRatio: false,
        animation: {
            duration: 220,
        },
        interaction: {
            mode: "index",
            intersect: false,
        },
        plugins: {
            legend: {
                display: false,
            },
            tooltip: {
                backgroundColor: "rgba(10, 16, 30, 0.96)",
                padding: 14,
                cornerRadius: 10,
                boxWidth: 8,
                boxHeight: 8,
                usePointStyle: true,
                titleFont: { size: 12, weight: "600" },
                bodyFont: { size: 12 },
                bodySpacing: 8,
                borderColor: "rgba(255,255,255,0.08)",
                borderWidth: 1,
                callbacks: {
                    labelPointStyle() {
                        return { pointStyle: "circle", rotation: 0 };
                    },
                    labelColor(context: { dataset: { borderColor: string } }) {
                        const color = context.dataset.borderColor as string;
                        return { borderColor: color, backgroundColor: color, borderWidth: 0 };
                    },
                    label(context: { dataset: { label: string }; parsed: { y: number } }) {
                        return `  ${context.dataset.label}    ${formatMoney(context.parsed.y ?? 0)}`;
                    },
                },
            },
        },
        scales: {
            x: {
                grid: {
                    display: false,
                },
                ticks: {
                    maxRotation: 0,
                    autoSkip: true,
                    maxTicksLimit: 4,
                    color: "rgba(148, 163, 184, 0.82)",
                    font: {
                        size: 10,
                    },
                },
                border: {
                    display: false,
                },
            },
            y: {
                type: "linear",
                display: true,
                position: "left",
                grid: {
                    color: "rgba(148, 163, 184, 0.08)",
                },
                ticks: {
                    maxTicksLimit: 3,
                    color: "rgba(148, 163, 184, 0.78)",
                    font: { size: 10 },
                    callback(value: number | string) {
                        return formatMoney(Number(value));
                    },
                },
                border: { display: false },
            },
            y1: {
                type: "linear",
                display: true,
                position: "right",
                grid: {
                    drawOnChartArea: false,
                },
                ticks: {
                    maxTicksLimit: 3,
                    color: "rgba(148, 163, 184, 0.5)",
                    font: { size: 10 },
                    callback(value: number | string) {
                        return formatMoney(Number(value));
                    },
                },
                border: { display: false },
            },
        },
    };
});

watch(
    () => props.generatedAt,
    () => {
        void loadOverview();
    },
    { immediate: true },
);

onBeforeUnmount(() => {
    inflightController?.abort(new ApiAbortError("aborted"));
});

function applyEmptyOverview(): void {
    analytics.value = {
        displayCurrency: props.dashboard?.displayCurrency || "CNY",
        breakdown: [],
        trend: {
            dates: [],
            series: [],
            totalValue: 0,
        },
        cached: false,
        generatedAt: props.generatedAt || new Date().toISOString(),
    };
    loading.value = false;
    error.value = "";
}

async function loadOverview(): Promise<void> {
    if (!hasOverviewData.value) {
        inflightController?.abort(new ApiAbortError("aborted"));
        inflightController = null;
        applyEmptyOverview();
        return;
    }

    inflightController?.abort(new ApiAbortError("aborted"));
    const controller = new AbortController();
    inflightController = controller;
    loading.value = true;
    error.value = "";

    try {
        analytics.value = await api<OverviewAnalytics>("/api/overview", {
            signal: controller.signal,
            timeoutMs: 20000,
        });
    } catch (nextError) {
        if (nextError instanceof ApiAbortError) {
            return;
        }
        analytics.value = null;
        error.value = nextError instanceof Error ? nextError.message : t("overview.loadFailed");
    } finally {
        if (inflightController === controller) {
            inflightController = null;
            loading.value = false;
        }
    }
}
</script>

<template>
    <section class="module-content overview-module">
        <div class="panel-header panel-header-stack">
            <div>
                <h3 class="title">{{ t("modules.overview") }}</h3>
            </div>
            <div class="toolbar-row">
                <Button size="small" text icon="pi pi-refresh" :label="t('common.refresh')" @click="$emit('refresh')" />
            </div>
        </div>

        <SummaryStrip :dashboard="dashboard" :item-count="itemCount" :live-price-count="livePriceCount" />

        <div v-if="loading" class="overview-loading-grid">
            <div class="overview-card">
                <div class="overview-loading-card">
                    <Skeleton width="8rem" height="1rem" />
                    <div class="overview-loading-breakdown">
                        <Skeleton shape="circle" size="10rem" />
                        <div class="overview-loading-list">
                            <Skeleton v-for="index in 4" :key="index" width="100%" height="2.5rem" />
                        </div>
                    </div>
                </div>
            </div>

            <div class="overview-card">
                <div class="overview-loading-card">
                    <Skeleton width="8rem" height="1rem" />
                    <Skeleton width="100%" height="18rem" />
                </div>
            </div>
        </div>

        <div v-else-if="error" class="overview-empty">{{ error }}</div>

        <div v-else-if="analytics" class="overview-stack">
            <div class="overview-card overview-card-top">
                <div class="overview-head">
                    <h4>{{ t("overview.charts.category.title") }}</h4>
                    <div class="toolbar-row">
                        <span class="overview-meta-chip">{{ t("overview.meta.cache", { state: cacheSummary }) }}</span>
                        <span v-if="analytics.cacheExpiresAt" class="overview-meta-chip">{{ t("common.cacheFreshUntil", { time: formatDateTime(analytics.cacheExpiresAt) }) }}</span>
                    </div>
                </div>

                <div v-if="analytics.breakdown.length" class="overview-breakdown">
                    <div class="overview-doughnut-wrap">
                        <div class="overview-doughnut-shell">
                            <Chart type="doughnut" :data="doughnutData" :options="doughnutOptions" class="overview-doughnut-chart" />
                            <div class="overview-doughnut-center">
                                <strong>{{ formatMoney(breakdownTotal) }}</strong>
                                <span>{{ t("overview.charts.category.totalValue") }}</span>
                            </div>
                        </div>
                    </div>

                    <div class="overview-breakdown-list">
                        <div v-for="(slice, index) in analytics.breakdown" :key="slice.itemId" class="overview-breakdown-row">
                            <div class="overview-breakdown-line">
                                <span class="overview-breakdown-dot" :style="{ backgroundColor: chartPalette[index % chartPalette.length] }"></span>
                                <strong>{{ slice.name || slice.symbol }}</strong>
                                <span class="overview-breakdown-pct">{{ formatNumber(slice.weight * 100, 1) }}%</span>
                            </div>
                            <div class="overview-breakdown-bar">
                                <div class="overview-breakdown-fill" :style="{ width: `${Math.max(slice.weight * 100, 6)}%`, backgroundColor: chartPalette[index % chartPalette.length] }"></div>
                            </div>
                            <span class="overview-breakdown-value">{{ formatMoney(slice.value) }}</span>
                        </div>
                    </div>
                </div>
                <div v-else class="overview-empty">{{ t("overview.charts.category.empty") }}</div>
            </div>

            <div class="overview-card overview-card-bottom">
                <div class="overview-head">
                    <h4>{{ t("overview.charts.trend.title") }}</h4>
                </div>

                <div v-if="trendData" class="overview-trend">
                    <div class="overview-trend-shell">
                        <Chart type="line" :data="trendData" :options="trendOptions" class="overview-trend-chart" />
                    </div>

                    <div class="overview-series-list">
                        <div v-for="(series, index) in analytics.trend.series" :key="series.itemId" class="overview-legend-item">
                            <span class="overview-breakdown-dot" :style="{ backgroundColor: chartPalette[index % chartPalette.length] }"></span>
                            <span class="overview-legend-label">{{ series.name || series.symbol }}</span>
                            <b class="overview-legend-value">{{ formatMoney(series.latestValue) }}</b>
                        </div>
                    </div>
                </div>
                <div v-else class="overview-empty">{{ t("overview.charts.trend.empty") }}</div>
            </div>
        </div>
    </section>
</template>

<style scoped>
.overview-module {
    min-height: 0;
    min-width: 0;
    width: 100%;
    gap: 24px;
    overflow: hidden;
}

.overview-loading-grid,
.overview-stack {
    display: grid;
    grid-template-rows: 260px minmax(0, 1fr);
    gap: 36px;
    padding-bottom: 12px;
    min-height: 0;
    min-width: 0;
    width: 100%;
    flex: 1 1 0;
    overflow: hidden;
}

.overview-card {
    min-height: 0;
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: 16px;
    overflow: hidden;
}

.overview-breakdown,
.overview-trend,
.overview-loading-card {
    min-height: 0;
    flex: 1 1 0;
}

.overview-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    min-width: 0;
}

.overview-meta-chip {
    font-size: 11px;
    color: var(--muted);
    background: color-mix(in srgb, var(--panel-soft) 92%, transparent);
    border: 1px solid color-mix(in srgb, var(--border) 88%, transparent);
    border-radius: 999px;
    padding: 3px 8px;
    white-space: nowrap;
    line-height: 1.6;
}

.overview-head h4 {
    margin: 0;
    font: 600 14px/1.1 var(--font-display);
}

.overview-breakdown {
    display: grid;
    grid-template-columns: 280px minmax(0, 1fr);
    gap: 24px;
    height: 100%;
    min-height: 0;
    overflow: hidden;
    position: relative;
}

.overview-doughnut-wrap {
    min-height: 0;
    min-width: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
}

.overview-doughnut-shell {
    border: none;
    background: transparent;
    position: relative;
    display: grid;
    width: 100%;
    max-width: 240px;
    aspect-ratio: 1 / 1;
    min-height: 0;
}

.overview-doughnut-chart {
    width: 100%;
    height: 100%;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    position: relative;
    z-index: 10;
}

.overview-doughnut-chart :deep(canvas) {
    width: 100% !important;
    height: 100% !important;
    max-width: 100% !important;
    max-height: 100% !important;
    display: block;
}

.overview-doughnut-center {
    position: absolute;
    inset: 0;
    display: grid;
    place-content: center;
    gap: 3px;
    text-align: center;
    pointer-events: none;
    z-index: 1;
}

.overview-doughnut-center strong {
    font: 600 16px/1.05 var(--font-display);
}

.overview-doughnut-center span {
    font-size: 10px;
    color: var(--muted);
}

.overview-breakdown-list {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 16px;
    overflow-y: auto;
    overflow-x: hidden;
    min-height: 0;
    min-width: 0;
    align-content: start;
}

.overview-breakdown-dot {
    width: 8px;
    height: 8px;
    border-radius: 999px;
    flex: 0 0 auto;
}

.overview-breakdown-row {
    padding: 4px 8px;
    border: none;
    background: transparent;
    display: grid;
    gap: 4px;
}

.overview-breakdown-line {
    display: grid;
    grid-template-columns: 8px minmax(0, 1fr) auto;
    align-items: center;
    gap: 8px;
}

.overview-breakdown-line strong {
    font-size: 11px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

.overview-breakdown-pct,
.overview-breakdown-value {
    font-size: 10px;
    color: var(--muted);
}

.overview-breakdown-bar {
    height: 4px;
    border-radius: 999px;
    background: color-mix(in srgb, var(--border) 84%, transparent);
    overflow: hidden;
}

.overview-breakdown-fill {
    height: 100%;
    border-radius: inherit;
}

.overview-trend {
    display: grid;
    grid-template-rows: minmax(0, 1fr) auto;
    gap: 8px;
    height: 100%;
    min-height: 0;
    min-width: 0;
    overflow: hidden;
}

.overview-trend-shell {
    border: none;
    background: transparent;
    padding: 8px;
    min-height: 0;
    overflow: hidden;
}

.overview-trend-chart {
    width: 100%;
    height: 100%;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
}

.overview-trend-chart :deep(canvas) {
    width: 100% !important;
    height: 100% !important;
    max-width: 100% !important;
    max-height: 100% !important;
    display: block;
}

.overview-series-list {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    overflow: hidden;
    min-width: 0;
}

.overview-legend-item {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 8px;
    border: none;
    background: transparent;
    font-size: 10px;
}

.overview-legend-label {
    color: var(--muted);
}

.overview-legend-value {
    font-size: 10px;
}

.overview-empty {
    min-height: 0;
    min-width: 0;
    height: 100%;
    border: 1px dashed color-mix(in srgb, var(--border) 90%, transparent);
    border-radius: 12px;
    display: grid;
    place-items: center;
    color: var(--muted);
}

.overview-loading-card,
.overview-loading-breakdown,
.overview-loading-list {
    display: grid;
    gap: 10px;
}

.overview-loading-breakdown {
    grid-template-columns: 10rem 1fr;
    align-items: center;
}

@media (max-width: 1180px) {
    .overview-loading-grid,
    .overview-stack {
        grid-template-rows: 250px minmax(0, 1fr);
    }

    .overview-breakdown {
        grid-template-columns: 1fr;
    }

    .overview-head {
        flex-direction: column;
        align-items: flex-start;
    }
}
</style>
