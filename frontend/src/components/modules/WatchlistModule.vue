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
const lastSyncedAt = computed(() => {
    const timestamps = props.filteredItems.map((item) => item.quoteUpdatedAt).filter(Boolean) as string[];
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
                <h3 class="title">{{ t("watchlist.title") }}</h3>
            </div>
            <div class="toolbar-row">
                <InputText v-model="searchProxy" class="search-input" :placeholder="t('watchlist.searchPlaceholder')" />
                <Button size="small" icon="pi pi-plus" :label="t('common.add')" @click="$emit('add-item')" />
            </div>
        </div>

        <div class="table-meta-row">
            <span>{{ t("watchlist.meta.results", { count: filteredItems.length }) }}</span>
            <span>{{ t("watchlist.meta.lastSynced", { time: lastSyncedAt ? formatDateTime(lastSyncedAt) : t("common.notAvailable") }) }}</span>
        </div>

        <div class="table-shell">
            <table class="watch-table">
                <thead>
                    <tr>
                        <th>{{ t("watchlist.table.item") }}</th>
                        <th>{{ t("watchlist.table.currentPrice") }}</th>
                        <th>{{ t("watchlist.table.dayChange") }}</th>
                        <th>{{ t("watchlist.table.positionPnL") }}</th>
                        <th>{{ t("watchlist.table.intradayRange") }}</th>
                        <th class="watch-table-sticky watch-table-sticky-dca">{{ t("watchlist.table.dca") }}</th>
                        <th class="watch-table-sticky watch-table-sticky-actions"></th>
                    </tr>
                </thead>
                <tbody v-if="filteredItems.length">
                    <tr v-for="item in filteredItems" :key="item.id" :class="{ selected: selectedItemId === item.id }" @click="$emit('select-item', item.id)">
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
                                <strong :class="item.change >= 0 ? 'tone-rise' : 'tone-fall'">{{ formatMoney(item.change, true) }}</strong>
                                <span :class="item.changePercent >= 0 ? 'tone-rise' : 'tone-fall'">{{ formatPercent(item.changePercent) }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong :class="item.currentPrice >= item.costPrice ? 'tone-rise' : 'tone-fall'">
                                    {{ formatMoney(item.quantity * item.currentPrice - item.quantity * item.costPrice, true) }}
                                </strong>
                                <span :class="item.currentPrice >= item.costPrice ? 'tone-rise' : 'tone-fall'">
                                    {{ formatPercent(item.costPrice > 0 ? ((item.currentPrice - item.costPrice) / item.costPrice) * 100 : 0) }}
                                </span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatRange(item.dayLow, item.dayHigh, item.currency) }}</strong>
                                <span>{{ item.openPrice > 0 ? t("watchlist.openPrice", { price: formatUnitPrice(item.openPrice, item.currency) }) : t("watchlist.rangePending") }}</span>
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
                                    :aria-label="t('watchlist.dcaEntries', { count: item.dcaEntries.length })"
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
                                    :aria-label="item.pinnedAt ? t('watchlist.aria.unpin') : t('watchlist.aria.pin')"
                                    @click="$emit('toggle-pin', item)"
                                />
                                <Button size="small" text rounded icon="pi pi-pencil" :aria-label="t('watchlist.aria.edit')" @click="$emit('edit-item', item)" />
                                <Button size="small" text rounded severity="danger" icon="pi pi-trash" :aria-label="t('watchlist.aria.delete')" @click="$emit('delete-item', item.id)" />
                            </div>
                        </td>
                    </tr>
                </tbody>
                <tbody v-else>
                    <tr>
                        <td colspan="7" class="empty-row">{{ t("watchlist.empty") }}</td>
                    </tr>
                </tbody>
            </table>
        </div>
    </section>
</template>

<style scoped>
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
    width: 132px;
}

.watch-table th.watch-table-sticky-dca,
.watch-table td.watch-table-sticky-dca {
    right: 92px;
    width: 88px;
    min-width: 88px;
    max-width: 88px;
    text-align: left;
}

.watch-table th.watch-table-sticky-actions,
.watch-table td.watch-table-sticky-actions {
    right: 0;
    width: 92px;
    min-width: 92px;
    max-width: 92px;
}

.watch-table td.watch-table-cell-dca,
.watch-table td.table-action-cell {
    padding-left: 6px;
    padding-right: 6px;
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
    padding: 0.15rem 0.45rem;
    gap: 0.25rem;
    justify-content: center;
    white-space: nowrap;
}

.watch-table :deep(.dca-list-button .p-button-icon),
.watch-table :deep(.dca-list-button .p-button-label) {
    font-size: 11px;
}

.watch-table .table-action-cell :deep(.p-button) {
    width: 24px;
    height: 24px;
    min-width: 24px;
    padding: 0;
}

.watch-table .table-action-cell :deep(.p-button.is-pinned-action) {
    color: var(--accent-strong);
    background: color-mix(in srgb, var(--accent-soft) 94%, var(--panel-strong));
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 24%, var(--border));
}
</style>
