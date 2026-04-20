<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Tag from "primevue/tag";

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

<style scoped>
.alert-pills {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
}

.table-action-stack {
    display: flex;
    align-items: center;
    gap: 6px;
}

.empty-card {
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-panel);
    padding: 20px;
    color: var(--muted);
    line-height: 1.6;
    background: rgba(148, 163, 184, 0.06);
}

.alert-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
}

.alert-card {
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: linear-gradient(180deg, color-mix(in srgb, var(--panel-soft) 92%, var(--accent-soft)) 0%, var(--panel-strong) 100%);
    padding: 14px;
    display: grid;
    gap: 10px;
    box-shadow: var(--shadow-soft);
}

.alert-card:hover {
    border-color: color-mix(in srgb, var(--accent) 24%, var(--border));
}

.alert-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
}

.alert-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
}

.alert-head strong {
    display: block;
    font-size: 13px;
    margin-bottom: 4px;
}

.alert-head > div > span,
.alert-actions > span {
    color: var(--muted);
    font-size: 11px;
}

@media (max-width: 880px) {
    .alert-head,
    .alert-actions {
        align-items: stretch;
        flex-direction: column;
    }
}
</style>
