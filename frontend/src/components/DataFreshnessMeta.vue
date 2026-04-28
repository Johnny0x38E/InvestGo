<script setup lang="ts">
import { ref } from "vue";
import Button from "primevue/button";
import Popover, { type PopoverMethods } from "primevue/popover";

import { useI18n } from "../i18n";

defineProps<{
    summary: string;
    details: Array<{
        label: string;
        value: string;
    }>;
}>();

const { t } = useI18n();
const popover = ref<PopoverMethods | null>(null);
</script>

<template>
    <div class="data-freshness-meta">
        <span class="data-freshness-summary">{{ summary }}</span>
        <Button
            text
            rounded
            size="small"
            icon="pi pi-info-circle"
            class="data-freshness-button"
            :aria-label="t('common.dataDetails')"
            :title="t('common.dataDetails')"
            @click="popover?.toggle($event)"
        />
        <Popover ref="popover">
            <div class="data-freshness-popover">
                <div v-for="entry in details" :key="entry.label" class="data-freshness-row">
                    <span>{{ entry.label }}</span>
                    <strong>{{ entry.value }}</strong>
                </div>
            </div>
        </Popover>
    </div>
</template>

<style scoped>
.data-freshness-meta {
    min-width: 0;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    max-width: 100%;
}

.data-freshness-summary {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 11px;
    color: var(--muted);
    background: color-mix(in srgb, var(--panel-soft) 92%, transparent);
    border: 1px solid color-mix(in srgb, var(--border) 88%, transparent);
    border-radius: 999px;
    padding: 3px 8px;
    line-height: 1.6;
}

.data-freshness-button {
    flex: 0 0 auto;
    width: 28px;
    height: 28px;
    color: var(--muted);
}

.data-freshness-popover {
    min-width: 260px;
    max-width: min(360px, 80vw);
    display: grid;
    gap: 9px;
    padding: 2px;
}

.data-freshness-row {
    display: grid;
    grid-template-columns: minmax(76px, auto) minmax(0, 1fr);
    gap: 14px;
    align-items: baseline;
}

.data-freshness-row span {
    font-size: 11px;
    color: var(--muted);
    white-space: nowrap;
}

.data-freshness-row strong {
    min-width: 0;
    overflow-wrap: anywhere;
    font: 500 12px/1.4 var(--font-ui);
    color: var(--ink);
    text-align: right;
}
</style>
