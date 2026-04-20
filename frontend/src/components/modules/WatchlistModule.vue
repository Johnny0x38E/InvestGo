<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import PriceChart from "../PriceChart.vue";
import { getHistoryRangeOptions } from "../../constants";
import { formatDateTime, formatMoney, formatPercent, formatShares, formatUnitPrice, historyRangeLabel } from "../../format";
import { useI18n } from "../../i18n";
import type { HistoryInterval, HistorySeries, MarketMetricCard, WatchlistItem } from "../../types";

const props = defineProps<{
    selectedItem: WatchlistItem | null;
    historyInterval: HistoryInterval;
    historySeries: HistorySeries | null;
    historyLoading: boolean;
    historyError: string;
}>();

defineEmits<{
    (event: "refresh"): void;
    (event: "select-interval", value: HistoryInterval): void;
    (event: "delete-item", value: string): void;
}>();

const { t } = useI18n();
const historyRangeOptions = computed(() => getHistoryRangeOptions());
const historyCacheSummary = computed(() => (props.historySeries?.cached ? t("common.cacheHit") : t("common.cacheMiss")));

const marketSnapshot = computed(() => {
    const item = props.selectedItem;
    const series = props.historySeries;
    if (!item) {
        return null;
    }
    const snapshot = series?.snapshot;
    const livePrice = snapshot?.livePrice ?? item.currentPrice ?? series?.endPrice ?? 0;
    const effectiveChange = snapshot?.effectiveChange ?? series?.change ?? item.change ?? 0;
    const effectiveChangePct = snapshot?.effectiveChangePct ?? series?.changePercent ?? item.changePercent ?? 0;
    const changeTone: MarketMetricCard["tone"] = effectiveChange > 0 ? "rise" : effectiveChange < 0 ? "fall" : "neutral";
    const positionValue = snapshot?.positionValue ?? item.position?.marketValue ?? 0;
    const positionBaseline = snapshot?.positionBaseline ?? item.position?.costBasis ?? 0;
    const positionPnL = snapshot?.positionPnL ?? item.position?.unrealisedPnL ?? 0;
    const positionPnLPct = snapshot?.positionPnLPct ?? item.position?.unrealisedPnLPct ?? 0;
    const positionTone: MarketMetricCard["tone"] = positionValue > positionBaseline ? "rise" : positionValue < positionBaseline ? "fall" : "neutral";

    return {
        item,
        series,
        livePrice,
        effectiveChange,
        effectiveChangePct,
        previousClose: snapshot?.previousClose ?? 0,
        openPrice: snapshot?.openPrice ?? 0,
        rangeHigh: snapshot?.rangeHigh ?? 0,
        rangeLow: snapshot?.rangeLow ?? 0,
        amplitudePct: snapshot?.amplitudePct ?? 0,
        positionValue,
        positionBaseline,
        positionPnL,
        positionPnLPct,
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
        changeLabel: t("watchlist.changeLabel", {
            range: historyRangeLabel(props.historyInterval),
        }),
        changeValue: formatMoney(snapshot.effectiveChange, true),
        changePercent: formatPercent(snapshot.effectiveChangePct),
        quoteSource: snapshot.item.quoteSource || "-",
        chartSource: snapshot.series?.source || t("watchlist.noChartData"),
        syncedAt: formatDateTime(snapshot.item.quoteUpdatedAt),
        tone: snapshot.changeTone,
    };
});

// Build the combined position card data for market value and PnL; return null when there is no position.
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
        costPrice: hasPosition ? formatUnitPrice(snapshot.item.costPrice, snapshot.item.currency, 4) : "-",
        quantity: snapshot.item.quantity,
        tone: snapshot.positionTone,
    };
});

// Build the detail cards shown on the right side of the market module, excluding the separately rendered position card.
const marketCards = computed<MarketMetricCard[]>(() => {
    const snapshot = marketSnapshot.value;
    if (!snapshot) {
        return [];
    }

    return [
        {
            label: t("watchlist.cards.prevCloseOpen"),
            value: `${formatUnitPrice(snapshot.previousClose, snapshot.item.currency)} / ${formatUnitPrice(snapshot.openPrice, snapshot.item.currency)}`,
            sub: snapshot.item.quoteSource || "-",
            tone: "neutral",
        },
        {
            label: t("watchlist.cards.rangeHighLow"),
            value: `${formatUnitPrice(snapshot.rangeHigh, snapshot.item.currency)} / ${formatUnitPrice(snapshot.rangeLow, snapshot.item.currency)}`,
            sub: historyRangeLabel(props.historyInterval),
            tone: "neutral",
        },
        {
            label: t("watchlist.cards.amplitude"),
            value: formatPercent(snapshot.amplitudePct),
            sub: snapshot.previousClose > 0 ? t("watchlist.cards.amplitudeEstimated") : t("watchlist.cards.amplitudePending"),
            tone: "neutral",
        },
    ];
});
</script>

<template>
    <section class="module-content watchlist-module">
        <div class="panel-header">
            <div>
                <h3 class="title">{{ t("watchlist.title") }}</h3>
            </div>
            <div class="toolbar-row">
                <Button v-if="selectedItem" size="small" text icon="pi pi-bookmark-fill" :label="t('watchlist.unwatch')" style="color: var(--accent)" @click="$emit('delete-item', selectedItem.id)" />
                <Button size="small" text icon="pi pi-refresh" :label="t('watchlist.refresh')" @click="$emit('refresh')" />
            </div>
        </div>

        <div class="market-board">
            <div class="market-main">
                <PriceChart :series="historySeries" :loading="historyLoading" :error="historyError" />
            </div>

            <aside class="market-aside">
                <div v-if="marketOverview" class="market-inspector">
                    <section class="market-hero" :class="`tone-${marketOverview.tone}`">
                        <div class="market-hero-topbar">
                            <div class="market-hero-intervals">
                                <button
                                    v-for="entry in historyRangeOptions"
                                    :key="entry.value"
                                    class="interval-pill"
                                    :class="{
                                        active: historyInterval === entry.value,
                                    }"
                                    type="button"
                                    @click="$emit('select-interval', entry.value)"
                                >
                                    {{ entry.label }}
                                </button>
                            </div>
                        </div>

                        <h4>{{ marketOverview.title }}</h4>
                        <p class="market-hero-subline">
                            {{ marketOverview.market }} ·
                            {{ marketOverview.symbol }}
                        </p>

                        <div class="market-hero-main">
                            <strong class="market-hero-price">{{ marketOverview.price }}</strong>
                            <div class="market-hero-delta">
                                <span class="market-hero-delta-label">{{ marketOverview.changeLabel }}</span>
                                <b class="market-hero-delta-val">{{ marketOverview.changeValue }}</b>
                                <span class="market-hero-delta-pct">{{ marketOverview.changePercent }}</span>
                            </div>
                        </div>

                        <footer class="market-hero-foot">
                            <span class="market-hero-badge">{{
                                t("watchlist.hero.quote", {
                                    source: marketOverview.quoteSource,
                                })
                            }}</span>
                            <span class="market-hero-badge">{{
                                t("watchlist.meta.chartSource", {
                                    source: marketOverview.chartSource,
                                })
                            }}</span>
                            <span class="market-hero-badge">{{
                                t("watchlist.meta.syncedAt", {
                                    time: marketOverview.syncedAt,
                                })
                            }}</span>
                            <span v-if="historySeries" class="market-hero-badge">{{ t("watchlist.meta.cache", { state: historyCacheSummary }) }}</span>
                            <span v-if="historySeries?.cacheExpiresAt" class="market-hero-badge">{{ t("watchlist.meta.cacheFreshUntil", { time: formatDateTime(historySeries.cacheExpiresAt) }) }}</span>
                        </footer>
                    </section>

                    <div class="market-metrics">
                        <article v-if="positionDetail" class="market-position-card" :class="positionDetail.hasPosition ? `tone-${positionDetail.tone}` : ''">
                            <span class="market-pos-label">{{ t("watchlist.position.title") }}</span>
                            <template v-if="positionDetail.hasPosition">
                                <div class="market-pos-main">
                                    <div class="market-pos-stat">
                                        <strong class="market-pos-value">{{ positionDetail.value }}</strong>
                                        <span class="market-pos-stat-label">{{ t("watchlist.position.currentValue") }}</span>
                                    </div>
                                    <div class="market-pos-stat market-pos-stat--right">
                                        <b class="market-pos-pnl">{{ positionDetail.pnl }}</b>
                                        <span class="market-pos-pnl-pct">{{ positionDetail.pnlPct }}</span>
                                    </div>
                                </div>
                                <div class="market-pos-detail">
                                    <div class="market-pos-stat">
                                        <span class="market-pos-stat-label">{{ t("watchlist.position.costPrice") }}</span>
                                        <span class="market-pos-detail-val">{{ positionDetail.costPrice }}</span>
                                    </div>
                                    <div class="market-pos-stat market-pos-stat--right">
                                        <span class="market-pos-stat-label">{{ t("watchlist.position.quantity") }}</span>
                                        <span class="market-pos-detail-val">{{
                                            t("watchlist.position.quantityValue", {
                                                count: formatShares(positionDetail.quantity),
                                            })
                                        }}</span>
                                    </div>
                                </div>
                            </template>
                            <span v-else class="market-pos-empty">{{ t("watchlist.position.empty") }}</span>
                        </article>

                        <article v-for="card in marketCards" :key="card.label" class="metric-strip" :class="`tone-${card.tone}`">
                            <span class="metric-strip-label">{{ card.label }}</span>
                            <strong class="metric-strip-value">{{ card.value }}</strong>
                            <span class="metric-strip-sub">{{ card.sub }}</span>
                        </article>
                    </div>
                </div>

                <div v-else class="market-inspector market-inspector-empty">
                    <span>{{ t("watchlist.selectPrompt") }}</span>
                </div>
            </aside>
        </div>
    </section>
</template>

<style scoped>
.watchlist-module {
    height: 100%;
}

.market-board {
    min-height: 0;
    flex: 1 1 0;
    display: flex;
    flex-direction: column;
    gap: 16px;
}

.market-main {
    position: relative;
    display: flex;
    flex-direction: column;
    flex: 1 1 0;
    min-height: 240px;
}

.market-aside {
    min-height: 0;
    flex: 0 0 auto;
}

.market-inspector {
    display: grid;
    grid-template-columns: 1fr 1fr;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background-color: var(--panel-strong);
    background-image: linear-gradient(150deg, color-mix(in srgb, var(--panel-strong) 90%, var(--accent)) 0%, var(--panel-strong) 40%);
}

.market-inspector-empty {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 120px;
    font-size: 12px;
    color: var(--muted);
}

.market-hero {
    padding: 18px 18px 16px;
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
}

.market-hero-topbar {
    margin-bottom: 16px;
}

.market-hero h4 {
    font: 600 15px/1.2 var(--font-display);
    color: var(--ink);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    margin-bottom: 3px;
}

.market-hero-subline {
    font-size: 12px;
    color: var(--muted);
    margin-bottom: 18px;
}

.market-hero-main {
    display: flex;
    flex-direction: column;
    justify-content: flex-start;
    align-items: flex-start;
    gap: 8px;
    margin-top: 12px;
    margin-bottom: 24px;
}

.market-hero-price {
    font: 500 clamp(18px, 1.35vw, 24px) / 1 var(--font-display);
    color: var(--ink);
    letter-spacing: -0.02em;
    flex: none;
}

.tone-rise .market-hero-price {
    color: var(--rise);
}

.tone-fall .market-hero-price {
    color: var(--fall);
}

.market-hero-delta {
    display: flex;
    flex-direction: row;
    align-items: baseline;
    gap: 6px;
}

.market-hero-delta-label {
    font-size: 11px;
    color: var(--muted);
    line-height: 1.2;
}

.market-hero-delta-val {
    font: 500 clamp(12px, 0.85vw, 15px) / 1.2 var(--font-display);
    color: var(--ink);
    letter-spacing: -0.01em;
}

.tone-rise .market-hero-delta-val {
    color: var(--rise);
}

.tone-fall .market-hero-delta-val {
    color: var(--fall);
}

.market-hero-delta-pct {
    font-size: 12px;
    color: var(--muted);
}

.tone-rise .market-hero-delta-pct {
    color: color-mix(in srgb, var(--rise) 70%, var(--muted));
}

.tone-fall .market-hero-delta-pct {
    color: color-mix(in srgb, var(--fall) 70%, var(--muted));
}

.market-hero-intervals {
    display: flex;
    justify-content: flex-start;
    gap: 4px;
    flex-wrap: wrap;
}

.market-hero-intervals .interval-pill {
    min-height: 32px;
    min-width: 52px;
    padding: 4px 12px;
    font-size: 12px;
    text-align: center;
    white-space: nowrap;
}

.market-hero-foot {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    margin-top: auto;
    padding-top: 14px;
}

.market-hero-badge {
    font-size: 11px;
    color: var(--muted);
    background: color-mix(in srgb, var(--panel-soft) 92%, transparent);
    border: 1px solid color-mix(in srgb, var(--border) 88%, transparent);
    border-radius: 999px;
    padding: 3px 8px;
    white-space: nowrap;
    line-height: 1.6;
}

.interval-pill {
    min-height: 28px;
    padding: 0 10px;
    border-radius: var(--radius-micro);
    border: 1px solid transparent;
    background: transparent;
    color: var(--muted);
    font: 600 12px/1 var(--font-ui);
    cursor: pointer;
}

.interval-pill.active {
    color: var(--ink);
    border-color: var(--border);
    background: var(--panel-strong);
    box-shadow: var(--shadow-soft);
}

.market-metrics {
    display: flex;
    flex-direction: column;
    gap: 0;
    align-content: start;
}

.metric-strip {
    display: grid;
    grid-template-columns: 1fr auto;
    grid-template-rows: auto auto;
    column-gap: 8px;
    padding: 10px 16px;
    border-bottom: 1px solid var(--border);
}

.metric-strip:last-child {
    border-bottom: none;
}

.metric-strip-label {
    grid-column: 1;
    grid-row: 1 / 3;
    align-self: center;
    font-size: 12px;
    color: var(--muted);
    white-space: nowrap;
    letter-spacing: 0.01em;
}

.metric-strip-value {
    grid-column: 2;
    grid-row: 1;
    font: 500 clamp(12px, 0.82vw, 14px) / 1.3 var(--font-display);
    text-align: right;
    color: var(--ink);
}

.metric-strip-sub {
    grid-column: 2;
    grid-row: 2;
    font-size: 11px;
    color: var(--muted);
    text-align: right;
    line-height: 1.3;
}

.tone-rise .metric-strip-value {
    color: var(--rise);
}

.tone-fall .metric-strip-value {
    color: var(--fall);
}

.market-position-card {
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 10px;
    min-height: 138px;
}

.market-pos-label {
    font-size: 11px;
    font-weight: 600;
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: 0.07em;
}

.market-pos-main,
.market-pos-detail {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 8px;
}

.market-pos-detail {
    padding-top: 10px;
    border-top: 1px solid var(--border);
}

.market-pos-stat {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
}

.market-pos-stat--right {
    align-items: flex-end;
    flex: none;
}

.market-pos-stat-label {
    font-size: 11px;
    color: var(--muted);
    line-height: 1.2;
}

.market-pos-value {
    font: 500 15px/1.2 var(--font-display);
    color: var(--ink);
    letter-spacing: -0.01em;
}

.market-pos-pnl {
    font: 500 15px/1.2 var(--font-display);
    color: var(--ink);
    letter-spacing: -0.01em;
}

.tone-rise .market-pos-pnl {
    color: var(--rise);
}

.tone-fall .market-pos-pnl {
    color: var(--fall);
}

.market-pos-pnl-pct {
    font-size: 12px;
    color: var(--muted);
    text-align: right;
}

.tone-rise .market-pos-pnl-pct {
    color: color-mix(in srgb, var(--rise) 65%, var(--muted));
}

.tone-fall .market-pos-pnl-pct {
    color: color-mix(in srgb, var(--fall) 65%, var(--muted));
}

.market-pos-detail-val {
    font: 500 13px/1.3 var(--font-display);
    color: var(--ink);
}

.market-pos-empty {
    font-size: 12px;
    color: var(--muted);
    padding: 4px 0;
}

@media (max-width: 1180px) {
    .market-inspector {
        grid-template-columns: 1fr;
    }

    .market-hero {
        border-right: none;
        border-bottom: 1px solid var(--border);
    }

    .market-metrics {
        flex-direction: row;
        flex-wrap: wrap;
    }

    .metric-strip {
        flex: 1 1 calc(50% - 1px);
    }
}

@media (max-width: 880px) {
    .market-metrics {
        flex-direction: column;
        flex-wrap: nowrap;
    }

    .metric-strip {
        flex: none;
    }
}
</style>
