<script setup lang="ts">
import {
    computed,
    onBeforeUnmount,
    onMounted,
    reactive,
    ref,
    watch,
} from "vue";

import { api } from "./api";
import AppHeader from "./components/AppHeader.vue";
import AppSidebar from "./components/AppSidebar.vue";
import AlertDialog from "./components/dialogs/AlertDialog.vue";
import ConfirmDialog from "./components/dialogs/ConfirmDialog.vue";
import DCADetailDialog from "./components/dialogs/DCADetailDialog.vue";
import ItemDialog from "./components/dialogs/ItemDialog.vue";
import SettingsModule from "./components/modules/SettingsModule.vue";
import AlertsModule from "./components/modules/AlertsModule.vue";
import HotModule from "./components/modules/HotModule.vue";
import MarketModule from "./components/modules/MarketModule.vue";
import OverviewModule from "./components/modules/OverviewModule.vue";
import WatchlistModule from "./components/modules/WatchlistModule.vue";
import { appendClientLog, installClientLogCapture } from "./devlog";
import { useDeveloperLogs } from "./composables/useDeveloperLogs";
import { useHistorySeries } from "./composables/useHistorySeries";
import {
    defaultSettings,
    emptyAlertForm,
    emptyItemForm,
    mapAlertToForm,
    mapItemToForm,
    serialiseItemForm,
} from "./forms";
import { setFormatterSettings } from "./format";
import { setI18nLocale, translate } from "./i18n";
import { applyPrimeVueColorTheme } from "./theme";
import type {
    AlertFormModel,
    AlertRule,
    AppSettings,
    HotItem,
    HotMarketGroup,
    ItemFormModel,
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
const appShellRef = ref<HTMLElement | null>(null);
const search = ref("");
const selectedItemId = ref("");
const activeModule = ref<ModuleKey>("overview");
const hotMarketGroup = ref<HotMarketGroup>("cn");
const sidebarWidth = ref(220);
const sidebarHidden = ref(false);
const settingsTab = ref<SettingsTabKey>("general");
const settingsVisible = ref(false);
const itemDialogVisible = ref(false);
const alertDialogVisible = ref(false);
const confirmDialogVisible = ref(false);
const savingSettings = ref(false);
const savingItem = ref(false);
const dcaDetailVisible = ref(false);
const dcaDetailItem = ref<WatchlistItem | null>(null);
const itemDialogInitialTab = ref<"basic" | "dca">("basic");
const savingAlert = ref(false);
const deleting = ref(false);
const matchMediaList = window.matchMedia("(prefers-color-scheme: dark)");

const settingsDraft = reactive<AppSettings>({ ...defaultSettings });
const itemForm = reactive<ItemFormModel>(emptyItemForm());
const alertForm = reactive<AlertFormModel>(emptyAlertForm());
const confirmTitle = ref("");
const confirmMessage = ref("");
const confirmLabel = ref(translate("common.delete"));
const pendingDelete = reactive<{ kind: "" | "item" | "alert"; id: string }>({
    kind: "",
    id: "",
});
let refreshTimer = 0;
let developerLogTimer = 0;
let sidebarResizeActive = false;

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

const selectedItem = computed(
    () => items.value.find((item) => item.id === selectedItemId.value) ?? null,
);

const alertItemOptions = computed<OptionItem<string>[]>(() =>
    items.value.map((item) => ({
        label: `${item.name || item.symbol} · ${item.symbol}`,
        value: item.id,
    })),
);

const trackedHotKeys = computed(() =>
    items.value.map((item) => `${item.market}:${item.symbol}`),
);

watch(
    settings,
    (value) => {
        // Persisted settings remain the source of truth for active formatting and other business-facing behavior so drafts do not affect displayed data.
        setFormatterSettings(value);
        setI18nLocale(value.locale);
        document.documentElement.lang =
            value.locale === "system"
                ? navigator.language || "zh-CN"
                : value.locale;
    },
    { deep: true, immediate: true },
);

watch(
    () =>
        [
            settingsVisible.value,
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
        const appearance =
            activeModule.value === "settings" ? settingsDraft : settings.value;
        document.documentElement.dataset.fontPreset = appearance.fontPreset;
        document.documentElement.dataset.colorTheme = appearance.colorTheme;
        document.documentElement.dataset.priceColorScheme =
            appearance.priceColorScheme;
        document.documentElement.dataset.themeMode = appearance.themeMode;
        applyPrimeVueColorTheme(appearance.colorTheme);
        applyResolvedTheme(appearance.themeMode);
    },
    { immediate: true },
);

watch(activeModule, (module) => {
    if (module === "settings") {
        Object.assign(settingsDraft, settings.value);
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

const {
    developerLogs,
    loadingLogs,
    logFilePath,
    loadBackendLogs,
    clearDeveloperLogs,
    copyDeveloperLogs,
} = useDeveloperLogs(setStatus);

watch(
    () =>
        [
            settingsVisible.value,
            settingsTab.value,
            settingsDraft.developerMode,
        ] as const,
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

onMounted(async () => {
    installClientLogCapture();
    matchMediaList.addEventListener("change", syncThemeMode);
    await loadState();
});

onBeforeUnmount(() => {
    window.clearTimeout(refreshTimer);
    window.clearInterval(developerLogTimer);
    matchMediaList.removeEventListener("change", syncThemeMode);
    stopSidebarResize();
});

function clampSidebarWidth(value: number): number {
    return Math.min(Math.max(Math.round(value), 220), 380);
}

function toggleSidebar(): void {
    sidebarHidden.value = !sidebarHidden.value;
}

function startSidebarResize(): void {
    sidebarHidden.value = false;
    if (sidebarResizeActive) {
        return;
    }
    sidebarResizeActive = true;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    window.addEventListener("mousemove", handleSidebarResize);
    window.addEventListener("mouseup", stopSidebarResize);
}

function handleSidebarResize(event: MouseEvent): void {
    if (!sidebarResizeActive) {
        return;
    }
    const shellLeft = appShellRef.value?.getBoundingClientRect().left ?? 0;
    sidebarWidth.value = clampSidebarWidth(event.clientX - shellLeft);
}

function stopSidebarResize(): void {
    if (!sidebarResizeActive) {
        return;
    }
    sidebarResizeActive = false;
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
    window.removeEventListener("mousemove", handleSidebarResize);
    window.removeEventListener("mouseup", stopSidebarResize);
}

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

// Fetch the full backend snapshot for initial load and manual refresh flows.
async function loadState(silent = false): Promise<void> {
    if (!silent) {
        setStatus(translate("app.loadingDashboard"), "success");
    }

    try {
        const snapshot = await api<StateSnapshot>("/api/state");
        applySnapshot(snapshot);
        setStatus(translate("app.dashboardLoaded"), "success");

        void refreshQuotes(true, false);
    } catch (error) {
        setStatus(
            error instanceof Error
                ? error.message
                : translate("app.loadFailed"),
            "error",
        );
    }
}

// Hydrate frontend state from the backend snapshot and reset the current selection when needed.
function applySnapshot(snapshot: StateSnapshot): void {
    dashboard.value = snapshot.dashboard;
    items.value = snapshot.items ?? [];
    alerts.value = snapshot.alerts ?? [];
    settings.value = snapshot.settings;
    setI18nLocale(snapshot.settings.locale);
    runtime.value = snapshot.runtime;
    quoteSources.value = snapshot.quoteSources ?? [];
    storagePath.value = snapshot.storagePath;
    generatedAt.value = snapshot.generatedAt;

    if (!items.value.some((item) => item.id === selectedItemId.value)) {
        selectedItemId.value = items.value[0]?.id ?? "";
    }

    scheduleAutoRefresh();
}

// Refresh live quotes and optionally reload the currently selected chart range.
async function refreshQuotes(
    silent = false,
    refreshHistory = true,
): Promise<void> {
    try {
        if (!silent) {
            setStatus(translate("app.syncingQuotes"), "success");
        }
        const snapshot = await api<StateSnapshot>("/api/refresh", {
            method: "POST",
        });
        applySnapshot(snapshot);
        if (
            refreshHistory &&
            activeModule.value === "market" &&
            selectedItem.value
        ) {
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
        setStatus(
            error instanceof Error
                ? error.message
                : translate("app.refreshFailed"),
            "error",
        );
    } finally {
        scheduleAutoRefresh();
    }
}

// Schedule the next automatic sync using the configured interval so every chart range stays up to date.
function scheduleAutoRefresh(): void {
    window.clearTimeout(refreshTimer);
    const intervalMs =
        Math.max(settings.value.refreshIntervalSeconds || 60, 10) * 1000;

    refreshTimer = window.setTimeout(() => {
        void refreshQuotes(true);
    }, intervalMs);
}

// Update the top status bar message and tone.
function setStatus(message: string, tone: StatusTone): void {
    statusText.value = message;
    statusTone.value = tone;
}

// Open the settings dialog and copy the current settings into the draft model.
function openSettings(): void {
    Object.assign(settingsDraft, settings.value);
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
        // After saving settings, refresh the chart if the market module is active so the new settings take effect immediately.
        if (activeModule.value === "market" && selectedItem.value) {
            void loadHistory(true, true);
        }
        activeModule.value = "overview";
    } catch (error) {
        setStatus(
            error instanceof Error
                ? error.message
                : translate("app.settingsSaveFailed"),
            "error",
        );
    } finally {
        savingSettings.value = false;
    }
}

// Open the item editor dialog.
function openItemDialog(
    item?: WatchlistItem,
    initialTab: "basic" | "dca" = "basic",
): void {
    Object.assign(itemForm, item ? mapItemToForm(item) : emptyItemForm());
    itemDialogInitialTab.value = initialTab;
    itemDialogVisible.value = true;
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

// Save the item and refresh cached data so the active chart stays aligned with the latest state.
async function saveItem(): Promise<void> {
    savingItem.value = true;
    try {
        const payload = serialiseItemForm(itemForm);
        const path = itemForm.id ? `/api/items/${itemForm.id}` : "/api/items";
        const method = itemForm.id ? "PUT" : "POST";
        const snapshot = await api<StateSnapshot>(path, {
            method,
            body: JSON.stringify(payload),
        });
        clearHistoryCache();
        applySnapshot(snapshot);
        itemDialogVisible.value = false;
        setStatus(
            itemForm.id
                ? translate("app.itemUpdated")
                : translate("app.itemAdded"),
            "success",
        );
        activeModule.value = "watchlist";
    } catch (error) {
        setStatus(
            error instanceof Error
                ? error.message
                : translate("app.itemSaveFailed"),
            "error",
        );
    } finally {
        savingItem.value = false;
    }
}

// Quickly add an instrument from the hot list into the watchlist.
async function quickAddHotItem(item: HotItem): Promise<void> {
    const key = `${item.market}:${item.symbol}`;
    if (trackedHotKeys.value.includes(key)) {
        setStatus(translate("app.itemAlreadyTracked"), "warn");
        return;
    }

    try {
        // Quick add only writes the baseline holding fields; the current price is still backfilled by the unified quote source.
        const snapshot = await api<StateSnapshot>("/api/items", {
            method: "POST",
            body: JSON.stringify({
                symbol: item.symbol,
                name: item.name,
                market: item.market,
                currency: item.currency,
                quantity: 0,
                costPrice: item.currentPrice || 0,
                tags: [translate("app.quickAddTag")],
                thesis: translate("app.quickAddThesis"),
            }),
        });
        applySnapshot(snapshot);
        setStatus(
            translate("app.hotItemAdded", { symbol: item.symbol }),
            "success",
        );
    } catch (error) {
        setStatus(
            error instanceof Error
                ? error.message
                : translate("app.addItemFailed"),
            "error",
        );
    }
}

async function toggleItemPinned(item: WatchlistItem): Promise<void> {
    try {
        const snapshot = await api<StateSnapshot>(`/api/items/${item.id}/pin`, {
            method: "PUT",
            body: JSON.stringify({ pinned: !item.pinnedAt }),
        });
        applySnapshot(snapshot);
        setStatus(
            item.pinnedAt
                ? translate("app.itemUnpinned")
                : translate("app.itemPinned"),
            "success",
        );
    } catch (error) {
        setStatus(
            error instanceof Error ? error.message : translate("app.pinFailed"),
            "error",
        );
    }
}

// Clear related history cache entries when an item is deleted.
async function performDeleteItem(id: string): Promise<void> {
    try {
        const snapshot = await api<StateSnapshot>(`/api/items/${id}`, {
            method: "DELETE",
        });
        clearHistoryCache();
        applySnapshot(snapshot);
        setStatus(translate("app.itemDeleted"), "success");
    } catch (error) {
        setStatus(
            error instanceof Error
                ? error.message
                : translate("app.deleteFailed"),
            "error",
        );
    }
}

// Open the alert editor dialog.
function openAlertDialog(alert?: AlertRule): void {
    Object.assign(
        alertForm,
        alert ? mapAlertToForm(alert) : emptyAlertForm(items.value[0]?.id),
    );
    alertDialogVisible.value = true;
}

// Save the alert rule and switch the UI to the alerts module.
async function saveAlert(): Promise<void> {
    savingAlert.value = true;
    try {
        const path = alertForm.id
            ? `/api/alerts/${alertForm.id}`
            : "/api/alerts";
        const method = alertForm.id ? "PUT" : "POST";
        const snapshot = await api<StateSnapshot>(path, {
            method,
            body: JSON.stringify(alertForm),
        });
        applySnapshot(snapshot);
        alertDialogVisible.value = false;
        setStatus(
            alertForm.id
                ? translate("app.alertUpdated")
                : translate("app.alertAdded"),
            "success",
        );
        activeModule.value = "alerts";
    } catch (error) {
        setStatus(
            error instanceof Error
                ? error.message
                : translate("app.alertSaveFailed"),
            "error",
        );
    } finally {
        savingAlert.value = false;
    }
}

// Delete an alert rule.
async function performDeleteAlert(id: string): Promise<void> {
    try {
        const snapshot = await api<StateSnapshot>(`/api/alerts/${id}`, {
            method: "DELETE",
        });
        applySnapshot(snapshot);
        setStatus(translate("app.alertDeleted"), "success");
    } catch (error) {
        setStatus(
            error instanceof Error
                ? error.message
                : translate("app.deleteFailed"),
            "error",
        );
    }
}

// Record the pending delete target and open the confirmation dialog.
function requestDeleteItem(id: string): void {
    pendingDelete.kind = "item";
    pendingDelete.id = id;
    confirmTitle.value = translate("dialogs.confirm.deleteItemTitle");
    confirmMessage.value = translate("dialogs.confirm.deleteItemMessage");
    confirmLabel.value = translate("dialogs.confirm.deleteItemLabel");
    confirmDialogVisible.value = true;
}

function requestDeleteAlert(id: string): void {
    pendingDelete.kind = "alert";
    pendingDelete.id = id;
    confirmTitle.value = translate("dialogs.confirm.deleteAlertTitle");
    confirmMessage.value = translate("dialogs.confirm.deleteAlertMessage");
    confirmLabel.value = translate("dialogs.confirm.deleteAlertLabel");
    confirmDialogVisible.value = true;
}

// Execute the confirmed delete action.
async function confirmDelete(): Promise<void> {
    if (!pendingDelete.kind || !pendingDelete.id) {
        confirmDialogVisible.value = false;
        return;
    }

    deleting.value = true;
    try {
        // The delete target has already been frozen into pendingDelete before confirmation, so this branch only performs the matching action.
        if (pendingDelete.kind === "item") {
            await performDeleteItem(pendingDelete.id);
        } else {
            await performDeleteAlert(pendingDelete.id);
        }
        confirmDialogVisible.value = false;
    } finally {
        deleting.value = false;
        pendingDelete.kind = "";
        pendingDelete.id = "";
    }
}

// Switch the active module and eagerly load the current chart when entering the market module.
function switchModule(next: ModuleKey): void {
    appendClientLog(
        "info",
        "tabs",
        `switch module ${activeModule.value} -> ${next}`,
    );
    activeModule.value = next;
    if (next === "market") {
        void loadHistory(true);
    }
}
</script>

<template>
    <div
        ref="appShellRef"
        class="app-shell"
        :class="{ 'is-sidebar-hidden': sidebarHidden }"
        :style="{ '--sidebar-width': `${sidebarWidth}px` }"
    >
        <div v-if="!sidebarHidden" class="sidebar-column">
            <div class="sidebar-topbar">
                <button
                    type="button"
                    :aria-label="translate('sidebar.hide')"
                    class="sidebar-chrome-toggle"
                    @click="toggleSidebar"
                >
                    <i class="pi pi-align-left" aria-hidden="true"></i>
                </button>
            </div>
            <AppSidebar
                :active-module="activeModule"
                :items="items"
                :selected-item-id="selectedItemId"
                :hot-market-group="hotMarketGroup"
                @switch-module="switchModule"
                @select-item="selectedItemId = $event"
                @update:hot-market-group="hotMarketGroup = $event"
                @open-settings="openSettings"
                @toggle-visibility="toggleSidebar"
                @start-resize="startSidebarResize"
            />
        </div>

        <div class="main-column">
            <div class="main-topbar">
                <button
                    v-if="sidebarHidden"
                    type="button"
                    :aria-label="translate('sidebar.show')"
                    class="sidebar-chrome-toggle"
                    @click="toggleSidebar"
                >
                    <i class="pi pi-align-left" aria-hidden="true"></i>
                </button>
                <AppHeader
                    :status-text="statusText"
                    :status-tone="statusTone"
                    :generated-at="generatedAt"
                />
            </div>

            <div class="workspace-panel">
                <div class="workspace-stage">
                    <OverviewModule
                        v-if="activeModule === 'overview'"
                        :dashboard="dashboard"
                        :item-count="items.length"
                        :live-price-count="runtime.livePriceCount"
                        :runtime="runtime"
                        :generated-at="generatedAt"
                    />

                    <MarketModule
                        v-else-if="activeModule === 'market'"
                        :selected-item="selectedItem"
                        :history-interval="historyInterval"
                        :history-series="historySeries"
                        :history-loading="historyLoading"
                        :history-error="historyError"
                        @refresh="refreshQuotes()"
                        @select-interval="selectHistoryInterval"
                    />

                    <HotModule
                        v-else-if="activeModule === 'hot'"
                        :tracked-keys="trackedHotKeys"
                        :market-group="hotMarketGroup"
                        @update:market-group="hotMarketGroup = $event"
                        @add-item="quickAddHotItem"
                    />

                    <WatchlistModule
                        v-else-if="activeModule === 'watchlist'"
                        :search="search"
                        :filtered-items="filteredItems"
                        :selected-item-id="selectedItemId"
                        @update:search="search = $event"
                        @add-item="openItemDialog()"
                        @edit-item="openItemDialog"
                        @delete-item="requestDeleteItem"
                        @toggle-pin="toggleItemPinned"
                        @select-item="selectedItemId = $event"
                        @show-dca="showDCADetail"
                    />

                    <AlertsModule
                        v-else-if="activeModule === 'alerts'"
                        :alerts="alerts"
                        :items="items"
                        @add-alert="openAlertDialog()"
                        @edit-alert="openAlertDialog"
                        @delete-alert="requestDeleteAlert"
                    />

                    <SettingsModule
                        v-else-if="activeModule === 'settings'"
                        :settings-tab="settingsTab"
                        :settings-draft="settingsDraft"
                        :quote-sources="quoteSources"
                        :runtime="runtime"
                        :item-count="items.length"
                        :storage-path="storagePath"
                        :log-file-path="logFilePath"
                        :developer-logs="developerLogs"
                        :saving="savingSettings"
                        :loading-logs="loadingLogs"
                        @update:settings-tab="settingsTab = $event"
                        @save="saveSettings"
                        @cancel="activeModule = 'overview'"
                        @refresh-logs="loadBackendLogs()"
                        @copy-logs="copyDeveloperLogs"
                        @clear-logs="clearDeveloperLogs"
                    />
                </div>
            </div>
        </div>

        <ItemDialog
            v-if="itemDialogVisible"
            :visible="itemDialogVisible"
            :form="itemForm"
            :saving="savingItem"
            :initial-tab="itemDialogInitialTab"
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
    </div>
</template>
