<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import InputNumber from "primevue/inputnumber";
import Select from "primevue/select";
import ToggleSwitch from "primevue/toggleswitch";
import appMark from "../../assets/app-mark.svg";
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
    COLOR_THEME_SWATCHES,
} from "../../constants";
import { formatDateTime } from "../../format";
import { useI18n } from "../../i18n";
import type {
    AppSettings,
    DeveloperLogEntry,
    QuoteSourceOption,
    RuntimeStatus,
    SettingsTabKey,
} from "../../types";

const props = defineProps<{
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
    (event: "update:settingsTab", value: SettingsTabKey): void;
    (event: "save"): void;
    (event: "refresh-logs"): void;
    (event: "copy-logs"): void;
    (event: "clear-logs"): void;
    (event: "cancel"): void;
}>();

const settingsTabProxy = computed({
    get: () => props.settingsTab,
    set: (value: SettingsTabKey) => emit("update:settingsTab", value),
});

const developerLogCount = computed(() => props.developerLogs.length);
const { t } = useI18n();
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
    <section class="module-content settings-module">
        <div class="panel-header">
            <div>
                <h3 class="title">{{ t("settings.title") }}</h3>
            </div>
            <div class="toolbar-row">
                <Button
                    size="small"
                    text
                    :label="t('common.cancel')"
                    @click="$emit('cancel')"
                />
                <Button
                    size="small"
                    :label="t('common.save')"
                    :loading="saving"
                    @click="$emit('save')"
                />
            </div>
        </div>

        <div class="settings-layout">
            <nav
                class="settings-nav"
                role="tablist"
                :aria-label="t('settings.title')"
            >
                <button
                    v-for="entry in settingsTabs"
                    :key="entry.key"
                    class="settings-nav-item"
                    :class="{ active: settingsTabProxy === entry.key }"
                    :aria-selected="settingsTabProxy === entry.key"
                    role="tab"
                    type="button"
                    @click="settingsTabProxy = entry.key"
                >
                    {{ entry.label }}
                </button>
            </nav>

            <section class="settings-body">
                <div
                    v-show="settingsTabProxy === 'general'"
                    class="settings-pane"
                >
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.runtime") }}</h4>
                        <div class="settings-grid">
                            <label>
                                <span>{{
                                    t("settings.labels.cnQuoteSource")
                                }}</span>
                                <Select
                                    v-model="settingsDraft.cnQuoteSource"
                                    :options="quoteSources"
                                    option-label="name"
                                    option-value="id"
                                    class="w-full"
                                />
                            </label>
                            <label>
                                <span>{{
                                    t("settings.labels.hkQuoteSource")
                                }}</span>
                                <Select
                                    v-model="settingsDraft.hkQuoteSource"
                                    :options="quoteSources"
                                    option-label="name"
                                    option-value="id"
                                    class="w-full"
                                />
                            </label>
                            <label>
                                <span>{{
                                    t("settings.labels.usQuoteSource")
                                }}</span>
                                <Select
                                    v-model="settingsDraft.usQuoteSource"
                                    :options="quoteSources"
                                    option-label="name"
                                    option-value="id"
                                    class="w-full"
                                />
                            </label>

                            <label>
                                <span>{{
                                    t("settings.labels.refreshInterval")
                                }}</span>
                                <InputNumber
                                    v-model="
                                        settingsDraft.refreshIntervalSeconds
                                    "
                                    :min="10"
                                    :step="10"
                                    fluid
                                />
                            </label>
                        </div>
                        <!-- <p class="settings-note">Watchlist live quotes are grouped by mainland China (A-shares plus domestic ETFs), Hong Kong (including HK ETFs), and US markets (including US ETFs), and charts automatically fall back when the selected source does not support them.</p> -->
                    </div>

                    <div class="settings-section">
                        <h4>{{ t("settings.sections.runtimeStatus") }}</h4>
                        <div class="settings-meta-grid">
                            <article>
                                <span>{{
                                    t("settings.labels.quoteSource")
                                }}</span
                                ><strong>{{
                                    runtime.quoteSource || "-"
                                }}</strong>
                            </article>
                            <article>
                                <span>{{
                                    t("settings.labels.liveCoverage")
                                }}</span
                                ><strong
                                    >{{ runtime.livePriceCount }}/{{
                                        itemCount
                                    }}</strong
                                >
                            </article>
                            <article>
                                <span>{{
                                    t("settings.labels.lastQuoteRefreshAt")
                                }}</span
                                ><strong>{{
                                    formatDateTime(runtime.lastQuoteRefreshAt)
                                }}</strong>
                            </article>
                            <article>
                                <span>{{
                                    t("settings.labels.lastQuoteAttemptAt")
                                }}</span
                                ><strong>{{
                                    formatDateTime(runtime.lastQuoteAttemptAt)
                                }}</strong>
                            </article>
                            <article class="full-span">
                                <span>{{
                                    t("settings.labels.lastQuoteError")
                                }}</span
                                ><strong>{{
                                    runtime.lastQuoteError || t("common.none")
                                }}</strong>
                            </article>
                            <article>
                                <span>{{
                                    t("settings.labels.lastFxRefreshAt")
                                }}</span
                                ><strong>{{
                                    formatDateTime(runtime.lastFxRefreshAt)
                                }}</strong>
                            </article>
                            <article class="full-span">
                                <span>{{
                                    t("settings.labels.lastFxError")
                                }}</span
                                ><strong>{{
                                    runtime.lastFxError || t("common.none")
                                }}</strong>
                            </article>
                            <article class="full-span">
                                <span>{{
                                    t("settings.labels.storagePath")
                                }}</span
                                ><strong>{{ storagePath || "-" }}</strong>
                            </article>
                        </div>
                    </div>
                </div>

                <div
                    v-show="settingsTabProxy === 'display'"
                    class="settings-pane"
                >
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.appearance") }}</h4>
                        <div class="settings-grid">
                            <label>
                                <span>{{
                                    t("settings.labels.themeMode")
                                }}</span>
                                <Select
                                    v-model="settingsDraft.themeMode"
                                    :options="themeModeOptions"
                                    option-label="label"
                                    option-value="value"
                                    class="w-full"
                                />
                            </label>
                            <label class="color-theme-label">
                                <span>{{ t("settings.labels.colorTheme") }}</span>
                                <div class="color-theme-swatches">
                                    <button
                                        v-for="opt in colorThemeOptions"
                                        :key="opt.value"
                                        type="button"
                                        :title="opt.label"
                                        :class="['color-swatch-btn', { active: settingsDraft.colorTheme === opt.value }]"
                                        :style="{ '--swatch-color': COLOR_THEME_SWATCHES[opt.value] }"
                                        @click="settingsDraft.colorTheme = opt.value"
                                    />
                                </div>
                            </label>
                            <label>
                                <span>{{
                                    t("settings.labels.fontPreset")
                                }}</span>
                                <Select
                                    v-model="settingsDraft.fontPreset"
                                    :options="fontPresetOptions"
                                    option-label="label"
                                    option-value="value"
                                    class="w-full"
                                />
                            </label>
                            <label>
                                <span>{{
                                    t("settings.labels.amountDisplay")
                                }}</span>
                                <Select
                                    v-model="settingsDraft.amountDisplay"
                                    :options="amountDisplayOptions"
                                    option-label="label"
                                    option-value="value"
                                    class="w-full"
                                />
                            </label>
                            <label>
                                <span>{{
                                    t("settings.labels.currencyDisplay")
                                }}</span>
                                <Select
                                    v-model="settingsDraft.currencyDisplay"
                                    :options="currencyDisplayOptions"
                                    option-label="label"
                                    option-value="value"
                                    class="w-full"
                                />
                            </label>
                            <label>
                                <span>{{
                                    t("settings.labels.priceColorScheme")
                                }}</span>
                                <Select
                                    v-model="settingsDraft.priceColorScheme"
                                    :options="priceColorOptions"
                                    option-label="label"
                                    option-value="value"
                                    class="w-full"
                                />
                            </label>
                            <label>
                                <span>{{
                                    t("settings.labels.dashboardCurrency")
                                }}</span>
                                <Select
                                    v-model="settingsDraft.dashboardCurrency"
                                    :options="dashboardCurrencyOptions"
                                    option-label="label"
                                    option-value="value"
                                    class="w-full"
                                />
                            </label>
                        </div>
                        <div class="settings-theme-preview">
                            <div class="settings-theme-preview-copy">
                                <strong>{{
                                    t("settings.themePreview.title")
                                }}</strong>
                                <span>{{
                                    t("settings.themePreview.description")
                                }}</span>
                            </div>
                            <div class="settings-theme-preview-swatches">
                                <span class="settings-theme-swatch accent">{{
                                    t("settings.themePreview.accent")
                                }}</span>
                                <span class="settings-theme-swatch rise">{{
                                    t("settings.themePreview.rise")
                                }}</span>
                                <span class="settings-theme-swatch fall">{{
                                    t("settings.themePreview.fall")
                                }}</span>
                            </div>
                            <div
                                class="settings-theme-preview-actions"
                                aria-hidden="true"
                            >
                                <Button
                                    size="small"
                                    :label="t('settings.themePreview.primary')"
                                    tabindex="-1"
                                />
                                <Button
                                    size="small"
                                    outlined
                                    :label="
                                        t('settings.themePreview.secondary')
                                    "
                                    tabindex="-1"
                                />
                                <Button
                                    size="small"
                                    text
                                    :label="t('settings.themePreview.text')"
                                    tabindex="-1"
                                />
                            </div>
                        </div>
                    </div>

                    <div class="settings-section">
                        <h4>{{ t("settings.sections.window") }}</h4>
                        <label class="developer-toggle">
                            <div>
                                <span>{{
                                    t("settings.labels.useNativeTitleBar")
                                }}</span>
                            </div>
                            <ToggleSwitch
                                v-model="settingsDraft.useNativeTitleBar"
                            />
                        </label>
                    </div>
                </div>

                <div
                    v-show="settingsTabProxy === 'region'"
                    class="settings-pane"
                >
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.region") }}</h4>
                        <div class="settings-grid">
                            <label>
                                <span>{{ t("settings.labels.locale") }}</span>
                                <Select
                                    v-model="settingsDraft.locale"
                                    :options="localeOptions"
                                    option-label="label"
                                    option-value="value"
                                    class="w-full"
                                />
                            </label>
                        </div>
                    </div>
                </div>

                <div
                    v-show="settingsTabProxy === 'developer'"
                    class="settings-pane"
                >
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.developerMode") }}</h4>
                        <label class="developer-toggle">
                            <div>
                                <span>{{
                                    t("settings.labels.developerMode")
                                }}</span>
                            </div>
                            <ToggleSwitch
                                v-model="settingsDraft.developerMode"
                            />
                        </label>
                    </div>

                    <div
                        v-if="settingsDraft.developerMode"
                        class="settings-section"
                    >
                        <div class="developer-toolbar">
                            <div class="developer-summary">
                                <strong>{{
                                    t("settings.developer.recentLogs", {
                                        count: developerLogCount,
                                    })
                                }}</strong>
                                <span>{{
                                    loadingLogs
                                        ? t("settings.developer.loading")
                                        : t("settings.developer.idle")
                                }}</span>
                            </div>
                            <div class="developer-actions">
                                <Button
                                    size="small"
                                    text
                                    icon="pi pi-refresh"
                                    :label="t('common.refresh')"
                                    @click="$emit('refresh-logs')"
                                />
                                <Button
                                    size="small"
                                    text
                                    icon="pi pi-copy"
                                    :label="t('common.copy')"
                                    @click="$emit('copy-logs')"
                                />
                                <Button
                                    size="small"
                                    text
                                    severity="danger"
                                    icon="pi pi-trash"
                                    :label="t('common.clear')"
                                    @click="$emit('clear-logs')"
                                />
                            </div>
                        </div>

                        <div class="settings-meta-grid">
                            <article>
                                <span>{{ t("settings.labels.logCount") }}</span
                                ><strong>{{ developerLogCount }}</strong>
                            </article>
                            <article>
                                <span>{{
                                    t("settings.labels.logFilePath")
                                }}</span
                                ><strong>{{ logFilePath || "-" }}</strong>
                            </article>
                        </div>

                        <div class="developer-log-list">
                            <article
                                v-for="entry in developerLogs"
                                :key="entry.id"
                                class="developer-log-entry"
                                :data-level="entry.level"
                            >
                                <div class="developer-log-meta">
                                    <span class="developer-log-level">{{
                                        entry.level.toUpperCase()
                                    }}</span>
                                    <span>{{ entry.source }}</span>
                                    <span>{{ entry.scope }}</span>
                                    <span>{{
                                        formatDateTime(entry.timestamp)
                                    }}</span>
                                </div>
                                <pre>{{ entry.message }}</pre>
                            </article>

                            <div
                                v-if="!developerLogs.length"
                                class="developer-log-empty"
                            >
                                {{ t("settings.developer.empty") }}
                            </div>
                        </div>
                    </div>
                </div>

                <div
                    v-show="settingsTabProxy === 'about'"
                    class="settings-pane"
                >
                    <div class="settings-section">
                        <h4>{{ t("settings.sections.about") }}</h4>
                        <div class="settings-about-card">
                            <div class="settings-about-brand">
                                <img :src="appMark" alt="InvestGo" />
                            </div>
                            <div class="settings-about-summary">
                                <div class="settings-about-heading">
                                    <strong>InvestGo</strong>
                                    <span class="settings-about-version"
                                        >v{{
                                            runtime.appVersion || "dev"
                                        }}</span
                                    >
                                </div>
                                <p>{{ t("settings.about.description") }}</p>
                            </div>
                        </div>

                        <div class="settings-about-links">
                            <Button
                                size="small"
                                outlined
                                icon="pi pi-github"
                                :label="t('settings.about.repository')"
                                class="settings-about-action"
                                @click="openExternal(projectMeta.repositoryUrl)"
                            />
                        </div>

                        <section class="settings-disclaimer-card">
                            <div class="settings-disclaimer-header">
                                <strong>{{
                                    t("settings.about.disclaimer")
                                }}</strong>
                            </div>
                            <p>
                                {{ t("settings.about.disclaimerParagraph1") }}
                            </p>
                            <p>
                                {{ t("settings.about.disclaimerParagraph2") }}
                            </p>
                            <p>
                                {{ t("settings.about.disclaimerParagraph3") }}
                            </p>
                        </section>
                    </div>
                </div>
            </section>
        </div>
    </section>
</template>

<style scoped>
:deep(.p-dialog-content) {
    height: 640px;
    overflow: hidden;
}

.settings-layout {
    height: 100%;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding-bottom: 24px;
}

.settings-nav {
    display: flex;
    flex-direction: row;
    gap: 8px;
    padding: 6px;
    border-bottom: 1px solid var(--border);
    margin-bottom: 8px;
    background: linear-gradient(180deg, color-mix(in srgb, var(--panel-soft) 94%, var(--accent-soft)) 0%, var(--panel-strong) 100%);
    box-shadow: var(--shadow-soft);
    border-radius: calc(var(--radius-panel) + 2px);
}

.settings-nav-item {
    min-height: 36px;
    padding: 0 16px;
    border-radius: calc(var(--radius-control) - 2px);
    border: 1px solid transparent;
    background: transparent;
    color: var(--muted);
    font: 600 12px/1 var(--font-ui);
    text-align: center;
    cursor: pointer;
    transition:
        background 140ms ease,
        border-color 140ms ease,
        color 140ms ease,
        transform 140ms ease,
        box-shadow 140ms ease;
}

.settings-nav-item:hover {
    background: color-mix(in srgb, var(--accent-soft) 48%, var(--panel-strong));
    color: var(--ink);
}

.settings-nav-item.active {
    color: var(--accent-strong);
    border-color: color-mix(in srgb, var(--accent) 24%, var(--border));
    background: linear-gradient(180deg, color-mix(in srgb, var(--accent-soft) 88%, var(--panel-strong)) 0%, color-mix(in srgb, var(--accent-soft) 34%, var(--panel-strong)) 100%);
    box-shadow:
        0 10px 18px color-mix(in srgb, var(--accent-soft) 34%, transparent),
        var(--shadow-soft);
}

.settings-body {
    min-height: 0;
    overflow: hidden;
    flex: 1;
}

.settings-pane {
    height: 100%;
    overflow-y: auto;
    overflow-x: hidden;
    display: grid;
    gap: 10px;
    align-content: start;
    padding-right: 6px;
}

.settings-section {
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: linear-gradient(180deg, color-mix(in srgb, var(--panel-soft) 92%, var(--accent-soft)) 0%, var(--panel-soft) 100%);
    padding: 12px;
    display: grid;
    gap: 12px;
    box-shadow: var(--shadow-soft);
}

.settings-section h4 {
    margin: 0;
    color: var(--ink);
    font: 700 14px/1.2 var(--font-display);
    letter-spacing: 0.01em;
}

.settings-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
}

.settings-section label {
    display: grid;
    gap: 7px;
}

.w-full,
.settings-grid :is(.p-select, .p-inputnumber, .p-inputtext, .p-textarea) {
    width: 100%;
}


.color-theme-label {
    display: flex;
    flex-direction: column;
    gap: 8px;
}

.color-theme-swatches {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    padding: 4px 0;
}

.color-swatch-btn {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background-color: var(--swatch-color);
    border: 2.5px solid transparent;
    cursor: pointer;
    padding: 0;
    position: relative;
    transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
    outline: none;
}

.color-swatch-btn:hover {
    transform: scale(1.15);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.25);
}

.color-swatch-btn.active {
    border-color: var(--ink);
    transform: scale(1.1);
    box-shadow: 0 0 0 1px var(--ink), 0 2px 8px rgba(0, 0, 0, 0.2);
}
.settings-theme-preview {
    display: grid;
    gap: 12px;
    border: 1px solid color-mix(in srgb, var(--accent) 20%, var(--border));
    border-radius: calc(var(--radius-panel) + 2px);
    padding: 14px;
    background:
        radial-gradient(circle at top right, color-mix(in srgb, var(--accent-soft) 135%, transparent) 0%, transparent 44%),
        linear-gradient(180deg, color-mix(in srgb, var(--panel-strong) 86%, var(--accent-soft)) 0%, var(--panel-strong) 100%);
    box-shadow: var(--shadow-soft);
}

.settings-theme-preview-copy {
    display: grid;
    gap: 4px;
}

.settings-theme-preview-copy strong {
    font-size: 13px;
}

.settings-theme-preview-copy span {
    color: var(--muted);
    line-height: 1.6;
}

.settings-theme-preview-swatches,
.settings-theme-preview-actions {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
}

.settings-theme-swatch {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-height: 34px;
    padding: 0 12px;
    border-radius: var(--radius-control);
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.01em;
    border: 1px solid transparent;
}

.settings-theme-swatch.accent {
    background: var(--accent-soft);
    border-color: color-mix(in srgb, var(--accent) 22%, transparent);
    color: var(--accent-strong);
}

.settings-theme-swatch.rise {
    background: color-mix(in srgb, var(--rise) 12%, var(--panel-strong));
    border-color: color-mix(in srgb, var(--rise) 18%, transparent);
    color: var(--rise);
}

.settings-theme-swatch.fall {
    background: color-mix(in srgb, var(--fall) 12%, var(--panel-strong));
    border-color: color-mix(in srgb, var(--fall) 18%, transparent);
    color: var(--fall);
}

.settings-theme-preview-actions :deep(.p-button) {
    pointer-events: none;
}

.settings-meta-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
}

.settings-meta-grid article {
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: var(--panel-strong);
    padding: 10px;
    display: grid;
    gap: 5px;
}

.settings-meta-grid article span {
    font-size: 11px;
    color: var(--muted);
}

.settings-meta-grid article strong {
    font-size: 13px;
    word-break: break-word;
}

.settings-meta-grid .full-span {
    grid-column: 1 / -1;
}

.developer-toggle {
    min-height: 52px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
}

.developer-toggle > div {
    display: grid;
    gap: 4px;
}

.developer-toolbar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 14px;
    flex-wrap: wrap;
}

.developer-summary {
    display: grid;
    gap: 4px;
}

.developer-summary span {
    color: var(--muted);
    font-size: 12px;
}

.developer-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
}

.developer-log-list {
    min-height: 0;
    overflow: visible;
    display: grid;
    gap: 10px;
}

.developer-log-entry {
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: var(--panel-strong);
    padding: 10px;
    display: grid;
    gap: 8px;
}

.developer-log-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 10px;
    font-size: 11px;
    color: var(--muted);
}

.developer-log-level {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 48px;
    padding: 2px 8px;
    border-radius: 999px;
    background: rgba(148, 163, 184, 0.14);
    color: var(--ink);
    font-weight: 700;
}

.developer-log-entry[data-level="error"] .developer-log-level {
    background: rgba(209, 67, 67, 0.14);
    color: var(--rise);
}

.developer-log-entry[data-level="warn"] .developer-log-level {
    background: rgba(217, 119, 6, 0.14);
    color: var(--warn);
}

.developer-log-entry[data-level="info"] .developer-log-level {
    background: var(--accent-soft);
    color: var(--accent-strong);
}

.developer-log-entry[data-level="debug"] .developer-log-level {
    background: rgba(15, 23, 42, 0.08);
    color: var(--muted);
}

.developer-log-entry pre {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    font:
        12px/1.6 "SF Mono",
        "JetBrains Mono",
        "Menlo",
        monospace;
    color: var(--ink);
}

.developer-log-empty {
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-panel);
    padding: 16px;
    color: var(--muted);
    line-height: 1.6;
    background: rgba(148, 163, 184, 0.06);
}

.settings-about {
    display: grid;
    gap: 8px;
    color: var(--muted);
    line-height: 1.6;
}

.settings-about p {
    margin: 0;
}

.settings-about-link {
    color: var(--accent);
    text-decoration: none;
    word-break: break-all;
}

.settings-about-link:hover {
    color: var(--accent-strong);
    text-decoration: underline;
}

.settings-about-card {
    display: grid;
    grid-template-columns: 1fr;
    gap: 14px;
    justify-items: start;
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: var(--panel-strong);
    padding: 18px;
}

.settings-about-brand {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 92px;
    height: 92px;
}

.settings-about-brand img {
    display: block;
    width: 92px;
    height: 92px;
    object-fit: contain;
}

.settings-about-summary {
    display: grid;
    gap: 8px;
    min-width: 0;
}

.settings-about-summary p {
    margin: 0;
    color: var(--muted);
    line-height: 1.6;
}

.settings-about-heading {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
}

.settings-about-heading strong {
    font-size: 26px;
    font-family: var(--font-display);
}

.settings-about-version {
    display: inline-flex;
    align-items: center;
    min-height: 28px;
    padding: 0 10px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent-strong);
    font-size: 12px;
    font-weight: 700;
}

.settings-about-links {
    display: flex;
    gap: 10px;
}

.settings-about-action {
    justify-self: start;
}

.settings-disclaimer-card {
    display: grid;
    gap: 10px;
    align-content: start;
    border: 1px solid var(--border);
    border-radius: var(--radius-panel);
    background: var(--panel-strong);
    padding: 14px;
}

.settings-disclaimer-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
}

.settings-disclaimer-header strong {
    font-size: 14px;
}

.settings-disclaimer-card p {
    margin: 0;
    color: var(--muted);
    line-height: 1.68;
    font-size: 12px;
}

@media (max-width: 1180px) {
    .settings-grid,
    .settings-meta-grid {
        grid-template-columns: 1fr;
    }
}
</style>
