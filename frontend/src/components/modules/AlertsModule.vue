<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from "vue";
import Button from "primevue/button";
import Tag from "primevue/tag";

import { appendClientLog } from "../../devlog";
import { formatDateTime, formatUnitPrice } from "../../format";
import type { AlertRule, WatchlistItem } from "../../types";

const props = defineProps<{
    alerts: AlertRule[];
    items: WatchlistItem[];
}>();

defineEmits<{
    (event: "add-alert"): void;
    (event: "edit-alert", value: AlertRule): void;
    (event: "delete-alert", value: string): void;
}>();

const safeAlerts = computed(() => props.alerts ?? []);
const safeItems = computed(() => props.items ?? []);
const itemMap = computed(() => new Map(safeItems.value.map((item) => [item.id, item])));

function itemName(itemId: string): string {
    return itemMap.value.get(itemId)?.name || "已删除标的";
}

function itemCurrency(itemId: string): string {
    return itemMap.value.get(itemId)?.currency || "CNY";
}

onMounted(() => {
    appendClientLog("info", "alerts", `alerts module mounted (alerts=${safeAlerts.value.length}, items=${safeItems.value.length})`);
});

onBeforeUnmount(() => {
    appendClientLog("info", "alerts", "alerts module unmounted");
});
</script>

<template>
    <section class="module-content">
        <div class="panel-header">
            <div>
                <h3 class="title">提醒</h3>
            </div>
            <div class="toolbar-row">
                <Button size="small" icon="pi pi-plus" label="添加" @click="$emit('add-alert')" :disabled="!safeItems.length" />
            </div>
        </div>

        <div v-if="safeAlerts.length" class="alert-grid">
            <article v-for="alert in safeAlerts" :key="alert.id" class="alert-card">
                <div class="alert-head">
                    <div>
                        <strong>{{ alert.name }}</strong>
                        <span>{{ itemName(alert.itemId) }}</span>
                    </div>
                    <Tag :severity="alert.triggered ? 'danger' : alert.enabled ? 'success' : 'secondary'" :value="alert.triggered ? '已触发' : alert.enabled ? '监控中' : '已停用'" rounded />
                </div>
                <div class="alert-pills">
                    <Tag :value="`${alert.condition === 'above' ? '高于' : '低于'} ${formatUnitPrice(alert.threshold, itemCurrency(alert.itemId))}`" />
                    <Tag v-if="alert.lastTriggeredAt" severity="warn" :value="`最近触发 ${formatDateTime(alert.lastTriggeredAt)}`" />
                </div>
                <div class="alert-actions">
                    <span>更新时间 {{ formatDateTime(alert.updatedAt) }}</span>
                    <div class="action-stack table-action-stack" @click.stop>
                        <Button size="small" text icon="pi pi-pencil" aria-label="编辑" class="table-action-button" @click="$emit('edit-alert', alert)" />
                        <Button
                            size="small"
                            text
                            severity="danger"
                            icon="pi pi-trash"
                            aria-label="删除"
                            class="table-action-button table-action-button-danger"
                            @click="$emit('delete-alert', alert.id)"
                        />
                    </div>
                </div>
            </article>
        </div>
        <div v-else class="empty-card">还没有提醒规则。</div>
    </section>
</template>
