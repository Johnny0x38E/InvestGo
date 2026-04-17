<script setup lang="ts">
import AlertsModule from "./modules/AlertsModule.vue";
import HotModule from "./modules/HotModule.vue";
import HoldingsModule from "./modules/HoldingsModule.vue";
import OverviewModule from "./modules/OverviewModule.vue";
import SettingsModule from "./modules/SettingsModule.vue";
import WatchlistModule from "./modules/WatchlistModule.vue";
import type {
    AlertRule,
    AppSettings,
    DeveloperLogEntry,
    HistoryInterval,
    HistorySeries,
    HotItem,
    HotMarketGroup,
    ModuleKey,
    QuoteSourceOption,
    SettingsTabKey,
    StateSnapshot,
    WatchlistItem,
} from "../types";

defineProps<{
    activeModule: ModuleKey;
    dashboard: StateSnapshot["dashboard"] | null;
    itemCount: number;
    livePriceCount: number;
    runtime: StateSnapshot["runtime"];
    generatedAt: string;
    selectedItem: WatchlistItem | null;
    historyInterval: HistoryInterval;
    historySeries: HistorySeries | null;
    historyLoading: boolean;
    historyError: string;
    trackedHotKeys: string[];
    hotMarketGroup: HotMarketGroup;
    search: string;
    filteredItems: WatchlistItem[];
    selectedItemId: string;
    alerts: AlertRule[];
    items: WatchlistItem[];
    settingsTab: SettingsTabKey;
    settingsDraft: AppSettings;
    quoteSources: QuoteSourceOption[];
    storagePath: string;
    logFilePath: string;
    developerLogs: DeveloperLogEntry[];
    savingSettings: boolean;
    loadingLogs: boolean;
}>();

defineEmits<{
    (event: "refresh"): void;
    (event: "select-interval", value: HistoryInterval): void;
    (event: "update:hotMarketGroup", value: HotMarketGroup): void;
    (event: "hot-watch-item", item: HotItem): void;
    (event: "hot-unwatch-item", item: HotItem): void;
    (event: "hot-open-position", item: HotItem): void;
    (event: "update:search", value: string): void;
    (event: "add-item"): void;
    (event: "edit-item", item: WatchlistItem): void;
    (event: "delete-item", value: string): void;
    (event: "toggle-pin", item: WatchlistItem): void;
    (event: "select-item", value: string): void;
    (event: "show-dca", item: WatchlistItem): void;
    (event: "add-alert"): void;
    (event: "edit-alert", value: AlertRule): void;
    (event: "delete-alert", value: string): void;
    (event: "update:settingsTab", value: SettingsTabKey): void;
    (event: "save-settings"): void;
    (event: "cancel-settings"): void;
    (event: "refresh-logs"): void;
    (event: "copy-logs"): void;
    (event: "clear-logs"): void;
}>();
</script>

<template>
    <OverviewModule
        v-if="activeModule === 'overview'"
        :dashboard="dashboard"
        :item-count="itemCount"
        :live-price-count="livePriceCount"
        :generated-at="generatedAt"
    />

    <WatchlistModule
        v-else-if="activeModule === 'watchlist'"
        :selected-item="selectedItem"
        :history-interval="historyInterval"
        :history-series="historySeries"
        :history-loading="historyLoading"
        :history-error="historyError"
        @refresh="$emit('refresh')"
        @select-interval="$emit('select-interval', $event)"
        @delete-item="$emit('delete-item', $event)"
    />

    <HotModule
        v-else-if="activeModule === 'hot'"
        :tracked-keys="trackedHotKeys"
        :market-group="hotMarketGroup"
        @update:market-group="$emit('update:hotMarketGroup', $event)"
        @watch-item="$emit('hot-watch-item', $event)"
        @unwatch-item="$emit('hot-unwatch-item', $event)"
        @open-position="$emit('hot-open-position', $event)"
    />

    <HoldingsModule
        v-else-if="activeModule === 'holdings'"
        :search="search"
        :filtered-items="filteredItems"
        :selected-item-id="selectedItemId"
        @update:search="$emit('update:search', $event)"
        @add-item="$emit('add-item')"
        @edit-item="$emit('edit-item', $event)"
        @delete-item="$emit('delete-item', $event)"
        @toggle-pin="$emit('toggle-pin', $event)"
        @select-item="$emit('select-item', $event)"
        @show-dca="$emit('show-dca', $event)"
    />

    <AlertsModule
        v-else-if="activeModule === 'alerts'"
        :alerts="alerts"
        :items="items"
        @add-alert="$emit('add-alert')"
        @edit-alert="$emit('edit-alert', $event)"
        @delete-alert="$emit('delete-alert', $event)"
    />

    <SettingsModule
        v-else-if="activeModule === 'settings'"
        :settings-tab="settingsTab"
        :settings-draft="settingsDraft"
        :quote-sources="quoteSources"
        :runtime="runtime"
        :item-count="itemCount"
        :storage-path="storagePath"
        :log-file-path="logFilePath"
        :developer-logs="developerLogs"
        :saving="savingSettings"
        :loading-logs="loadingLogs"
        @update:settings-tab="$emit('update:settingsTab', $event)"
        @save="$emit('save-settings')"
        @cancel="$emit('cancel-settings')"
        @refresh-logs="$emit('refresh-logs')"
        @copy-logs="$emit('copy-logs')"
        @clear-logs="$emit('clear-logs')"
    />
</template>
