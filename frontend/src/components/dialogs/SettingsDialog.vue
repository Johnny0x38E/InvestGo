<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Dialog from "primevue/dialog";
import InputNumber from "primevue/inputnumber";
import Select from "primevue/select";
import ToggleSwitch from "primevue/toggleswitch";
import appDockMark from "../../assets/app-dock.svg";
import { api } from "../../api";

import {
    getAmountDisplayOptions,
    getColorThemeOptions,
    getCurrencyDisplayOptions,
    getDashboardCurrencyOptions,
    getFontPresetOptions,
    getLocaleOptions,
    getPriceColorOptions,
    projectMeta,
    getSettingsTabs,
    getThemeModeOptions,
} from "../../constants";
import { formatDateTime } from "../../format";
import { useI18n } from "../../i18n";
import type { AppSettings, DeveloperLogEntry, QuoteSourceOption, RuntimeStatus, SettingsTabKey } from "../../types";

const props = defineProps<{
    visible: boolean;
    settingsTab: SettingsTabKey;
    settingsDraft: AppSettings;
    quoteSources: QuoteSourceOption[];
    runtime: RuntimeStatus;
    itemCount: number;
    storagePath: string;
    logFilePath: string;
    developerLogs: DeveloperLogEntry[];
    saving: boolean;
    loadingLogs: boolean;
}>();

const emit = defineEmits<{
    (event: "update:visible", value: boolean): void;
    (event: "update:settingsTab", value: SettingsTabKey): void;
    (event: "save"): void;
    (event: "refresh-logs"): void;
    (event: "copy-logs"): void;
    (event: "clear-logs"): void;
}>();

const visibleProxy = computed({
    get: () => props.visible,
    set: (value: boolean) => emit("update:visible", value),
});

const settingsTabProxy = computed({
    get: () => props.settingsTab,
    set: (value: SettingsTabKey) => emit("update:settingsTab", value),
});

const developerLogCount = computed(() => props.developerLogs.length);
const { locale, t } = useI18n();
const settingsTabs = computed(() => getSettingsTabs());
const themeModeOptions = computed(() => getThemeModeOptions());
const colorThemeOptions = computed(() => getColorThemeOptions());
const fontPresetOptions = computed(() => getFontPresetOptions());
const amountDisplayOptions = computed(() => getAmountDisplayOptions());
const currencyDisplayOptions = computed(() => getCurrencyDisplayOptions());
const priceColorOptions = computed(() => getPriceColorOptions());
const dashboardCurrencyOptions = computed(() => getDashboardCurrencyOptions());
const localeOptions = computed(() => getLocaleOptions());

async function openExternal(url: string): Promise<void> {
    await api("/api/open-external", {
        method: "POST",
        body: JSON.stringify({ url }),
    });
}
</script>

<template>
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="t('settings.title')" :style="{ width: '980px' }" class="desk-dialog settings-dialog">
        <div class="settings-layout">
            <aside class="settings-nav">
                <button
                    v-for="entry in settingsTabs"
                    :key="entry.key"
                    class="settings-nav-item"
                    :class="{ active: settingsTabProxy === entry.key }"
                    type="button"
                    @click="settingsTabProxy = entry.key"
                >
                    {{ entry.label }}
                </button>
            </aside>

            <section class="settings-body">
                <div v-show="settingsTabProxy === 'general'" class="settings-pane">
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.runtime") }}</h4>
                        <div class="settings-grid">
                            <label>
                                <span>{{ t("settings.labels.cnQuoteSource") }}</span>
                                <Select v-model="settingsDraft.cnQuoteSource" :options="quoteSources" option-label="name" option-value="id" class="w-full" />
                            </label>
                            <label>
                                <span>{{ t("settings.labels.hkQuoteSource") }}</span>
                                <Select v-model="settingsDraft.hkQuoteSource" :options="quoteSources" option-label="name" option-value="id" class="w-full" />
                            </label>
                            <label>
                                <span>{{ t("settings.labels.usQuoteSource") }}</span>
                                <Select v-model="settingsDraft.usQuoteSource" :options="quoteSources" option-label="name" option-value="id" class="w-full" />
                            </label>

                            <label>
                                <span>{{ t("settings.labels.refreshInterval") }}</span>
                                <InputNumber v-model="settingsDraft.refreshIntervalSeconds" :min="10" :step="10" fluid />
                            </label>
                        </div>
                        <!-- <p class="settings-note">自选列表的实时行情按境内（A股+境内ETF）/ 港股（含港股ETF）/ 美股（含美股ETF）分三组走对应数据源，图表在所选源不支持时自动回退。</p> -->
                    </div>

                    <div class="settings-section">
                        <h4>{{ t("settings.sections.runtimeStatus") }}</h4>
                        <div class="settings-meta-grid">
                            <article>
                                <span>{{ t("settings.labels.quoteSource") }}</span><strong>{{ runtime.quoteSource || "-" }}</strong>
                            </article>
                            <article>
                                <span>{{ t("settings.labels.liveCoverage") }}</span><strong>{{ runtime.livePriceCount }}/{{ itemCount }}</strong>
                            </article>
                            <article>
                                <span>{{ t("settings.labels.lastQuoteRefreshAt") }}</span><strong>{{ formatDateTime(runtime.lastQuoteRefreshAt) }}</strong>
                            </article>
                            <article>
                                <span>{{ t("settings.labels.lastQuoteAttemptAt") }}</span><strong>{{ formatDateTime(runtime.lastQuoteAttemptAt) }}</strong>
                            </article>
                            <article class="full-span">
                                <span>{{ t("settings.labels.lastQuoteError") }}</span><strong>{{ runtime.lastQuoteError || t("common.none") }}</strong>
                            </article>
                            <article>
                                <span>{{ t("settings.labels.lastFxRefreshAt") }}</span><strong>{{ formatDateTime(runtime.lastFxRefreshAt) }}</strong>
                            </article>
                            <article class="full-span">
                                <span>{{ t("settings.labels.lastFxError") }}</span><strong>{{ runtime.lastFxError || t("common.none") }}</strong>
                            </article>
                            <article class="full-span">
                                <span>{{ t("settings.labels.storagePath") }}</span><strong>{{ storagePath || "-" }}</strong>
                            </article>
                        </div>
                    </div>
                </div>

                <div v-show="settingsTabProxy === 'display'" class="settings-pane">
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.appearance") }}</h4>
                        <div class="settings-grid">
                            <label>
                                <span>{{ t("settings.labels.themeMode") }}</span>
                                <Select v-model="settingsDraft.themeMode" :options="themeModeOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>{{ t("settings.labels.colorTheme") }}</span>
                                <Select v-model="settingsDraft.colorTheme" :options="colorThemeOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>{{ t("settings.labels.fontPreset") }}</span>
                                <Select v-model="settingsDraft.fontPreset" :options="fontPresetOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>{{ t("settings.labels.amountDisplay") }}</span>
                                <Select v-model="settingsDraft.amountDisplay" :options="amountDisplayOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>{{ t("settings.labels.currencyDisplay") }}</span>
                                <Select v-model="settingsDraft.currencyDisplay" :options="currencyDisplayOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>{{ t("settings.labels.priceColorScheme") }}</span>
                                <Select v-model="settingsDraft.priceColorScheme" :options="priceColorOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>{{ t("settings.labels.dashboardCurrency") }}</span>
                                <Select v-model="settingsDraft.dashboardCurrency" :options="dashboardCurrencyOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                        </div>
                        <div class="settings-theme-preview">
                            <div class="settings-theme-preview-copy">
                                <strong>{{ t("settings.themePreview.title") }}</strong>
                                <span>{{ t("settings.themePreview.description") }}</span>
                            </div>
                            <div class="settings-theme-preview-swatches">
                                <span class="settings-theme-swatch accent">{{ t("settings.themePreview.accent") }}</span>
                                <span class="settings-theme-swatch rise">{{ t("settings.themePreview.rise") }}</span>
                                <span class="settings-theme-swatch fall">{{ t("settings.themePreview.fall") }}</span>
                            </div>
                            <div class="settings-theme-preview-actions" aria-hidden="true">
                                <Button size="small" :label="t('settings.themePreview.primary')" tabindex="-1" />
                                <Button size="small" outlined :label="t('settings.themePreview.secondary')" tabindex="-1" />
                                <Button size="small" text :label="t('settings.themePreview.text')" tabindex="-1" />
                            </div>
                        </div>
                    </div>

                    <div class="settings-section">
                        <h4>{{ t("settings.sections.window") }}</h4>
                        <label class="developer-toggle">
                            <div>
                                <span>{{ t("settings.labels.useNativeTitleBar") }}</span>
                            </div>
                            <ToggleSwitch v-model="settingsDraft.useNativeTitleBar" />
                        </label>
                    </div>
                </div>

                <div v-show="settingsTabProxy === 'region'" class="settings-pane">
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.region") }}</h4>
                        <div class="settings-grid">
                            <label>
                                <span>{{ t("settings.labels.locale") }}</span>
                                <Select v-model="settingsDraft.locale" :options="localeOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                        </div>
                    </div>
                </div>

                <div v-show="settingsTabProxy === 'developer'" class="settings-pane">
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.developerMode") }}</h4>
                        <label class="developer-toggle">
                            <div>
                                <span>{{ t("settings.labels.developerMode") }}</span>
                            </div>
                            <ToggleSwitch v-model="settingsDraft.developerMode" />
                        </label>
                    </div>

                    <div v-if="settingsDraft.developerMode" class="settings-section">
                        <div class="developer-toolbar">
                            <div class="developer-summary">
                                <strong>{{ t("settings.developer.recentLogs", { count: developerLogCount }) }}</strong>
                                <span>{{ loadingLogs ? t("settings.developer.loading") : t("settings.developer.idle") }}</span>
                            </div>
                            <div class="developer-actions">
                                <Button size="small" text icon="pi pi-refresh" :label="t('common.refresh')" @click="$emit('refresh-logs')" />
                                <Button size="small" text icon="pi pi-copy" :label="t('common.copy')" @click="$emit('copy-logs')" />
                                <Button size="small" text severity="danger" icon="pi pi-trash" :label="t('common.clear')" @click="$emit('clear-logs')" />
                            </div>
                        </div>

                        <div class="settings-meta-grid">
                            <article>
                                <span>{{ t("settings.labels.logCount") }}</span><strong>{{ developerLogCount }}</strong>
                            </article>
                            <article>
                                <span>{{ t("settings.labels.logFilePath") }}</span><strong>{{ logFilePath || "-" }}</strong>
                            </article>
                        </div>

                        <div class="developer-log-list">
                            <article v-for="entry in developerLogs" :key="entry.id" class="developer-log-entry" :data-level="entry.level">
                                <div class="developer-log-meta">
                                    <span class="developer-log-level">{{ entry.level.toUpperCase() }}</span>
                                    <span>{{ entry.source }}</span>
                                    <span>{{ entry.scope }}</span>
                                    <span>{{ formatDateTime(entry.timestamp) }}</span>
                                </div>
                                <pre>{{ entry.message }}</pre>
                            </article>

                            <div v-if="!developerLogs.length" class="developer-log-empty">{{ t("settings.developer.empty") }}</div>
                        </div>
                    </div>
                </div>

                <div v-show="settingsTabProxy === 'about'" class="settings-pane">
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.about") }}</h4>
                        <div class="settings-about-card">
                            <div class="settings-about-brand">
                                <img :src="appDockMark" alt="InvestGo" />
                            </div>
                            <div class="settings-about-summary">
                                <div class="settings-about-heading">
                                    <strong>InvestGo</strong>
                                    <span class="settings-about-version">v{{ runtime.appVersion || "dev" }}</span>
                                </div>
                                <p>{{ t("settings.about.description") }}</p>
                            </div>
                        </div>

                        <div class="settings-about-links">
                            <Button size="small" outlined icon="pi pi-github" :label="t('settings.about.repository')" class="settings-about-action" @click="openExternal(projectMeta.repositoryUrl)" />
                        </div>

                        <div class="settings-disclaimer-grid">
                            <section class="settings-disclaimer-card">
                                <div class="settings-disclaimer-header">
                                    <strong>{{ t("settings.about.disclaimer") }}</strong>
                                    <span>{{ t(`options.locale.${locale}`) }}</span>
                                </div>
                                <p>{{ t("settings.about.disclaimerParagraph1") }}</p>
                                <p>{{ t("settings.about.disclaimerParagraph2") }}</p>
                                <p>{{ t("settings.about.disclaimerParagraph3") }}</p>
                            </section>
                        </div>
                    </div>
                </div>
            </section>
        </div>

        <template #footer>
            <Button size="small" text :label="t('common.cancel')" @click="visibleProxy = false" />
            <Button size="small" :label="t('common.save')" :loading="saving" @click="$emit('save')" />
        </template>
    </Dialog>
</template>
