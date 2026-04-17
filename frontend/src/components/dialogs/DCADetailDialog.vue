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

// Backend sanitiseItem guarantees only valid entries are persisted; the filter here is a defensive guard.
const entries = computed(() => (props.item?.dcaEntries ?? []).filter((e) => e.amount > 0 && e.shares > 0));

const summary = computed(() => props.item?.dcaSummary ?? null);

function buyPrice(entry: { effectivePrice?: number }): string {
    if (!props.item) return "—";
    const p = "effectivePrice" in entry && typeof entry.effectivePrice === "number" ? entry.effectivePrice : 0;
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
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="dialogHeader" :style="{ width: '1100px' }" class="desk-dialog">
        <!-- DCA summary bar -->
        <div v-if="summary && summary.count > 0" class="dca-summary-bar" style="margin-bottom: 20px">
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
                <span class="dca-summary-value">{{ formatUnitPrice(summary.averageCost, item?.currency ?? "") }}</span>
            </div>
            <template v-if="summary.hasCurrentPrice">
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">{{ t("dialogs.dcaDetail.summary.currentValue") }}</span>
                    <span class="dca-summary-value">{{ formatUnitPrice(summary.currentValue, item?.currency ?? "") }}</span>
                </div>
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">{{ t("dialogs.dcaDetail.summary.positionPnL") }}</span>
                    <span class="dca-summary-value" :class="pnlTone(summary.pnl)">
                        {{ formatMoney(summary.pnl ?? 0, true) }}
                        <span style="font-weight: 400; font-size: 11px; margin-left: 4px">{{ formatPercent(summary.pnlPct) }}</span>
                    </span>
                </div>
            </template>
        </div>

        <!-- DCA detail table -->
        <div v-if="entries.length > 0" class="dca-detail-table">
            <!-- Header row -->
            <div class="dca-detail-head">
                <span class="dca-col-label dca-seq-col">#</span>
                <span class="dca-col-label">{{ t("dialogs.dcaDetail.table.date") }}</span>
                <span class="dca-col-label dca-num-col">{{ t("dialogs.dcaDetail.table.investedAmount") }}</span>
                <span class="dca-col-label dca-num-col">{{ t("dialogs.dcaDetail.table.boughtShares") }}</span>
                <span class="dca-col-label dca-num-col">{{ t("dialogs.dcaDetail.table.buyPrice") }}</span>
                <span class="dca-col-label dca-num-col">{{ t("dialogs.dcaDetail.table.fee") }}</span>
                <span class="dca-col-label">{{ t("dialogs.dcaDetail.table.note") }}</span>
            </div>

            <!-- Data rows -->
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

        <!-- Empty state -->
        <div v-else class="dca-empty-hint">{{ t("dialogs.dcaDetail.validEmpty") }}</div>

        <!-- Footer actions -->
        <template #footer>
            <Button size="small" text :label="t('common.close')" @click="visibleProxy = false" />
            <Button size="small" icon="pi pi-pencil" :label="t('dialogs.dcaDetail.editRecords')" @click="$emit('edit')" />
        </template>
    </Dialog>
</template>

<style scoped>
.dca-summary-bar {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 1px;
    background: var(--border);
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    overflow: hidden;
    margin-top: 16px;
}

.dca-summary-cell {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 10px 10px;
    background: var(--panel-strong);
}

.dca-summary-label {
    font: 500 10px/1 var(--font-ui);
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    white-space: nowrap;
}

.dca-summary-value {
    font: 400 13px/1.2 var(--font-display);
    color: var(--ink);
    white-space: nowrap;
}

.dca-summary-value.tone-rise {
    color: var(--rise);
}

.dca-summary-value.tone-fall {
    color: var(--fall);
}

.dca-empty-hint {
    padding: 24px 0 8px;
    text-align: center;
    font-size: 12px;
    color: var(--muted);
    line-height: 1.7;
}

.dca-detail-table {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
    overflow: hidden;
}

.dca-detail-head,
.dca-detail-row {
    display: grid;
    grid-template-columns: 40px 148px 148px 128px 128px 96px 1fr;
    gap: 0;
    padding: 0 4px;
}

.dca-detail-head {
    padding: 7px 4px;
    background: var(--panel-soft);
    border-bottom: 1px solid var(--border);
}

.dca-col-label {
    font: 600 11px/1 var(--font-ui);
    color: var(--muted);
    white-space: nowrap;
}

.dca-detail-head .dca-col-label {
    display: flex;
    align-items: center;
    justify-content: flex-start;
    padding: 0 8px;
    text-align: left;
}

.dca-detail-row {
    border-bottom: 1px solid var(--border);
}

.dca-detail-row:last-child {
    border-bottom: none;
}

.dca-detail-row:hover {
    background: var(--selection-bg);
}

.dca-detail-cell {
    display: flex;
    align-items: center;
    justify-content: flex-start;
    padding: 9px 8px;
    font: 500 13px/1.2 var(--font-display);
    color: var(--ink);
    text-align: left;
}

.dca-seq-col {
    justify-content: flex-start;
}

.dca-num-col {
    justify-content: flex-start;
    text-align: left;
}

.dca-seq {
    font: 500 11px/1 var(--font-ui);
    color: var(--muted);
}

.dca-note-col {
    color: var(--muted);
    font-size: 12px;
    font-weight: 400;
    overflow-wrap: anywhere;
}
</style>
