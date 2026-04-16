<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";

import { api } from "./api";
import AppHeader from "./components/AppHeader.vue";
import ModuleTabs from "./components/ModuleTabs.vue";
import SummaryStrip from "./components/SummaryStrip.vue";
import AlertDialog from "./components/dialogs/AlertDialog.vue";
import ConfirmDialog from "./components/dialogs/ConfirmDialog.vue";
import DCADetailDialog from "./components/dialogs/DCADetailDialog.vue";
import ItemDialog from "./components/dialogs/ItemDialog.vue";
import SettingsDialog from "./components/dialogs/SettingsDialog.vue";
import AlertsModule from "./components/modules/AlertsModule.vue";
import HotModule from "./components/modules/HotModule.vue";
import MarketModule from "./components/modules/MarketModule.vue";
import WatchlistModule from "./components/modules/WatchlistModule.vue";
import { appendClientLog, installClientLogCapture } from "./devlog";
import { useDeveloperLogs } from "./composables/useDeveloperLogs";
import { useHistorySeries } from "./composables/useHistorySeries";
import { defaultSettings, emptyAlertForm, emptyItemForm, mapAlertToForm, mapItemToForm, serialiseItemForm } from "./forms";
import { setFormatterSettings } from "./format";
import { setI18nLocale, translate } from "./i18n";
import { applyPrimeVueColorTheme } from "./theme";
import type { AlertFormModel, AlertRule, AppSettings, HotItem, ItemFormModel, ModuleKey, OptionItem, QuoteSourceOption, SettingsTabKey, StateSnapshot, StatusTone, WatchlistItem } from "./types";

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
const activeModule = ref<ModuleKey>("market");
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

const filteredItems = computed(() => {
    const keyword = search.value.trim().toLowerCase();
    if (!keyword) {
        return items.value;
    }

    return items.value.filter((item) => [item.symbol, item.name, item.market, item.thesis, ...(item.tags ?? [])].filter(Boolean).join(" ").toLowerCase().includes(keyword));
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
        // 数值格式等真正生效的业务设置仍以已保存值为准，避免草稿影响数据展示。
        setFormatterSettings(value);
        setI18nLocale(value.locale);
        document.documentElement.lang = value.locale === "system" ? navigator.language || "zh-CN" : value.locale;
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
        // 设置弹窗打开时，允许在当前界面先预览外观草稿；关闭后自动回到已保存状态。
        const appearance = settingsVisible.value ? settingsDraft : settings.value;
        document.documentElement.dataset.fontPreset = appearance.fontPreset;
        document.documentElement.dataset.colorTheme = appearance.colorTheme;
        document.documentElement.dataset.priceColorScheme = appearance.priceColorScheme;
        document.documentElement.dataset.themeMode = appearance.themeMode;
        applyPrimeVueColorTheme(appearance.colorTheme);
        applyResolvedTheme(appearance.themeMode);
    },
    { immediate: true },
);

watch(settingsVisible, (visible) => {
    if (visible) {
        Object.assign(settingsDraft, settings.value);
    }
});

const { historyInterval, historySeries, historyLoading, historyError, historyItemOptions, loadHistory, clearHistoryCache, selectHistoryInterval } = useHistorySeries(
    items,
    selectedItem,
    activeModule,
    setStatus,
);

const { developerLogs, loadingLogs, logFilePath, loadBackendLogs, clearDeveloperLogs, copyDeveloperLogs } = useDeveloperLogs(setStatus);

watch(
    () => [settingsVisible.value, settingsTab.value, settingsDraft.developerMode] as const,
    ([visible, tab, developerMode]) => {
        window.clearInterval(developerLogTimer);
        if (!visible || tab !== "developer" || !developerMode) {
            return;
        }

        // 只在开发者页签可见时轮询日志，避免无意义的后台请求。
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
});

// 同步系统主题到页面根节点，保持桌面应用的明暗跟随。
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

// 从后端拉取完整快照，供首次加载和手动刷新使用。
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
        setStatus(error instanceof Error ? error.message : translate("app.loadFailed"), "error");
    }
}

// 把后端快照灌回前端状态，并重置当前选中标的。
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

// 刷新实时行情，并按需要同步刷新当前图表范围。
async function refreshQuotes(silent = false, refreshHistory = true): Promise<void> {
    try {
        if (!silent) {
            setStatus(translate("app.syncingQuotes"), "success");
        }
        const snapshot = await api<StateSnapshot>("/api/refresh", { method: "POST" });
        applySnapshot(snapshot);
        if (refreshHistory && activeModule.value === "market" && selectedItem.value) {
            await loadHistory(true, true);
        }
        if (snapshot.runtime.lastQuoteError) {
            setStatus(snapshot.runtime.lastQuoteError, "error");
        } else if (snapshot.runtime.lastFxError) {
            setStatus(translate("app.quotesSyncedFxFailed", { error: snapshot.runtime.lastFxError }), "warn");
        } else if (!silent) {
            setStatus(translate("app.quotesSynced"), "success");
        }
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.refreshFailed"), "error");
    } finally {
        scheduleAutoRefresh();
    }
}

// 自动刷新按配置间隔继续下一次同步，用于所有区间图表的实时更新。
function scheduleAutoRefresh(): void {
    window.clearTimeout(refreshTimer);
    const intervalMs = Math.max(settings.value.refreshIntervalSeconds || 60, 10) * 1000;

    refreshTimer = window.setTimeout(() => {
        void refreshQuotes(true);
    }, intervalMs);
}

// 更新顶部状态栏文案和色调。
function setStatus(message: string, tone: StatusTone): void {
    statusText.value = message;
    statusTone.value = tone;
}

// 打开设置弹窗，并把当前设置复制到草稿对象。
function openSettings(): void {
    Object.assign(settingsDraft, settings.value);
    settingsVisible.value = true;
}

// 保存用户设置，并让后端返回新的完整快照。
async function saveSettings(): Promise<void> {
    savingSettings.value = true;
    try {
        const snapshot = await api<StateSnapshot>("/api/settings", {
            method: "PUT",
            body: JSON.stringify(settingsDraft),
        });
        applySnapshot(snapshot);
        settingsVisible.value = false;
        setStatus(translate("app.settingsSaved"), "success");
        // 设置保存后，如果当前在市场模块，刷新图表以确保使用新设置
        if (activeModule.value === "market" && selectedItem.value) {
            void loadHistory(true, true);
        }
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.settingsSaveFailed"), "error");
    } finally {
        savingSettings.value = false;
    }
}

// 打开标的编辑弹窗。
function openItemDialog(item?: WatchlistItem, initialTab: "basic" | "dca" = "basic"): void {
    Object.assign(itemForm, item ? mapItemToForm(item) : emptyItemForm());
    itemDialogInitialTab.value = initialTab;
    itemDialogVisible.value = true;
}

// 打开定投明细弹窗。
function showDCADetail(item: WatchlistItem): void {
    dcaDetailItem.value = item;
    dcaDetailVisible.value = true;
}

// 从定投明细弹窗切换到编辑弹窗的定投标签页。
function editFromDCADetail(): void {
    if (!dcaDetailItem.value) return;
    dcaDetailVisible.value = false;
    openItemDialog(dcaDetailItem.value, "dca");
}

// 保存标的并刷新缓存，确保当前图表继续对齐最新数据。
async function saveItem(): Promise<void> {
    savingItem.value = true;
    try {
        const payload = serialiseItemForm(itemForm);
        const path = itemForm.id ? `/api/items/${itemForm.id}` : "/api/items";
        const method = itemForm.id ? "PUT" : "POST";
        const snapshot = await api<StateSnapshot>(path, { method, body: JSON.stringify(payload) });
        clearHistoryCache();
        applySnapshot(snapshot);
        itemDialogVisible.value = false;
        setStatus(itemForm.id ? translate("app.itemUpdated") : translate("app.itemAdded"), "success");
        activeModule.value = "watchlist";
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.itemSaveFailed"), "error");
    } finally {
        savingItem.value = false;
    }
}

// 将热门榜单中的标的快速加入观察列表。
async function quickAddHotItem(item: HotItem): Promise<void> {
    const key = `${item.market}:${item.symbol}`;
    if (trackedHotKeys.value.includes(key)) {
        setStatus(translate("app.itemAlreadyTracked"), "warn");
        return;
    }

    try {
        // 快速加入只写入持仓基础信息，当前价仍由统一行情源回填。
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
        setStatus(translate("app.hotItemAdded", { symbol: item.symbol }), "success");
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.addItemFailed"), "error");
    }
}

// 删除标的时同步清空相关历史缓存。
async function performDeleteItem(id: string): Promise<void> {
    try {
        const snapshot = await api<StateSnapshot>(`/api/items/${id}`, { method: "DELETE" });
        clearHistoryCache();
        applySnapshot(snapshot);
        setStatus(translate("app.itemDeleted"), "success");
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.deleteFailed"), "error");
    }
}

// 打开提醒编辑弹窗。
function openAlertDialog(alert?: AlertRule): void {
    Object.assign(alertForm, alert ? mapAlertToForm(alert) : emptyAlertForm(items.value[0]?.id));
    alertDialogVisible.value = true;
}

// 保存提醒规则，并把列表切换到提醒模块。
async function saveAlert(): Promise<void> {
    savingAlert.value = true;
    try {
        const path = alertForm.id ? `/api/alerts/${alertForm.id}` : "/api/alerts";
        const method = alertForm.id ? "PUT" : "POST";
        const snapshot = await api<StateSnapshot>(path, {
            method,
            body: JSON.stringify(alertForm),
        });
        applySnapshot(snapshot);
        alertDialogVisible.value = false;
        setStatus(alertForm.id ? translate("app.alertUpdated") : translate("app.alertAdded"), "success");
        activeModule.value = "alerts";
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.alertSaveFailed"), "error");
    } finally {
        savingAlert.value = false;
    }
}

// 删除提醒规则。
async function performDeleteAlert(id: string): Promise<void> {
    try {
        const snapshot = await api<StateSnapshot>(`/api/alerts/${id}`, { method: "DELETE" });
        applySnapshot(snapshot);
        setStatus(translate("app.alertDeleted"), "success");
    } catch (error) {
        setStatus(error instanceof Error ? error.message : translate("app.deleteFailed"), "error");
    }
}

// 记录待删除对象，并弹出二次确认。
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

// 执行确认删除。
async function confirmDelete(): Promise<void> {
    if (!pendingDelete.kind || !pendingDelete.id) {
        confirmDialogVisible.value = false;
        return;
    }

    deleting.value = true;
    try {
        // 删除类型在确认前已经固化到 pendingDelete，这里只负责执行对应动作。
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

// 切换主模块，进入市场模块时补拉当前图表。
function switchModule(next: ModuleKey): void {
    appendClientLog("info", "tabs", `switch module ${activeModule.value} -> ${next}`);
    activeModule.value = next;
    if (next === "market") {
        void loadHistory(true);
    }
}
</script>

<template>
    <div class="app-shell">
        <AppHeader :status-text="statusText" :status-tone="statusTone" :generated-at="generatedAt" @open-settings="openSettings" />

        <SummaryStrip :dashboard="dashboard" :item-count="items.length" :live-price-count="runtime.livePriceCount" />

        <section class="workspace-panel">
            <ModuleTabs :active-module="activeModule" @switch="switchModule" />

            <div class="workspace-stage">
                <MarketModule
                    v-if="activeModule === 'market'"
                    :selected-item="selectedItem"
                    :selected-item-id="selectedItemId"
                    :history-interval="historyInterval"
                    :history-item-options="historyItemOptions"
                    :history-series="historySeries"
                    :history-loading="historyLoading"
                    :history-error="historyError"
                    @refresh="refreshQuotes()"
                    @update:selected-item-id="selectedItemId = $event"
                    @select-interval="selectHistoryInterval"
                />

                <HotModule v-else-if="activeModule === 'hot'" :tracked-keys="trackedHotKeys" @add-item="quickAddHotItem" />

                <WatchlistModule
                    v-else-if="activeModule === 'watchlist'"
                    :search="search"
                    :filtered-items="filteredItems"
                    :selected-item-id="selectedItemId"
                    @update:search="search = $event"
                    @add-item="openItemDialog()"
                    @edit-item="openItemDialog"
                    @delete-item="requestDeleteItem"
                    @select-item="selectedItemId = $event"
                    @show-dca="showDCADetail"
                />

                <AlertsModule v-else :alerts="alerts" :items="items" @add-alert="openAlertDialog()" @edit-alert="openAlertDialog" @delete-alert="requestDeleteAlert" />
            </div>
        </section>

        <SettingsDialog
            v-if="settingsVisible"
            :visible="settingsVisible"
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
            @update:visible="settingsVisible = $event"
            @update:settings-tab="settingsTab = $event"
            @save="saveSettings"
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
            @update:visible="itemDialogVisible = $event"
            @save="saveItem"
        />

        <DCADetailDialog v-if="dcaDetailVisible" :visible="dcaDetailVisible" :item="dcaDetailItem" @update:visible="dcaDetailVisible = $event" @edit="editFromDCADetail" />

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
