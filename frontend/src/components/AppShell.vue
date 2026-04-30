<script setup lang="ts">
import AppHeader from "./AppHeader.vue";
import AppSidebar from "./AppSidebar.vue";
import { useSidebarLayout } from "../composables/useSidebarLayout";
import { useI18n } from "../i18n";
import { shouldReserveMacWindowControls, shouldShowCustomWindowControls } from "../wails-runtime";
import type { HotMarketGroup, ModuleKey, StatusTone, WatchlistItem } from "../types";

const props = defineProps<{
    activeModule: ModuleKey;
    items: WatchlistItem[];
    selectedItemId: string;
    hotMarketGroup: HotMarketGroup;
    statusText: string;
    statusTone: StatusTone;
    generatedAt: string;
    useNativeTitleBar: boolean;
}>();

const emit = defineEmits<{
    (event: "switch-module", value: ModuleKey): void;
    (event: "select-item", value: string): void;
    (event: "update:hotMarketGroup", value: HotMarketGroup): void;
    (event: "open-settings"): void;
}>();

const { t } = useI18n();
const { appShellRef, sidebarWidth, sidebarHidden, toggleSidebar, startSidebarResize } = useSidebarLayout();
</script>

<template>
    <div
        ref="appShellRef"
        class="app-shell"
        :class="{
            'is-sidebar-hidden': sidebarHidden,
            'is-mac-custom-titlebar': shouldReserveMacWindowControls(props.useNativeTitleBar),
            'is-nonmac-custom-titlebar': shouldShowCustomWindowControls(props.useNativeTitleBar),
        }"
        :style="{ '--sidebar-width': `${sidebarWidth}px` }"
    >
        <div v-if="!sidebarHidden" class="sidebar-column">
            <div class="sidebar-topbar">
                <button type="button" :aria-label="t('sidebar.hide')" class="sidebar-chrome-toggle" @click="toggleSidebar">
                    <svg width="16" height="14" viewBox="0 0 16 14" fill="none" aria-hidden="true">
                        <rect x="0.65" y="0.65" width="14.7" height="12.7" rx="2.2" stroke="currentColor" stroke-width="1.3" />
                        <path d="M5.25 0.65V13.35" stroke="currentColor" stroke-width="1.3" />
                    </svg>
                </button>
            </div>
            <AppSidebar
                :active-module="activeModule"
                :items="items"
                :selected-item-id="selectedItemId"
                :hot-market-group="hotMarketGroup"
                @switch-module="emit('switch-module', $event)"
                @select-item="emit('select-item', $event)"
                @update:hot-market-group="emit('update:hotMarketGroup', $event)"
                @open-settings="emit('open-settings')"
                @start-resize="startSidebarResize"
            />
        </div>

        <div class="main-column">
            <div class="main-topbar">
                <button v-if="sidebarHidden" type="button" :aria-label="t('sidebar.show')" class="sidebar-chrome-toggle" @click="toggleSidebar">
                    <svg width="16" height="14" viewBox="0 0 16 14" fill="none" aria-hidden="true">
                        <rect x="0.65" y="0.65" width="14.7" height="12.7" rx="2.2" stroke="currentColor" stroke-width="1.3" />
                        <path d="M5.25 0.65V13.35" stroke="currentColor" stroke-width="1.3" />
                    </svg>
                </button>
                <AppHeader :status-text="statusText" :status-tone="statusTone" :generated-at="generatedAt" :show-window-controls="shouldShowCustomWindowControls(props.useNativeTitleBar)" />
            </div>

            <div class="workspace-panel">
                <div class="workspace-stage">
                    <slot></slot>
                </div>
            </div>
        </div>
    </div>
</template>

<style scoped>
.app-shell {
    --window-control-inset-left: 0px;
    --sidebar-shell-radius: 18px;
    height: 100%;
    display: grid;
    grid-template-columns: var(--sidebar-width, 220px) minmax(0, 1fr);
    gap: 10px;
    padding: 10px;
}

.app-shell.is-mac-custom-titlebar {
    --window-control-inset-left: 76px;
}

.app-shell.is-nonmac-custom-titlebar {
    --sidebar-shell-radius: 10px;
}

.app-shell.is-sidebar-hidden {
    grid-template-columns: minmax(0, 1fr);
}

.sidebar-column {
    min-height: 0;
    display: flex;
    flex-direction: column;
    border: 1px solid var(--border);
    background:
        radial-gradient(circle at top left, color-mix(in srgb, var(--accent-soft) 90%, transparent) 0%, transparent 42%),
        linear-gradient(180deg, color-mix(in srgb, var(--panel-soft) 68%, var(--panel-bg)) 0%, var(--panel-bg) 100%);
    box-shadow:
        0 18px 40px rgba(15, 23, 42, 0.1),
        0 6px 18px rgba(15, 23, 42, 0.06),
        0 1px 0 rgba(255, 255, 255, 0.18) inset;
    backdrop-filter: blur(14px);
    border-radius: var(--sidebar-shell-radius);
    overflow: hidden;
}

.sidebar-topbar {
    min-height: 52px;
    padding: 0 10px 0 var(--window-control-inset-left);
    display: flex;
    align-items: flex-start;
    justify-content: flex-end;
    flex-shrink: 0;
}

.main-column {
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
}

.main-topbar {
    display: flex;
    align-items: stretch;
}

.main-topbar :deep(.window-bar) {
    flex: 1 1 auto;
}

.main-topbar .sidebar-chrome-toggle {
    margin-left: var(--window-control-inset-left);
    margin-right: 4px;
    align-self: flex-start;
    margin-top: 2px;
    flex-shrink: 0;
}

.workspace-panel {
    flex: 1 1 auto;
    min-height: 0;
    border: none;
    background: transparent;
    box-shadow: none;
    backdrop-filter: none;
    border-radius: 0;
    overflow: visible;
    padding: 0;
    display: flex;
    flex-direction: column;
}

.workspace-stage {
    flex: 1 1 auto;
    width: 100%;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    padding: 2px 0 0;
}

.sidebar-chrome-toggle {
    width: 30px;
    height: 30px;
    padding: 0;
    border-radius: 8px;
    color: var(--muted);
    background: transparent;
    border: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    font-size: 14px;
    transition: color 120ms ease;
}

.sidebar-chrome-toggle:hover {
    color: var(--ink);
}

@media (max-width: 1180px) {
    .sidebar-column {
        display: none;
    }

    .app-shell {
        grid-template-columns: minmax(0, 1fr);
    }
}

@media (max-width: 880px) {
    .app-shell {
        height: auto;
        min-height: 100%;
    }
}
</style>
