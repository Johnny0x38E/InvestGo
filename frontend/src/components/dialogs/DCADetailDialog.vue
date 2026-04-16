<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Dialog from "primevue/dialog";

import { formatMoney, formatNumber, formatPercent, formatUnitPrice, resolvedLocale } from "../../format";
import { useI18n } from "../../i18n";
import type { WatchlistItem } from "../../types";

const props = defineProps<{
    visible: boolean;
    item: WatchlistItem | null;
}>();

const emit = defineEmits<{
    (event: "update:visible", value: boolean): void;
    (event: "edit"): void;
}>();

const visibleProxy = computed({
    get: () => props.visible,
    set: (v: boolean) => emit("update:visible", v),
});

const { t } = useI18n();

const dialogHeader = computed(() => {
    if (!props.item) return t("dialogs.dcaDetail.title");
    const name = props.item.name || props.item.symbol;
    return t("dialogs.dcaDetail.titleWithName", { name });
});

// 过滤出有效的定投条目（amount 和 shares 都必须 > 0）
const entries = computed(() => (props.item?.dcaEntries ?? []).filter((e) => e.amount > 0 && e.shares > 0));

const summary = computed(() => {
    const valid = entries.value;
    const totalAmount = valid.reduce((s, e) => s + e.amount, 0);
    const totalShares = valid.reduce((s, e) => s + e.shares, 0);
    const totalFees = valid.reduce((s, e) => s + (e.fee ?? 0), 0);
    // 与后端 sanitiseItem 保持一致：优先使用手动买入价
    let totalEffectiveCost = 0;
    for (const e of valid) {
        const price = e.price ?? 0;
        const fee = e.fee ?? 0;
        if (price > 0) {
            totalEffectiveCost += price * e.shares;
        } else {
            totalEffectiveCost += Math.max(e.amount - fee, 0);
        }
    }
    const avgCost = totalShares > 0 ? totalEffectiveCost / totalShares : 0;
    const curPrice = props.item?.currentPrice ?? 0;
    const currentValue = totalShares * curPrice;
    const pnl = curPrice > 0 ? currentValue - totalEffectiveCost : null;
    const pnlPct = totalEffectiveCost > 0 && pnl !== null ? (pnl / totalEffectiveCost) * 100 : null;
    return {
        count: valid.length,
        totalAmount,
        totalShares,
        totalFees,
        avgCost,
        currentValue,
        pnl,
        pnlPct,
        hasCurPrice: curPrice > 0,
    };
});

function buyPrice(entry: { price?: number; fee?: number; amount: number; shares: number }): string {
    if (!props.item) return "—";
    let p: number;
    if (entry.price && entry.price > 0) {
        p = entry.price;
    } else if (entry.shares > 0) {
        const net = Math.max(entry.amount - (entry.fee ?? 0), 0);
        p = net / entry.shares;
    } else {
        p = 0;
    }
    return p > 0 ? formatUnitPrice(p, props.item.currency) : "—";
}

function formatEntryDate(iso: string): string {
    try {
        return new Intl.DateTimeFormat(resolvedLocale(), {
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
        }).format(new Date(iso));
    } catch {
        return iso.substring(0, 10);
    }
}

function pnlTone(v: number | null): string {
    if (v === null) return "";
    return v > 0 ? "tone-rise" : v < 0 ? "tone-fall" : "";
}
</script>

<template>
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="dialogHeader" :style="{ width: '860px' }" class="desk-dialog">
        <!-- ── 汇总栏 ─────────────────────────────────────────────────────── -->
        <div v-if="summary.count > 0" class="dca-summary-bar" style="margin-bottom: 20px">
            <div class="dca-summary-cell">
                <span class="dca-summary-label">{{ t("dialogs.dcaDetail.summary.count") }}</span>
                <span class="dca-summary-value">{{ t("dialogs.dcaDetail.summary.countValue", { count: summary.count }) }}</span>
            </div>
            <div class="dca-summary-cell">
                <span class="dca-summary-label">{{ t("dialogs.dcaDetail.summary.totalInvested") }}</span>
                <span class="dca-summary-value">{{ formatUnitPrice(summary.totalAmount, item?.currency ?? "") }}</span>
            </div>
            <div v-if="summary.totalFees > 0" class="dca-summary-cell">
                <span class="dca-summary-label">{{ t("dialogs.dcaDetail.summary.totalFees") }}</span>
                <span class="dca-summary-value">{{ formatUnitPrice(summary.totalFees, item?.currency ?? "") }}</span>
            </div>
            <div class="dca-summary-cell">
                <span class="dca-summary-label">{{ t("dialogs.dcaDetail.summary.totalShares") }}</span>
                <span class="dca-summary-value">{{ formatNumber(summary.totalShares, 4) }}</span>
            </div>
            <div class="dca-summary-cell">
                <span class="dca-summary-label">{{ t("dialogs.dcaDetail.summary.weightedAvgPrice") }}</span>
                <span class="dca-summary-value">{{ formatUnitPrice(summary.avgCost, item?.currency ?? "") }}</span>
            </div>
            <template v-if="summary.hasCurPrice">
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">{{ t("dialogs.dcaDetail.summary.currentValue") }}</span>
                    <span class="dca-summary-value">{{ formatUnitPrice(summary.currentValue, item?.currency ?? "") }}</span>
                </div>
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">{{ t("dialogs.dcaDetail.summary.unrealizedPnL") }}</span>
                    <span class="dca-summary-value" :class="pnlTone(summary.pnl)">
                        {{ formatMoney(summary.pnl ?? 0, true) }}
                        <span style="font-weight: 400; font-size: 11px; margin-left: 4px">
                            {{ summary.pnlPct !== null ? formatPercent(summary.pnlPct) : "" }}
                        </span>
                    </span>
                </div>
            </template>
        </div>

        <!-- ── 明细表格 ────────────────────────────────────────────────────── -->
        <div v-if="entries.length > 0" class="dca-detail-table">
            <!-- 表头 -->
            <div class="dca-detail-head">
                <span class="dca-col-label dca-seq-col">#</span>
                <span class="dca-col-label">{{ t("dialogs.dcaDetail.table.date") }}</span>
                <span class="dca-col-label dca-num-col">{{ t("dialogs.dcaDetail.table.investedAmount") }}</span>
                <span class="dca-col-label dca-num-col">{{ t("dialogs.dcaDetail.table.boughtShares") }}</span>
                <span class="dca-col-label dca-num-col">{{ t("dialogs.dcaDetail.table.buyPrice") }}</span>
                <span class="dca-col-label dca-num-col">{{ t("dialogs.dcaDetail.table.fee") }}</span>
                <span class="dca-col-label">{{ t("dialogs.dcaDetail.table.note") }}</span>
            </div>

            <!-- 数据行 -->
            <div v-for="(entry, idx) in entries" :key="entry.id" class="dca-detail-row">
                <span class="dca-detail-cell dca-seq-col dca-seq">{{ idx + 1 }}</span>
                <span class="dca-detail-cell">{{ formatEntryDate(entry.date) }}</span>
                <span class="dca-detail-cell dca-num-col">{{ formatUnitPrice(entry.amount, item?.currency ?? "") }}</span>
                <span class="dca-detail-cell dca-num-col">{{ formatNumber(entry.shares, 4) }}</span>
                <span class="dca-detail-cell dca-num-col">{{ buyPrice(entry) }}</span>
                <span class="dca-detail-cell dca-num-col">{{ entry.fee && entry.fee > 0 ? formatUnitPrice(entry.fee, item?.currency ?? "") : "—" }}</span>
                <span class="dca-detail-cell dca-note-col">{{ entry.note || "—" }}</span>
            </div>
        </div>

        <!-- 空状态 -->
        <div v-else class="dca-empty-hint">{{ t("dialogs.dcaDetail.validEmpty") }}</div>

        <!-- ── 底部操作 ─────────────────────────────────────────────────────── -->
        <template #footer>
            <Button size="small" text :label="t('common.close')" @click="visibleProxy = false" />
            <Button size="small" icon="pi pi-pencil" :label="t('dialogs.dcaDetail.editRecords')" @click="$emit('edit')" />
        </template>
    </Dialog>
</template>
