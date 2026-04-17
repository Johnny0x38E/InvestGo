<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import InputText from "primevue/inputtext";
import Tag from "primevue/tag";

import { formatDateTime, formatMoney, formatPercent, formatRange, formatUnitPrice } from "../../format";
import { useI18n } from "../../i18n";
import type { WatchlistItem } from "../../types";

const props = defineProps<{
    search: string;
    filteredItems: WatchlistItem[];
    selectedItemId: string;
}>();

const emit = defineEmits<{
    (event: "update:search", value: string): void;
    (event: "add-item"): void;
    (event: "edit-item", value: WatchlistItem): void;
    (event: "delete-item", value: string): void;
    (event: "toggle-pin", value: WatchlistItem): void;
    (event: "select-item", value: string): void;
    (event: "show-dca", value: WatchlistItem): void;
}>();

const searchProxy = computed({
    get: () => props.search,
    set: (value: string) => emit("update:search", value),
});

const { t } = useI18n();
const holdingsItems = computed(() => props.filteredItems.filter((item) => item.position?.hasPosition));

const lastSyncedAt = computed(() => {
    const timestamps = holdingsItems.value.map((item) => item.quoteUpdatedAt).filter(Boolean) as string[];
    if (!timestamps.length) {
        return "";
    }

    return timestamps.reduce((latest, current) => (new Date(current).getTime() > new Date(latest).getTime() ? current : latest));
});
</script>

<template>
    <section class="module-content">
        <div class="panel-header">
            <div>
                <h3 class="title">{{ t("holdings.title") }}</h3>
            </div>
            <div class="toolbar-row">
                <InputText v-model="searchProxy" class="search-input" :placeholder="t('holdings.searchPlaceholder')" />
                <Button size="small" icon="pi pi-plus" :label="t('holdings.addPosition')" @click="$emit('add-item')" />
            </div>
        </div>

        <div class="table-meta-row">
            <span>{{ t("holdings.meta.results", { count: holdingsItems.length }) }}</span>
            <span>{{ t("holdings.meta.lastSynced", { time: lastSyncedAt ? formatDateTime(lastSyncedAt) : t("common.notAvailable") }) }}</span>
        </div>

        <div class="table-shell">
            <table class="watch-table">
                <thead>
                    <tr>
                        <th>{{ t("holdings.table.item") }}</th>
                        <th>{{ t("holdings.table.currentPrice") }}</th>
                        <th>{{ t("holdings.table.dayChange") }}</th>
                        <th>{{ t("holdings.table.positionPnL") }}</th>
                        <th>{{ t("holdings.table.intradayRange") }}</th>
                        <th class="watch-table-sticky watch-table-sticky-dca">{{ t("holdings.table.dca") }}</th>
                        <th class="watch-table-sticky watch-table-sticky-actions"></th>
                    </tr>
                </thead>
                <tbody v-if="holdingsItems.length">
                    <tr v-for="item in holdingsItems" :key="item.id" :class="{ selected: selectedItemId === item.id }" @click="$emit('select-item', item.id)">
                        <td>
                            <div class="item-block">
                                <strong>{{ item.name || item.symbol }}</strong>
                                <span>{{ item.market }} · {{ item.symbol }}</span>
                                <div class="tag-row">
                                    <Tag v-for="tag in item.tags" :key="tag" :value="tag" rounded />
                                </div>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatUnitPrice(item.currentPrice, item.currency) }}</strong>
                                <span>{{ item.quoteSource || "manual" }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong :class="item.change > 0 ? 'tone-rise' : item.change < 0 ? 'tone-fall' : ''">{{ formatMoney(item.change, true) }}</strong>
                                <span :class="item.changePercent > 0 ? 'tone-rise' : item.changePercent < 0 ? 'tone-fall' : ''">{{ formatPercent(item.changePercent) }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong :class="(item.position?.unrealisedPnL ?? 0) > 0 ? 'tone-rise' : (item.position?.unrealisedPnL ?? 0) < 0 ? 'tone-fall' : ''">
                                    {{ formatMoney(item.position?.unrealisedPnL ?? 0, true) }}
                                </strong>
                                <span :class="(item.position?.unrealisedPnLPct ?? 0) > 0 ? 'tone-rise' : (item.position?.unrealisedPnLPct ?? 0) < 0 ? 'tone-fall' : ''">
                                    {{ formatPercent(item.position?.unrealisedPnLPct ?? 0) }}
                                </span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatRange(item.dayLow, item.dayHigh, item.currency) }}</strong>
                                <span>{{ item.openPrice > 0 ? t("holdings.openPrice", { price: formatUnitPrice(item.openPrice, item.currency) }) : t("holdings.rangePending") }}</span>
                            </div>
                        </td>
                        <td class="watch-table-cell-dca watch-table-sticky watch-table-sticky-dca">
                            <div class="action-stack table-action-stack table-action-stack-centered">
                                <Button
                                    v-if="item.dcaEntries?.length"
                                    size="small"
                                    text
                                    rounded
                                    icon="pi pi-chart-line"
                                    :label="String(item.dcaEntries.length)"
                                    :aria-label="t('holdings.dcaEntries', { count: item.dcaEntries.length })"
                                    class="dca-list-button"
                                    @click.stop="$emit('show-dca', item)"
                                />
                                <span v-else class="dca-empty-placeholder">—</span>
                            </div>
                        </td>
                        <td class="table-action-cell watch-table-sticky watch-table-sticky-actions">
                            <div class="action-stack table-action-stack" @click.stop>
                                <Button
                                    size="small"
                                    text
                                    rounded
                                    icon="pi pi-thumbtack"
                                    :class="{ 'is-pinned-action': Boolean(item.pinnedAt) }"
                                    :aria-label="item.pinnedAt ? t('holdings.aria.unpin') : t('holdings.aria.pin')"
                                    @click="$emit('toggle-pin', item)"
                                />
                                <Button size="small" text rounded icon="pi pi-pencil" :aria-label="t('holdings.aria.edit')" @click="$emit('edit-item', item)" />
                                <Button size="small" text rounded severity="danger" icon="pi pi-trash" :aria-label="t('holdings.aria.delete')" @click="$emit('delete-item', item.id)" />
                            </div>
                        </td>
                    </tr>
                </tbody>
                <tbody v-else>
                    <tr>
                        <td colspan="7" class="empty-row">{{ t("holdings.empty") }}</td>
                    </tr>
                </tbody>
            </table>
        </div>
    </section>
</template>

<style scoped>
.watch-table td {
    vertical-align: top;
}

.watch-table td.watch-table-cell-dca,
.watch-table td.table-action-cell {
    vertical-align: middle;
}

.watch-table th:first-child,
.watch-table td:first-child {
    width: 34%;
}

.watch-table th:nth-child(2),
.watch-table td:nth-child(2) {
    width: 106px;
}

.watch-table th:nth-child(3),
.watch-table td:nth-child(3) {
    width: 108px;
}

.watch-table th:nth-child(4),
.watch-table td:nth-child(4) {
    width: 108px;
}

.watch-table th:nth-child(5),
.watch-table td:nth-child(5) {
    width: 160px;
    white-space: nowrap;
}

.watch-table th.watch-table-sticky-dca,
.watch-table td.watch-table-sticky-dca {
    right: 112px;
    width: 88px;
    min-width: 88px;
    max-width: 88px;
    text-align: left;
}

.watch-table th.watch-table-sticky-actions,
.watch-table td.watch-table-sticky-actions {
    right: 0;
    width: 124px;
    min-width: 124px;
    max-width: 124px;
}

.watch-table td.watch-table-cell-dca,
.watch-table td.table-action-cell {
    padding-left: 14px;
    padding-right: 14px;
}

.watch-table th.watch-table-sticky-dca {
    padding-left: 18px;
    padding-right: 14px;
}

.watch-table .table-action-stack {
    width: 100%;
    justify-content: center;
    gap: 6px;
}

.watch-table .table-action-stack-centered {
    width: 100%;
    justify-content: flex-start;
}

.watch-table .dca-empty-placeholder {
    display: inline-flex;
    align-items: center;
    min-height: 22px;
    padding: 0 0.45rem;
    color: var(--muted);
    font-size: 12px;
    line-height: 1;
}

.watch-table :deep(.dca-list-button.p-button) {
    min-width: 0;
    padding: 0.42rem 0.65rem;
    gap: 0.3rem;
    justify-content: center;
    white-space: nowrap;
    border-radius: var(--radius-control);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 38%, var(--border));
}

.watch-table :deep(.dca-list-button .p-button-icon),
.watch-table :deep(.dca-list-button .p-button-label) {
    font-size: 13px;
    color: var(--accent);
}

.watch-table :deep(.dca-list-button.p-button:hover) {
    background: color-mix(in srgb, var(--accent-soft) 72%, transparent);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 60%, var(--border));
}

.watch-table .table-action-cell :deep(.p-button) {
    width: 28px;
    height: 28px;
    min-width: 28px;
    padding: 0;
}

.watch-table .table-action-cell :deep(.p-button.is-pinned-action) {
    color: var(--accent-strong);
    background: color-mix(in srgb, var(--accent-soft) 94%, var(--panel-strong));
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 24%, var(--border));
}
</style>
