<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";

import { api } from "./api";
import AppShell from "./components/AppShell.vue";
import AppWorkspace from "./components/AppWorkspace.vue";
import AlertDialog from "./components/dialogs/AlertDialog.vue";
import ConfirmDialog from "./components/dialogs/ConfirmDialog.vue";
import DCADetailDialog from "./components/dialogs/DCADetailDialog.vue";
import ItemDialog from "./components/dialogs/ItemDialog.vue";
import { appendClientLog, installClientLogCapture } from "./devlog";
import { useDeveloperLogs } from "./composables/useDeveloperLogs";
import { useHistorySeries } from "./composables/useHistorySeries";
import { useItemDialog } from "./composables/useItemDialog";
import { useAlertDialog } from "./composables/useAlertDialog";
import { useConfirmDialog } from "./composables/useConfirmDialog";
import { defaultSettings, normaliseSettings } from "./forms";
import { setFormatterSettings } from "./format";
import { setI18nLocale, translate } from "./i18n";
import { applyPrimeVueColorTheme } from "./theme";
import type {
    AlertRule,
    AppSettings,
    HotItem,
    HotMarketGroup,
    ModuleKey,
    OptionItem,
    QuoteSourceOption,
    SettingsTabKey,
    StateSnapshot,
    StatusTone,
    WatchlistItem,
} from "./types";

const dashboard = ref<StateSnapshot["dashboard"] | null>(null);
const items = ref<WatchlistItem[]>([]);
const alerts = ref<AlertRule[]>([]);
const settings = ref<AppSettings>({ ...defaultSettings });
const runtime = ref<StateSnapshot["runtime"]>({
    quoteSource: "",
    livePriceCount: 0,
    appVersion: "dev",
});
const quoteSources = ref<QuoteSourceOption[]>([]);
const storagePath = ref("");
const generatedAt = ref("");
const statusText = ref(translate("app.loading"));
const statusTone = ref<StatusTone>("success");
const search = ref("");
const selectedItemId = ref("");
const activeModule = ref<ModuleKey>("overview");
const hotMarketGroup = ref<HotMarketGroup>("cn");
const settingsTab = ref<SettingsTabKey>("general");
const savingSettings = ref(false);
const dcaDetailVisible = ref(false);
const dcaDetailItem = ref<WatchlistItem | null>(null);
const matchMediaList = window.matchMedia("(prefers-color-scheme: dark)");

const settingsDraft = reactive<AppSettings>({ ...defaultSettings });
let developerLogTimer = 0;
let autoRefreshTimer = 0;
let autoRefreshInFlight = false;
const hotAutoRefreshToken = ref(0);

const filteredItems = computed(() => {
    const keyword = search.value.trim().toLowerCase();
    if (!keyword) {
        return items.value;
    }

    return items.value.filter((item) =>
        [item.symbol, item.name, item.market, item.thesis, ...(item.tags ?? [])]
            .filter(Boolean)
            .join(" ")
            .toLowerCase()
            .includes(keyword),
    );
});

const selectedItem = computed(() => items.value.find((item) => item.id === selectedItemId.value) ?? null);

const alertItemOptions = computed<OptionItem<string>[]>(() =>
    items.value.map((item) => ({
        label: `${item.name || item.symbol} · ${item.symbol}`,
        value: item.id,
    })),
);

const trackedHotKeys = computed(() => items.value.map((item) => `${item.market}:${item.symbol}`));

watch(
    settings,
    (value) => {
        // Persisted settings remain the source of truth for active formatting and other business-facing behavior so drafts do not affect displayed data.
        setFormatterSettings(value);
        setI18nLocale(value.locale);
        document.documentElement.lang = value.locale === "system" ? navigator.language || "zh-CN" : value.locale;
    },
    { deep: true, immediate: true },
);

watch(
    () =>
        [
            activeModule.value,
            settings.value.fontPreset,
            settings.value.colorTheme,
            settings.value.priceColorScheme,
            settings.value.themeMode,
            settingsDraft.fontPreset,
            settingsDraft.colorTheme,
            settingsDraft.priceColorScheme,
            settingsDraft.themeMode,
        ] as const,
    () => {
        // While the settings dialog is open, allow the current view to preview appearance drafts and automatically revert to saved values when it closes.
        const appearance = activeModule.value === "settings" ? settingsDraft : settings.value;
        document.documentElement.dataset.fontPreset = appearance.fontPreset;
        document.documentElement.dataset.colorTheme = appearance.colorTheme;
        document.documentElement.dataset.priceColorScheme = appearance.priceColorScheme;
        document.documentElement.dataset.themeMode = appearance.themeMode;
        applyPrimeVueColorTheme(appearance.colorTheme);
        applyResolvedTheme(appearance.themeMode);
    },
    { immediate: true },
);

watch(activeModule, (module) => {
    if (module === "settings") {
        // Seed the draft only when entering settings so unsaved edits remain
        // isolated from the persisted application state.
        Object.assign(settingsDraft, settings.value);
    }
});

watch(activeModule, (module, previous) => {
    if (module === previous) {
        return;
    }
    if (module === "watchlist") {
        // Entering the watchlist refreshes only the selected instrument
        // so upstream providers are not hit with the whole list.
        void refreshSelectedItem(true, false);
    } else if (module === "overview") {
        // Overview aggregates the whole portfolio; refresh on module entry.
        void refreshQuotes(true, false);
    }
});

const {
    historyInterval,
    historySeries,
    historyLoading,
    historyError,
    loadHistory,
    clearHistoryCache,
    selectHistoryInterval,
} = useHistorySeries(items, selectedItem, activeModule, setStatus);

const { developerLogs, loadingLogs, logFilePath, loadBackendLogs, clearDeveloperLogs, copyDeveloperLogs } =
    useDeveloperLogs(setStatus);

watch(
    () => [activeModule.value === "settings", settingsTab.value, settingsDraft.developerMode] as const,
    ([visible, tab, developerMode]) => {
        window.clearInterval(developerLogTimer);
        if (!visible || tab !== "developer" || !developerMode) {
            return;
        }

        // Poll logs only while the developer tab is visible to avoid unnecessary background requests.
        void loadBackendLogs(true);
        developerLogTimer = window.setInterval(() => {
            void loadBackendLogs(true);
        }, 4000);
    },
    { immediate: true },
);

watch(
    () => settings.value.hotCacheTTLSeconds,
    () => {
        scheduleAutoRefresh();
    },
);

onMounted(async () => {
    installClientLogCapture();
    matchMediaList.addEventListener("change", syncThemeMode);
    await loadState();
    scheduleAutoRefresh();
});

onBeforeUnmount(() => {
    window.clearInterval(developerLogTimer);
    window.clearInterval(autoRefreshTimer);
    matchMediaList.removeEventListener("change", syncThemeMode);
});

// Sync the system theme to the document root so the desktop shell continues to follow light and dark mode changes.
function syncThemeMode(): void {
    applyResolvedTheme(settings.value.themeMode);
}

function resolvedTheme(themeMode: AppSettings["themeMode"]): "light" | "dark" {
    if (themeMode === "light" || themeMode === "dark") {
        return themeMode;
    }
    return matchMediaList.matches ? "dark" : "light";
}

function applyResolvedTheme(themeMode: AppSettings["themeMode"]): void {
    const nextTheme = resolvedTheme(themeMode);
    document.documentElement.dataset.theme = nextTheme;
    document.documentElement.classList.toggle("app-dark", nextTheme === "dark");
}

function autoRefreshIntervalMs(): number {
    return Math.max(10, settings.value.hotCacheTTLSeconds || defaultSettings.hotCacheTTLSeconds) * 1000;
}

function scheduleAutoRefresh(): void {
    window.clearInterval(autoRefreshTimer);
    autoRefreshTimer = window.setInterval(() => {
        void runAutoRefresh();
    }, autoRefreshIntervalMs());
}

async function runAutoRefresh(): Promise<void> {
    if (autoRefreshInFlight) {
        return;
    }

    autoRefreshInFlight = true;
    try {
        switch (activeModule.value) {
            case "watchlist":
                // Auto-refresh keeps the selected quote live, but history refreshes
                // must stay cache-aware so periodic ticks do not bypass provider limits.
                await refreshSelectedItem(true, true, true, false);
                break;
            case "hot":
                await refreshQuotes(true, false, true);
                hotAutoRefreshToken.value += 1;
                break;
            default:
                await refreshQuotes(true, false, true);
                break;
        }
    } finally {
        autoRefreshInFlight = false;
    }
}

// Fetch the full backend snapshot for initial load and manual refresh flows.
async function loadState(silent = false): Promise<void> {
    if (!silent) {
        setStatus(translate("app.loadingDashboard"), "success");
    }

    try {
        const snapshot = await api<StateSnapshot>("/api/state");
        applySnapshot(snapshot);
        setStatus(translate("app.dashboardLoaded"), "success");
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.loadFailed"), "error");
    }
}

// Hydrate frontend state from the backend snapshot and reset the current selection when needed.
function applySnapshot(snapshot: StateSnapshot): void {
    dashboard.value = snapshot.dashboard;
    items.value = snapshot.items ?? [];
    alerts.value = snapshot.alerts ?? [];
    settings.value = normaliseSettings(snapshot.settings);
    setI18nLocale(settings.value.locale);
    runtime.value = snapshot.runtime;
    quoteSources.value = snapshot.quoteSources ?? [];
    storagePath.value = snapshot.storagePath;
    generatedAt.value = snapshot.generatedAt;

    if (!items.value.some((item) => item.id === selectedItemId.value)) {
        // Preserve the current selection when possible, but repair it after a
        // snapshot update so list-driven modules never point at a deleted item.
        selectedItemId.value = items.value[0]?.id ?? "";
    }
}

// Refresh live quotes and optionally reload the currently selected chart range.
async function refreshQuotes(silent = false, refreshHistory = true, force = false): Promise<void> {
    try {
        if (!silent) {
            setStatus(translate("app.syncingQuotes"), "success");
        }
        const query = force ? "?force=1" : "";
        const snapshot = await api<StateSnapshot>(`/api/refresh${query}`, {
            method: "POST",
        });
        applySnapshot(snapshot);
        if (refreshHistory && activeModule.value === "watchlist" && selectedItem.value) {
            // Quote refresh changes the chart-side market snapshot, so refresh
            // the active series in the background to keep the side panel and
            // chart overlays aligned with the latest live quote.
            await loadHistory(true, true);
        }
        if (snapshot.runtime.lastQuoteError) {
            setStatus(snapshot.runtime.lastQuoteError, "error");
        } else if (snapshot.runtime.lastFxError) {
            setStatus(
                translate("app.quotesSyncedFxFailed", {
                    error: snapshot.runtime.lastFxError,
                }),
                "warn",
            );
        } else if (!silent) {
            setStatus(translate("app.quotesSynced"), "success");
        }
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.refreshFailed"), "error");
    }
}

// Refresh only the selected watchlist item so the market view follows the active instrument instead of batching the whole watchlist.
async function refreshSelectedItem(
    silent = false,
    refreshHistory = true,
    force = false,
    forceHistory = force,
): Promise<void> {
    const currentItem = selectedItem.value;
    if (!currentItem) {
        return;
    }

    try {
        if (!silent) {
            setStatus(translate("app.syncingQuotes"), "success");
        }
        const query = force ? "?force=1" : "";
        const snapshot = await api<StateSnapshot>(`/api/items/${encodeURIComponent(currentItem.id)}/refresh${query}`, {
            method: "POST",
        });
        applySnapshot(snapshot);
        if (refreshHistory && activeModule.value === "watchlist" && selectedItem.value?.id === currentItem.id) {
            await loadHistory(true, forceHistory);
        }
        if (snapshot.runtime.lastQuoteError) {
            setStatus(snapshot.runtime.lastQuoteError, "error");
        } else if (snapshot.runtime.lastFxError) {
            setStatus(
                translate("app.quotesSyncedFxFailed", {
                    error: snapshot.runtime.lastFxError,
                }),
                "warn",
            );
        } else if (!silent) {
            setStatus(translate("app.quotesSynced"), "success");
        }
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.refreshFailed"), "error");
    }
}

// Update the top status bar message and tone.
function setStatus(message: string, tone: StatusTone): void {
    statusText.value = message;
    statusTone.value = tone;
}

// Open the settings dialog; the activeModule watcher seeds settingsDraft.
function openSettings(): void {
    activeModule.value = "settings";
}

// Persist user settings and let the backend return a refreshed full snapshot.
async function saveSettings(): Promise<void> {
    savingSettings.value = true;
    try {
        const snapshot = await api<StateSnapshot>("/api/settings", {
            method: "PUT",
            body: JSON.stringify(settingsDraft),
        });
        applySnapshot(snapshot);
        setStatus(translate("app.settingsSaved"), "success");
        // After saving settings, refresh the chart if the watchlist module is active so the new settings take effect immediately.
        if (activeModule.value === "watchlist" && selectedItem.value) {
            void loadHistory(true, true);
        }
        activeModule.value = "overview";
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.settingsSaveFailed"), "error");
    } finally {
        savingSettings.value = false;
    }
}

const {
    itemDialogVisible,
    itemDialogInitialTab,
    itemDialogWatchOnly,
    savingItem,
    itemForm,
    openItemDialog,
    openHotWatchDialog,
    openHotPositionDialog,
    saveItem,
    quickAddHotItem: quickAddHotItemInner,
    toggleItemPinned,
    performDeleteItem: performDeleteItemInner,
} = useItemDialog(applySnapshot, clearHistoryCache, setStatus);

async function quickAddHotItem(item: HotItem): Promise<void> {
    const key = `${item.market}:${item.symbol}`;
    await quickAddHotItemInner(item, trackedHotKeys.value.includes(key));
}

const {
    alertDialogVisible,
    savingAlert,
    alertForm,
    openAlertDialog,
    saveAlert,
    performDeleteAlert: performDeleteAlertInner,
} = useAlertDialog(applySnapshot, setStatus, () => {
    activeModule.value = "alerts";
});

const {
    confirmDialogVisible,
    confirmTitle,
    confirmMessage,
    confirmLabel,
    deleting,
    requestDeleteItem,
    requestDeleteAlert,
    confirmDelete,
} = useConfirmDialog(performDeleteItemInner, performDeleteAlertInner);

function unwatchHotItem(item: HotItem): void {
    const existing = items.value.find((i) => i.symbol === item.symbol && i.market === item.market);
    if (existing) {
        requestDeleteItem(existing.id);
    }
}

// Open the DCA detail dialog.
function showDCADetail(item: WatchlistItem): void {
    dcaDetailItem.value = item;
    dcaDetailVisible.value = true;
}

// Jump from the DCA detail dialog back into the item editor with the DCA tab selected.
function editFromDCADetail(): void {
    if (!dcaDetailItem.value) return;
    dcaDetailVisible.value = false;
    openItemDialog(dcaDetailItem.value, "dca");
}

// Switch the active module; watchlist data loading is handled by the module watcher so it can choose single-item refreshes.
function switchModule(next: ModuleKey): void {
    appendClientLog("info", "tabs", `switch module ${activeModule.value} -> ${next}`);
    activeModule.value = next;
}
</script>
<template>
    <AppShell
        :active-module="activeModule"
        :items="items"
        :selected-item-id="selectedItemId"
        :hot-market-group="hotMarketGroup"
        :status-text="statusText"
        :status-tone="statusTone"
        :generated-at="generatedAt"
        @switch-module="switchModule"
        @select-item="selectedItemId = $event"
        @update:hot-market-group="hotMarketGroup = $event"
        @open-settings="openSettings"
    >
        <AppWorkspace
            :active-module="activeModule"
            :dashboard="dashboard"
            :item-count="items.length"
            :live-price-count="runtime.livePriceCount"
            :runtime="runtime"
            :generated-at="generatedAt"
            :selected-item="selectedItem"
            :history-interval="historyInterval"
            :history-series="historySeries"
            :history-loading="historyLoading"
            :history-error="historyError"
            :tracked-hot-keys="trackedHotKeys"
            :hot-market-group="hotMarketGroup"
            :hot-auto-refresh-token="hotAutoRefreshToken"
            :search="search"
            :filtered-items="filteredItems"
            :selected-item-id="selectedItemId"
            :alerts="alerts"
            :items="items"
            :settings-tab="settingsTab"
            :settings-draft="settingsDraft"
            :quote-sources="quoteSources"
            :storage-path="storagePath"
            :log-file-path="logFilePath"
            :developer-logs="developerLogs"
            :saving-settings="savingSettings"
            :loading-logs="loadingLogs"
            @refresh="
                activeModule === 'watchlist' ? refreshSelectedItem(false, true, true) : refreshQuotes(false, true, true)
            "
            @select-interval="selectHistoryInterval"
            @update:hot-market-group="hotMarketGroup = $event"
            @hot-watch-item="openHotWatchDialog"
            @hot-unwatch-item="unwatchHotItem"
            @hot-open-position="openHotPositionDialog"
            @update:search="search = $event"
            @add-item="openItemDialog()"
            @edit-item="openItemDialog"
            @delete-item="requestDeleteItem"
            @toggle-pin="toggleItemPinned"
            @select-item="selectedItemId = $event"
            @show-dca="showDCADetail"
            @add-alert="openAlertDialog(undefined, items[0]?.id)"
            @edit-alert="openAlertDialog"
            @delete-alert="requestDeleteAlert"
            @update:settings-tab="settingsTab = $event"
            @save-settings="saveSettings"
            @cancel-settings="activeModule = 'overview'"
            @refresh-logs="loadBackendLogs()"
            @copy-logs="copyDeveloperLogs"
            @clear-logs="clearDeveloperLogs"
        />

        <ItemDialog
            v-if="itemDialogVisible"
            :visible="itemDialogVisible"
            :form="itemForm"
            :saving="savingItem"
            :initial-tab="itemDialogInitialTab"
            :watch-only="itemDialogWatchOnly"
            @update:visible="itemDialogVisible = $event"
            @save="saveItem"
        />

        <DCADetailDialog
            v-if="dcaDetailVisible"
            :visible="dcaDetailVisible"
            :item="dcaDetailItem"
            @update:visible="dcaDetailVisible = $event"
            @edit="editFromDCADetail"
        />

        <AlertDialog
            v-if="alertDialogVisible"
            :visible="alertDialogVisible"
            :form="alertForm"
            :item-options="alertItemOptions"
            :saving="savingAlert"
            @update:visible="alertDialogVisible = $event"
            @save="saveAlert"
        />

        <ConfirmDialog
            v-if="confirmDialogVisible"
            :visible="confirmDialogVisible"
            :title="confirmTitle"
            :message="confirmMessage"
            :confirm-label="confirmLabel"
            :loading="deleting"
            @update:visible="confirmDialogVisible = $event"
            @confirm="confirmDelete"
        />
    </AppShell>
</template>
