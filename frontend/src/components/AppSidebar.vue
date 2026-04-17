<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";

import { getHotMarketOptions, getModuleTabs } from "../constants";
import { useI18n } from "../i18n";
import type { HotMarketGroup, ModuleKey, WatchlistItem } from "../types";

const props = defineProps<{
    activeModule: ModuleKey;
    items: WatchlistItem[];
    selectedItemId: string;
    hotMarketGroup: HotMarketGroup;
}>();

const emit = defineEmits<{
    (event: "switch-module", value: ModuleKey): void;
    (event: "select-item", value: string): void;
    (event: "update:hotMarketGroup", value: HotMarketGroup): void;
    (event: "open-settings"): void;
    (event: "start-resize", e: MouseEvent): void;
}>();

const { t } = useI18n();
const moduleTabs = computed(() => getModuleTabs());
const hotMarketOptions = computed(() => getHotMarketOptions());

// Items sorted for the watchlist sidebar: position items first, then watch-only items.
// Within each group the original order is preserved.
const sidebarItems = computed(() => {
    const withPos = props.items.filter((item) => item.position?.hasPosition);
    const watchOnly = props.items.filter((item) => !item.position?.hasPosition);
    return [...withPos, ...watchOnly];
});

function itemHasPosition(item: WatchlistItem): boolean {
    return item.position?.hasPosition ?? false;
}

function switchModule(next: ModuleKey): void {
    emit("switch-module", next);
}
</script>

<template>
    <aside class="app-sidebar">
        <nav class="sidebar-primary-nav">
            <button v-for="tab in moduleTabs" :key="tab.key" class="sidebar-primary-item" :class="{ active: activeModule === tab.key }" type="button" @click="switchModule(tab.key)">
                <i :class="tab.icon"></i>
                <span>{{ tab.label }}</span>
            </button>
        </nav>

        <div class="sidebar-secondary-shell">
            <section v-if="activeModule === 'watchlist'" class="sidebar-secondary-group">
                <button v-for="item in sidebarItems" :key="item.id" class="sidebar-secondary-item" :class="{ active: selectedItemId === item.id }" type="button" @click="$emit('select-item', item.id)">
                    <div class="sidebar-item-name-row">
                        <strong :title="item.name || item.symbol">{{ item.name || item.symbol }}</strong>
                        <i v-if="itemHasPosition(item)" class="pi pi-wallet sidebar-position-icon" :title="t('sidebar.positionBadgeTitle')"></i>
                    </div>
                    <span>{{ item.market }} · {{ item.symbol }}</span>
                </button>
            </section>

            <section v-else-if="activeModule === 'hot'" class="sidebar-secondary-group">
                <button
                    v-for="entry in hotMarketOptions"
                    :key="entry.value"
                    class="sidebar-secondary-item sidebar-secondary-item-compact"
                    :class="{ active: hotMarketGroup === entry.value }"
                    type="button"
                    @click="$emit('update:hotMarketGroup', entry.value)"
                >
                    <strong>{{ entry.label }}</strong>
                </button>
            </section>
        </div>

        <div class="sidebar-footer">
            <Button size="small" text icon="pi pi-cog" :label="t('settings.title')" class="sidebar-settings-button" :class="{ active: activeModule === 'settings' }" @click="$emit('open-settings')" />
        </div>

        <div class="sidebar-resize-handle" @mousedown.prevent.stop="$emit('start-resize', $event)"></div>
    </aside>
</template>

<style scoped>
.app-sidebar {
    flex: 1 1 auto;
    min-height: 0;
    overflow: hidden;
    padding: 8px 12px 12px;
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    gap: 14px;
    position: relative;
}

.sidebar-primary-nav,
.sidebar-secondary-group {
    display: grid;
    gap: 6px;
}

.sidebar-primary-item,
.sidebar-secondary-item {
    width: 100%;
    border: 1px solid transparent;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
    text-align: left;
    transition:
        background 140ms ease,
        border-color 140ms ease,
        color 140ms ease,
        transform 140ms ease;
}

.sidebar-primary-item {
    min-height: 38px;
    padding: 0 12px;
    border-radius: calc(var(--radius-control) - 2px);
    font: 500 12px/1 var(--font-ui);
    display: inline-flex;
    align-items: center;
    gap: 9px;
}

.sidebar-primary-item:hover,
.sidebar-secondary-item:hover {
    background: color-mix(in srgb, var(--accent-soft) 54%, var(--panel-soft));
    color: var(--ink);
}

.sidebar-primary-item.active,
.sidebar-secondary-item.active {
    border-color: color-mix(in srgb, var(--accent) 18%, var(--border));
    background: linear-gradient(180deg, color-mix(in srgb, var(--accent-soft) 90%, var(--panel-strong)) 0%, color-mix(in srgb, var(--accent-soft) 42%, var(--panel-strong)) 100%);
    color: var(--accent-strong);
    box-shadow: var(--shadow-soft);
}

.sidebar-secondary-shell {
    min-height: 0;
    overflow: auto;
    padding: 0 2px 2px;
}

.sidebar-secondary-group {
    padding-top: 12px;
    border-top: 1px solid var(--border);
}

.sidebar-secondary-item {
    display: grid;
    gap: 2px;
    padding: 6px 10px;
    border-radius: 10px;
}

.sidebar-secondary-item strong {
    font-size: 12px;
    line-height: 1.25;
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: block;
}

.sidebar-item-name-row strong {
    display: block;
    font-size: 12px;
    line-height: 1.25;
    font-weight: 500;
}

.sidebar-item-name-row {
    display: flex;
    align-items: center;
    gap: 5px;
    overflow: hidden;
}

.sidebar-item-name-row strong {
    flex: 1 1 0;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

.sidebar-position-icon {
    flex: 0 0 auto;
    font-size: 10px;
    color: color-mix(in srgb, var(--accent) 35%, transparent);
    transition: color 140ms ease;
}

.sidebar-secondary-item:hover .sidebar-position-icon {
    color: color-mix(in srgb, var(--accent) 60%, transparent);
}

.sidebar-secondary-item.active .sidebar-position-icon {
    color: var(--accent);
}

.sidebar-secondary-item span {
    font-size: 10px;
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: block;
}

.sidebar-secondary-item-compact {
    gap: 0;
    min-height: 38px;
    align-items: center;
}

.sidebar-footer {
    padding-top: 10px;
    border-top: 1px solid var(--border);
    display: grid;
    gap: 4px;
}

.sidebar-settings-button {
    justify-content: flex-start;
    width: 100%;
}

.sidebar-settings-button.active {
    background: var(--button-ghost-active);
    color: var(--ink);
}

.sidebar-resize-handle {
    position: absolute;
    top: 0;
    right: -6px;
    width: 12px;
    height: 100%;
    cursor: col-resize;
    z-index: 3;
}

.sidebar-resize-handle::after {
    content: "";
    position: absolute;
    top: 16px;
    bottom: 16px;
    left: 5px;
    width: 2px;
    border-radius: 999px;
    background: color-mix(in srgb, var(--border-strong) 88%, transparent);
    opacity: 0;
    transition:
        opacity 120ms ease,
        background 120ms ease;
}

.sidebar-resize-handle:hover::after {
    opacity: 1;
    background: color-mix(in srgb, var(--accent) 42%, var(--border-strong));
}

@media (max-width: 880px) {
    .sidebar-primary-nav,
    .sidebar-secondary-group {
        gap: 4px;
    }
}
</style>
