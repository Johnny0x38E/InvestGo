<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import InputText from "primevue/inputtext";
import Tag from "primevue/tag";

import { formatDateTime, formatMoney, formatPercent, formatRange, formatShortTime, formatUnitPrice } from "../../format";
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
}>();

const searchProxy = computed({
    get: () => props.search,
    set: (value: string) => emit("update:search", value),
});
</script>

<template>
    <section class="module-content">
        <div class="panel-header">
            <div>
                <h3 class="title">自选</h3>
            </div>
            <div class="toolbar-row">
                <InputText v-model="searchProxy" class="search-input" placeholder="代码 / 名称 / 标签 / 备注" />
                <Button size="small" icon="pi pi-plus" label="添加" @click="$emit('add-item')" />
            </div>
        </div>

        <div class="table-shell">
            <table class="watch-table">
                <thead>
                    <tr>
                        <th>标的</th>
                        <th>现价</th>
                        <th>日涨跌</th>
                        <th>浮盈亏</th>
                        <th>日内区间</th>
                        <th>最近同步</th>
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
                                <span>{{ item.openPrice > 0 ? `开盘 ${formatUnitPrice(item.openPrice, item.currency)}` : "区间待同步" }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="value-stack">
                                <strong>{{ formatShortTime(item.quoteUpdatedAt) }}</strong>
                                <span>{{ formatDateTime(item.quoteUpdatedAt) }}</span>
                            </div>
                        </td>
                        <td>
                            <div class="action-stack" @click.stop>
                                <Button size="small" text icon="pi pi-pencil" aria-label="编辑" @click="$emit('edit-item', item)" />
                                <Button size="small" text severity="danger" icon="pi pi-trash" aria-label="删除" @click="$emit('delete-item', item.id)" />
                            </div>
                        </td>
                    </tr>
                </tbody>
                <tbody v-else>
                    <tr>
                        <td colspan="7" class="empty-row">还没有匹配到标的。</td>
                    </tr>
                </tbody>
            </table>
        </div>
    </section>
</template>
