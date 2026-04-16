<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import InputText from "primevue/inputtext";
import Tag from "primevue/tag";

import { formatDateTime, formatMoney, formatPercent, formatRange, formatShortTime, formatUnitPrice } from "../../format";
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
    (event: "select-item", value: string): void;
    (event: "show-dca", value: WatchlistItem): void;
}>();

const searchProxy = computed({
    get: () => props.search,
    set: (value: string) => emit("update:search", value),
});

const { t } = useI18n();
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

        <div class="table-shell">
            <table class="watch-table">
                <thead>
                    <tr>
                        <th>{{ t("watchlist.table.item") }}</th>
                        <th>{{ t("watchlist.table.currentPrice") }}</th>
                        <th>{{ t("watchlist.table.dayChange") }}</th>
                        <th>{{ t("watchlist.table.unrealizedPnL") }}</th>
                        <th>{{ t("watchlist.table.intradayRange") }}</th>
                        <th>{{ t("watchlist.table.lastSynced") }}</th>
                        <th>{{ t("watchlist.table.dca") }}</th>
                        <th></th>
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
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatShortTime(item.quoteUpdatedAt) }}</strong>
                                <span>{{ formatDateTime(item.quoteUpdatedAt) }}</span>
                            </div>
                        </td>
                        <td class="watch-table-cell-dca">
                            <div class="action-stack table-action-stack table-action-stack-centered">
                                <Button
                                    v-if="item.dcaEntries?.length"
                                    size="small"
                                    outlined
                                    icon="pi pi-chart-line"
                                    :label="t('watchlist.dcaEntries', { count: item.dcaEntries.length })"
                                    class="dca-list-button"
                                    @click.stop="$emit('show-dca', item)"
                                />
                                <span v-else style="color: var(--muted); font-size: 12px">—</span>
                            </div>
                        </td>
                        <td class="table-action-cell">
                            <div class="action-stack table-action-stack" @click.stop>
                                <Button size="small" text rounded icon="pi pi-pencil" :aria-label="t('watchlist.aria.edit')" @click="$emit('edit-item', item)" />
                                <Button size="small" text rounded severity="danger" icon="pi pi-trash" :aria-label="t('watchlist.aria.delete')" @click="$emit('delete-item', item.id)" />
                            </div>
                        </td>
                    </tr>
                </tbody>
                <tbody v-else>
                    <tr>
                        <td colspan="8" class="empty-row">{{ t("watchlist.empty") }}</td>
                    </tr>
                </tbody>
            </table>
        </div>
    </section>
</template>
