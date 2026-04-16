<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from "vue";
import Button from "primevue/button";
import Tag from "primevue/tag";

import { appendClientLog } from "../../devlog";
import { formatDateTime, formatUnitPrice } from "../../format";
import { useI18n } from "../../i18n";
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
const { t } = useI18n();

function itemName(itemId: string): string {
    return itemMap.value.get(itemId)?.name || t("alerts.deletedItem");
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
                <h3 class="title">{{ t("alerts.title") }}</h3>
            </div>
            <div class="toolbar-row">
                <Button size="small" icon="pi pi-plus" :label="t('common.add')" @click="$emit('add-alert')" :disabled="!safeItems.length" />
            </div>
        </div>

        <div v-if="safeAlerts.length" class="alert-grid">
            <article v-for="alert in safeAlerts" :key="alert.id" class="alert-card">
                <div class="alert-head">
                    <div>
                        <strong>{{ alert.name }}</strong>
                        <span>{{ itemName(alert.itemId) }}</span>
                    </div>
                    <Tag :severity="alert.triggered ? 'danger' : alert.enabled ? 'success' : 'secondary'" :value="alert.triggered ? t('alerts.triggered') : alert.enabled ? t('alerts.monitoring') : t('alerts.disabled')" rounded />
                </div>
                <div class="alert-pills">
                    <Tag :value="alert.condition === 'above' ? t('alerts.above', { value: formatUnitPrice(alert.threshold, itemCurrency(alert.itemId)) }) : t('alerts.below', { value: formatUnitPrice(alert.threshold, itemCurrency(alert.itemId)) })" />
                    <Tag v-if="alert.lastTriggeredAt" severity="warn" :value="t('alerts.lastTriggered', { time: formatDateTime(alert.lastTriggeredAt) })" />
                </div>
                <div class="alert-actions">
                    <span>{{ t("alerts.updatedAt", { time: formatDateTime(alert.updatedAt) }) }}</span>
                    <div class="action-stack table-action-stack" @click.stop>
                        <Button size="small" text rounded icon="pi pi-pencil" :aria-label="t('alerts.aria.edit')" @click="$emit('edit-alert', alert)" />
                        <Button size="small" text rounded severity="danger" icon="pi pi-trash" :aria-label="t('alerts.aria.delete')" @click="$emit('delete-alert', alert.id)" />
                    </div>
                </div>
            </article>
        </div>
        <div v-else class="empty-card">{{ t("alerts.empty") }}</div>
    </section>
</template>
