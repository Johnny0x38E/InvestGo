<script setup lang="ts">
import {
    computed,
    onActivated,
    onBeforeUnmount,
    onDeactivated,
    onMounted,
    ref,
} from "vue";

import { formatHistoryTick, formatNumber } from "../format";
import { useI18n } from "../i18n";
import type { HistorySeries } from "../types";

const props = defineProps<{
    series: HistorySeries | null;
    loading: boolean;
    error: string;
}>();

const hoverIndex = ref<number | null>(null);
const tooltipPosition = ref<{ left: number; top: number } | null>(null);
const frameRef = ref<HTMLDivElement | null>(null);
const plotRef = ref<HTMLDivElement | null>(null);
const width = ref(1000);
const height = ref(420);
const padding = { top: 32, right: 14, bottom: 24, left: 14 };
const gradientID = `market-area-${Math.random().toString(36).slice(2, 10)}`;
let resizeObserver: ResizeObserver | null = null;
const { t } = useI18n();

const points = computed(() => props.series?.points ?? []);
const enrichedPoints = computed(() => {
    if (!points.value.length) {
        return [];
    }

    const highs = points.value.map((point) => point.high);
    const lows = points.value.map((point) => point.low);
    const minPrice = Math.min(...lows);
    const maxPrice = Math.max(...highs);
    const span = maxPrice - minPrice || maxPrice || 1;

    // Project price points into SVG coordinates first so the line, area, and hover hit testing can all reuse the same geometry.
    const xAt = (index: number) =>
        padding.left +
        (index / Math.max(points.value.length - 1, 1)) *
            (width.value - padding.left - padding.right);
    const yAt = (value: number) =>
        padding.top +
        (1 - (value - minPrice) / span) *
            (height.value - padding.top - padding.bottom);

    return points.value.map((point, index) => ({
        ...point,
        x: xAt(index),
        y: yAt(point.close),
        openY: yAt(point.open || point.close),
        highY: yAt(point.high),
        lowY: yAt(point.low),
    }));
});

const stats = computed(() => {
    const list = enrichedPoints.value;
    if (!list.length) {
        return null;
    }

    const high = list.reduce(
        (winner, point) => (point.high > winner.high ? point : winner),
        list[0],
    );
    const low = list.reduce(
        (winner, point) => (point.low < winner.low ? point : winner),
        list[0],
    );
    return {
        high,
        low,
        open: list[0],
        close: list[list.length - 1],
    };
});

const linePath = computed(() => buildAreaPath(enrichedPoints.value).line);
const areaPath = computed(() => buildAreaPath(enrichedPoints.value).area);
const ticks = computed(() => {
    if (!props.series || !enrichedPoints.value.length) {
        return [];
    }

    const maxIndex = enrichedPoints.value.length - 1;
    return [
        0,
        Math.floor(maxIndex / 3),
        Math.floor((maxIndex * 2) / 3),
        maxIndex,
    ].map((index) => enrichedPoints.value[index]);
});

const gridValues = computed(() => {
    if (!props.series || !enrichedPoints.value.length) {
        return [];
    }

    const high = props.series.high;
    const low = props.series.low;
    const span = high - low || high || 1;
    return Array.from({ length: 4 }, (_, index) => high - (span / 3) * index);
});

const currentPoint = computed(() => {
    const list = enrichedPoints.value;
    if (!list.length) {
        return null;
    }

    const index = hoverIndex.value ?? list.length - 1;
    return list[index] ?? list[list.length - 1];
});

const hoverPoint = computed(() => {
    const list = enrichedPoints.value;
    if (!list.length || hoverIndex.value == null) {
        return null;
    }

    return list[hoverIndex.value] ?? null;
});

const tooltipStyle = computed(() => {
    if (!tooltipPosition.value) {
        return {};
    }

    return {
        left: `${tooltipPosition.value.left}px`,
        top: `${tooltipPosition.value.top}px`,
    };
});

function buildAreaPath(list: Array<{ x: number; y: number }>): {
    line: string;
    area: string;
} {
    if (!list.length) {
        return { line: "", area: "" };
    }

    const line = buildSmoothPath(list);
    // Reuse the line path for the filled area and close the tail back to the bottom baseline.
    const baseline = (height.value - padding.bottom).toFixed(2);
    const area = `${line} L ${list[list.length - 1].x.toFixed(2)} ${baseline} L ${list[0].x.toFixed(2)} ${baseline} Z`;
    return { line, area };
}

function buildSmoothPath(list: Array<{ x: number; y: number }>): string {
    if (list.length === 1) {
        return `M ${list[0].x.toFixed(2)} ${list[0].y.toFixed(2)}`;
    }

    // Smooth the polyline with quadratic Bezier segments so small datasets do not produce harsh corners.
    const path: string[] = [
        `M ${list[0].x.toFixed(2)} ${list[0].y.toFixed(2)}`,
    ];
    for (let index = 1; index < list.length - 1; index += 1) {
        const xc = (list[index].x + list[index + 1].x) / 2;
        const yc = (list[index].y + list[index + 1].y) / 2;
        path.push(
            `Q ${list[index].x.toFixed(2)} ${list[index].y.toFixed(2)} ${xc.toFixed(2)} ${yc.toFixed(2)}`,
        );
    }

    const last = list[list.length - 1];
    const prev = list[list.length - 2];
    path.push(
        `Q ${prev.x.toFixed(2)} ${prev.y.toFixed(2)} ${last.x.toFixed(2)} ${last.y.toFixed(2)}`,
    );
    return path.join(" ");
}

function pointAtClientX(event: MouseEvent): void {
    const target = event.currentTarget as SVGRectElement | null;
    if (!target) {
        return;
    }

    const bounds = target.getBoundingClientRect();
    updateTooltipPosition(event);
    // Resolve hover hits by the current mouse X coordinate instead of relying on a fixed index step.
    const ratio = (event.clientX - bounds.left) / Math.max(bounds.width, 1);
    const nextX =
        padding.left + ratio * (width.value - padding.left - padding.right);
    let winner = 0;
    let distance = Number.POSITIVE_INFINITY;

    enrichedPoints.value.forEach((point, index) => {
        const gap = Math.abs(point.x - nextX);
        if (gap < distance) {
            distance = gap;
            winner = index;
        }
    });

    hoverIndex.value = winner;
}

function clearHover(): void {
    hoverIndex.value = null;
    tooltipPosition.value = null;
}

function updateTooltipPosition(event: MouseEvent): void {
    const frameBounds = frameRef.value?.getBoundingClientRect();
    if (!frameBounds) {
        return;
    }

    const tooltipWidth = 220;
    const tooltipHeight = 94;
    const margin = 12;
    const rawLeft = event.clientX - frameBounds.left + 18;
    const rawTopAbove = event.clientY - frameBounds.top - tooltipHeight - 10;
    const rawTopBelow = event.clientY - frameBounds.top + 16;
    const maxLeft = Math.max(frameBounds.width - tooltipWidth - margin, margin);
    const maxTop = Math.max(frameBounds.height - tooltipHeight - 34, margin);
    const top = rawTopAbove >= margin ? rawTopAbove : rawTopBelow;

    tooltipPosition.value = {
        left: Math.min(Math.max(rawLeft, margin), maxLeft),
        top: Math.min(Math.max(top, margin), maxTop),
    };
}

function yAt(value: number): number {
    const high = props.series?.high ?? 0;
    const low = props.series?.low ?? 0;
    const span = high - low || high || 1;
    return (
        padding.top +
        (1 - (value - low) / span) *
            (height.value - padding.top - padding.bottom)
    );
}

function syncPlotSize(): void {
    const host = plotRef.value;
    if (!host) {
        return;
    }

    const bounds = host.getBoundingClientRect();
    width.value = Math.max(Math.round(bounds.width), 320);
    height.value = Math.max(Math.round(bounds.height), 320);
}

function bindResizeObserver(): void {
    syncPlotSize();
    if (!plotRef.value || typeof ResizeObserver === "undefined") {
        return;
    }

    // Recompute the SVG size after tab switches and window resizes.
    // Read dimensions directly from contentRect to avoid extra layout work, then defer reactive writes with rAF
    // so ResizeObserver does not trigger a same-frame notification loop.
    resizeObserver?.disconnect();
    resizeObserver = new ResizeObserver((entries) => {
        const entry = entries[0];
        if (!entry) return;
        const { width: w, height: h } = entry.contentRect;
        requestAnimationFrame(() => {
            width.value = Math.max(Math.round(w), 320);
            height.value = Math.max(Math.round(h), 320);
        });
    });
    resizeObserver.observe(plotRef.value);
}

function unbindResizeObserver(): void {
    resizeObserver?.disconnect();
    resizeObserver = null;
}

onMounted(() => {
    bindResizeObserver();
});

onActivated(() => {
    bindResizeObserver();
});

onDeactivated(() => {
    unbindResizeObserver();
});

onBeforeUnmount(() => {
    unbindResizeObserver();
});
</script>

<template>
    <div class="chart-card-shell">
        <div v-if="loading" class="chart-empty">{{ t("chart.loading") }}</div>
        <div v-else-if="error" class="chart-empty chart-error">{{ error }}</div>
        <div v-else-if="!series || !enrichedPoints.length" class="chart-empty">
            {{ t("chart.empty") }}
        </div>
        <div
            v-else
            ref="frameRef"
            class="chart-frame"
            :class="series.change >= 0 ? 'is-rise' : 'is-fall'"
        >
            <div v-if="hoverPoint" class="chart-tooltip" :style="tooltipStyle">
                <strong>{{ formatNumber(hoverPoint.close, 2) }}</strong>
                <span>{{
                    formatHistoryTick(hoverPoint.timestamp, series.interval)
                }}</span>
                <span>{{
                    t("chart.openClose", {
                        open: formatNumber(hoverPoint.open, 2),
                        close: formatNumber(hoverPoint.close, 2),
                    })
                }}</span>
                <span>{{
                    t("chart.highLow", {
                        high: formatNumber(hoverPoint.high, 2),
                        low: formatNumber(hoverPoint.low, 2),
                    })
                }}</span>
            </div>

            <div ref="plotRef" class="chart-plot">
                <svg
                    class="chart-svg"
                    :viewBox="`0 0 ${width} ${height}`"
                    :aria-label="t('chart.aria')"
                >
                    <defs>
                        <linearGradient
                            :id="gradientID"
                            x1="0"
                            y1="0"
                            x2="0"
                            y2="1"
                        >
                            <stop class="chart-area-stop-top" offset="0%" />
                            <stop class="chart-area-stop-mid" offset="58%" />
                            <stop
                                class="chart-area-stop-bottom"
                                offset="100%"
                            />
                        </linearGradient>
                    </defs>

                    <template v-for="value in gridValues" :key="value">
                        <line
                            class="chart-grid"
                            :x1="padding.left"
                            :y1="yAt(value)"
                            :x2="width - padding.right"
                            :y2="yAt(value)"
                        />
                        <text
                            class="chart-axis-label chart-axis-label-left"
                            :x="padding.left + 2"
                            :y="yAt(value) - 8"
                        >
                            {{ formatNumber(value, 2) }}
                        </text>
                    </template>

                    <template v-if="stats">
                        <line
                            class="chart-marker chart-marker-accent"
                            :x1="padding.left"
                            :y1="stats.high.highY"
                            :x2="width - padding.right"
                            :y2="stats.high.highY"
                        />
                        <text
                            class="chart-marker-label chart-marker-label-accent"
                            :x="width - padding.right - 4"
                            :y="stats.high.highY - 5"
                        >
                            {{
                                t("chart.highMarker", {
                                    value: formatNumber(stats.high.high, 2),
                                })
                            }}
                        </text>

                        <line
                            class="chart-marker chart-marker-accent"
                            :x1="padding.left"
                            :y1="stats.low.lowY"
                            :x2="width - padding.right"
                            :y2="stats.low.lowY"
                        />
                        <text
                            class="chart-marker-label chart-marker-label-accent"
                            :x="width - padding.right - 4"
                            :y="stats.low.lowY - 5"
                        >
                            {{
                                t("chart.lowMarker", {
                                    value: formatNumber(stats.low.low, 2),
                                })
                            }}
                        </text>

                        <line
                            class="chart-marker"
                            :x1="padding.left"
                            :y1="stats.open.openY"
                            :x2="width - padding.right"
                            :y2="stats.open.openY"
                        />
                        <text
                            class="chart-marker-label"
                            :x="width - padding.right - 4"
                            :y="stats.open.openY - 5"
                        >
                            {{
                                t("chart.openMarker", {
                                    value: formatNumber(
                                        stats.open.open || stats.open.close,
                                        2,
                                    ),
                                })
                            }}
                        </text>

                        <line
                            class="chart-marker"
                            :x1="padding.left"
                            :y1="stats.close.y"
                            :x2="width - padding.right"
                            :y2="stats.close.y"
                        />
                        <text
                            class="chart-marker-label"
                            :x="width - padding.right - 4"
                            :y="stats.close.y - 5"
                        >
                            {{
                                t("chart.closeMarker", {
                                    value: formatNumber(stats.close.close, 2),
                                })
                            }}
                        </text>
                    </template>

                    <path
                        class="chart-area"
                        :d="areaPath"
                        :style="{ fill: `url(#${gradientID})` }"
                    />
                    <path class="chart-line" :d="linePath" />
                    <circle
                        v-if="currentPoint"
                        class="chart-dot"
                        :cx="currentPoint.x"
                        :cy="currentPoint.y"
                        r="4"
                    />
                    <rect
                        class="chart-hit-area"
                        :x="padding.left"
                        :y="padding.top"
                        :width="width - padding.left - padding.right"
                        :height="height - padding.top - padding.bottom"
                        @mousemove="pointAtClientX"
                        @mouseleave="clearHover"
                    />

                    <template
                        v-for="(point, index) in ticks"
                        :key="point.timestamp"
                    >
                        <text
                            class="chart-tick-label"
                            :x="point.x"
                            :y="height - 12"
                            :text-anchor="
                                index === 0
                                    ? 'start'
                                    : index === ticks.length - 1
                                      ? 'end'
                                      : 'middle'
                            "
                        >
                            {{
                                formatHistoryTick(
                                    point.timestamp,
                                    series.interval,
                                )
                            }}
                        </text>
                    </template>
                </svg>
            </div>
        </div>
    </div>
</template>
