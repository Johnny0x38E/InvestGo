<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";

import { getHotMarketOptions, getModuleTabs } from "../constants";
import { useI18n } from "../i18n";
import type { HotMarketGroup, ModuleKey, WatchlistItem } from "../types";

defineProps<{
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
    (event: "toggle-visibility"): void;
    (event: "start-resize", e: MouseEvent): void;
}>();

const { t } = useI18n();
const moduleTabs = computed(() => getModuleTabs());
const hotMarketOptions = computed(() => getHotMarketOptions());

function switchModule(next: ModuleKey): void {
    emit("switch-module", next);
}
</script>

<template>
    <aside class="app-sidebar">
        <nav class="sidebar-primary-nav">
            <button
                v-for="tab in moduleTabs"
                :key="tab.key"
                class="sidebar-primary-item"
                :class="{ active: activeModule === tab.key }"
                type="button"
                @click="switchModule(tab.key)"
            >
                <i :class="tab.icon"></i>
                <span>{{ tab.label }}</span>
            </button>
        </nav>

        <div class="sidebar-secondary-shell">
            <section
                v-if="activeModule === 'market'"
                class="sidebar-secondary-group"
            >
                <button
                    v-for="item in items"
                    :key="item.id"
                    class="sidebar-secondary-item"
                    :class="{ active: selectedItemId === item.id }"
                    type="button"
                    @click="$emit('select-item', item.id)"
                >
                    <strong :title="item.name || item.symbol">{{
                        item.name || item.symbol
                    }}</strong>
                    <span>{{ item.market }} · {{ item.symbol }}</span>
                </button>
            </section>

            <section
                v-else-if="activeModule === 'hot'"
                class="sidebar-secondary-group"
            >
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
            <Button
                size="small"
                text
                icon="pi pi-cog"
                :label="t('settings.title')"
                class="sidebar-settings-button"
                :class="{ active: activeModule === 'settings' }"
                @click="$emit('open-settings')"
            />
        </div>

        <div
            class="sidebar-resize-handle"
            @mousedown.prevent.stop="$emit('start-resize', $event)"
        ></div>
    </aside>
</template>
