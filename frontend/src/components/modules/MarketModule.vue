<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Select from "primevue/select";

import PriceChart from "../PriceChart.vue";
import { historyRangeOptions } from "../../constants";
import { formatDateTime, formatMoney, formatPercent, formatUnitPrice, historyRangeLabel } from "../../format";
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

// 绑定右侧下拉框的当前选中标的。
const selectedItemProxy = computed({
    get: () => props.selectedItemId,
    set: (value: string) => emit("update:selectedItemId", value),
});

const marketSnapshot = computed(() => {
    const item = props.selectedItem;
    const series = props.historySeries;
    if (!item) {
        return null;
    }

    // 实时行情价格优先；无实时价时回落到图表最后一根 K 线收盘价。
    const livePrice = item.currentPrice || series?.endPrice || 0;

    // 区间变化以实时价为终点（对比图表区间起点），与自选/热门列表保持价格来源一致。
    // 若缺少实时价或图表数据，则回落到后端计算好的 series.change / item.change。
    const hasLiveAndSeries = item.currentPrice > 0 && series != null && series.startPrice > 0;
    const effectiveChange = hasLiveAndSeries ? item.currentPrice - series.startPrice : (series?.change ?? item.change ?? 0);
    const effectiveChangePct = hasLiveAndSeries ? ((item.currentPrice - series!.startPrice) / series!.startPrice) * 100 : (series?.changePercent ?? item.changePercent ?? 0);
    const previousClose = item.previousClose || series?.startPrice || 0;
    const openPrice = item.openPrice || series?.points?.[0]?.open || 0;
    const rangeHigh = series?.high ?? item.dayHigh ?? 0;
    const rangeLow = series?.low ?? item.dayLow ?? 0;
    const amplitudePct = previousClose > 0 && rangeHigh > 0 && rangeLow > 0 ? ((rangeHigh - rangeLow) / previousClose) * 100 : 0;
    const positionValue = item.quantity > 0 ? item.quantity * livePrice : 0;
    const lastPoint = series?.points ? series.points[series.points.length - 1] : undefined;
    const lastVolume = lastPoint?.volume ?? 0;
    const changeTone: MarketMetricCard["tone"] = effectiveChange > 0 ? "rise" : effectiveChange < 0 ? "fall" : "neutral";
    const positionBaseline = item.quantity > 0 ? item.quantity * item.costPrice : 0;
    const positionPnL = item.quantity > 0 ? positionValue - positionBaseline : 0;
    const positionPnLPct = positionBaseline > 0 ? (positionPnL / positionBaseline) * 100 : 0;
    const positionTone: MarketMetricCard["tone"] = positionValue > positionBaseline ? "rise" : positionValue < positionBaseline ? "fall" : "neutral";

    return {
        item,
        series,
        livePrice,
        effectiveChange,
        effectiveChangePct,
        previousClose,
        openPrice,
        rangeHigh,
        rangeLow,
        amplitudePct,
        positionValue,
        positionBaseline,
        positionPnL,
        positionPnLPct,
        lastVolume,
        changeTone,
        positionTone,
    };
});

const marketOverview = computed(() => {
    const snapshot = marketSnapshot.value;
    if (!snapshot) {
        return null;
    }

    return {
        title: snapshot.item.name || snapshot.item.symbol,
        market: snapshot.item.market,
        symbol: snapshot.item.symbol,
        price: formatUnitPrice(snapshot.livePrice, snapshot.item.currency),
        changeLabel: `${historyRangeLabel(props.historyInterval)}变化`,
        changeValue: formatMoney(snapshot.effectiveChange, true),
        changePercent: formatPercent(snapshot.effectiveChangePct),
        quoteSource: snapshot.item.quoteSource || "-",
        chartSource: snapshot.series?.source || "无图表数据",
        syncedAt: formatDateTime(snapshot.item.quoteUpdatedAt),
        tone: snapshot.changeTone,
    };
});

// 持仓合并卡数据：市值 + 盈亏，无持仓时返回 null。
const positionDetail = computed(() => {
    const snapshot = marketSnapshot.value;
    if (!snapshot) return null;

    const hasPosition = snapshot.item.quantity > 0;
    return {
        hasPosition,
        value: hasPosition ? formatUnitPrice(snapshot.positionValue, snapshot.item.currency) : "-",
        pnl: hasPosition ? formatMoney(snapshot.positionPnL, true) : "-",
        pnlPct: hasPosition ? formatPercent(snapshot.positionPnLPct) : "-",
        costBasis: hasPosition ? formatUnitPrice(snapshot.positionBaseline, snapshot.item.currency) : "-",
        costPrice: hasPosition ? formatUnitPrice(snapshot.item.costPrice, snapshot.item.currency) : "-",
        quantity: snapshot.item.quantity,
        tone: snapshot.positionTone,
    };
});

// 生成市场模块右侧的详情卡片（不含持仓，已单独渲染）。
const marketCards = computed<MarketMetricCard[]>(() => {
    const snapshot = marketSnapshot.value;
    if (!snapshot) {
        return [];
    }

    return [
        {
            label: "昨收 / 开盘",
            value: `${formatUnitPrice(snapshot.previousClose, snapshot.item.currency)} / ${formatUnitPrice(snapshot.openPrice, snapshot.item.currency)}`,
            sub: snapshot.item.quoteSource || "-",
            tone: "neutral",
        },
        {
            label: "区间高低",
            value: `${formatUnitPrice(snapshot.rangeHigh, snapshot.item.currency)} / ${formatUnitPrice(snapshot.rangeLow, snapshot.item.currency)}`,
            sub: historyRangeLabel(props.historyInterval),
            tone: "neutral",
        },
        {
            label: "日内振幅",
            value: formatPercent(snapshot.amplitudePct),
            sub: snapshot.previousClose > 0 ? "按昨收估算" : "等待价格同步",
            tone: "neutral",
        },
    ];
});
</script>

<template>
    <section class="module-content market-module">
        <div class="panel-header">
            <div class="toolbar-row">
                <h3 class="title">行情</h3>
                <Button text icon="pi pi-refresh" label="刷新" @click="$emit('refresh')" />
            </div>
            <Select v-model="selectedItemProxy" :options="historyItemOptions" option-label="label" option-value="value" class="compact-select market-symbol-select" />
        </div>

        <div class="market-board">
            <div class="market-main">
                <PriceChart :series="historySeries" :loading="historyLoading" :error="historyError" />
            </div>

            <aside class="market-aside">
                <div v-if="marketOverview" class="market-inspector">
                    <section class="market-hero" :class="`tone-${marketOverview.tone}`">
                        <h4>{{ marketOverview.title }}</h4>
                        <p class="market-hero-subline">{{ marketOverview.market }} · {{ marketOverview.symbol }}</p>

                        <div class="market-hero-main">
                            <strong class="market-hero-price">{{ marketOverview.price }}</strong>
                            <div class="market-hero-delta">
                                <span class="market-hero-delta-label">{{ marketOverview.changeLabel }}</span>
                                <b class="market-hero-delta-val">{{ marketOverview.changeValue }}</b>
                                <span class="market-hero-delta-pct">{{ marketOverview.changePercent }}</span>
                            </div>
                        </div>

                        <div class="market-hero-intervals">
                            <button
                                v-for="entry in historyRangeOptions"
                                :key="entry.value"
                                class="interval-pill"
                                :class="{ active: historyInterval === entry.value }"
                                type="button"
                                @click="$emit('select-interval', entry.value)"
                            >
                                {{ entry.label }}
                            </button>
                        </div>

                        <footer class="market-hero-foot">
                            <span class="market-hero-badge">报价 {{ marketOverview.quoteSource }}</span>
                            <span class="market-hero-badge">图表 {{ marketOverview.chartSource }}</span>
                            <span class="market-hero-sync">{{ marketOverview.syncedAt }}</span>
                        </footer>
                    </section>

                    <div class="market-metrics">
                        <article v-if="positionDetail" class="market-position-card" :class="positionDetail.hasPosition ? `tone-${positionDetail.tone}` : ''">
                            <span class="market-pos-label">我的持仓</span>
                            <template v-if="positionDetail.hasPosition">
                                <div class="market-pos-main">
                                    <div class="market-pos-stat">
                                        <strong class="market-pos-value">{{ positionDetail.value }}</strong>
                                        <span class="market-pos-stat-label">当前资产</span>
                                    </div>
                                    <div class="market-pos-stat market-pos-stat--right">
                                        <b class="market-pos-pnl">{{ positionDetail.pnl }}</b>
                                        <span class="market-pos-pnl-pct">{{ positionDetail.pnlPct }}</span>
                                    </div>
                                </div>
                                <div class="market-pos-detail">
                                    <div class="market-pos-stat">
                                        <span class="market-pos-stat-label">成本价</span>
                                        <span class="market-pos-detail-val">{{ positionDetail.costPrice }}</span>
                                    </div>
                                    <div class="market-pos-stat market-pos-stat--right">
                                        <span class="market-pos-stat-label">持仓量</span>
                                        <span class="market-pos-detail-val">{{ positionDetail.quantity }} 股/份</span>
                                    </div>
                                </div>
                            </template>
                            <span v-else class="market-pos-empty">未录入持仓数量</span>
                        </article>

                        <article v-for="card in marketCards" :key="card.label" class="metric-strip" :class="`tone-${card.tone}`">
                            <span class="metric-strip-label">{{ card.label }}</span>
                            <strong class="metric-strip-value">{{ card.value }}</strong>
                            <span class="metric-strip-sub">{{ card.sub }}</span>
                        </article>
                    </div>
                </div>

                <div v-else class="market-inspector market-inspector-empty">
                    <span>请选择标的查看行情详情</span>
                </div>
            </aside>
        </div>
    </section>
</template>
