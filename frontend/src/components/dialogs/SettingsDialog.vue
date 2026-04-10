<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Dialog from "primevue/dialog";
import InputNumber from "primevue/inputnumber";
import Select from "primevue/select";
import ToggleSwitch from "primevue/toggleswitch";

import { amountDisplayOptions, colorThemeOptions, currencyDisplayOptions, dashboardCurrencyOptions, fontPresetOptions, localeOptions, priceColorOptions, settingsTabs, themeModeOptions } from "../../constants";
import { formatDateTime } from "../../format";
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
</script>

<template>
    <Dialog v-model:visible="visibleProxy" modal :closable="false" header="设置" :style="{ width: '980px' }" class="desk-dialog settings-dialog">
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
                        <h4>运行</h4>
                        <div class="settings-grid">
                            <label>
                                <span>A股 / 境内ETF 行情源</span>
                                <Select v-model="settingsDraft.cnQuoteSource" :options="quoteSources" option-label="name" option-value="id" class="w-full" />
                            </label>
                            <label>
                                <span>港股 / 港股ETF 行情源</span>
                                <Select v-model="settingsDraft.hkQuoteSource" :options="quoteSources" option-label="name" option-value="id" class="w-full" />
                            </label>
                            <label>
                                <span>美股 / 美股ETF 行情源</span>
                                <Select v-model="settingsDraft.usQuoteSource" :options="quoteSources" option-label="name" option-value="id" class="w-full" />
                            </label>

                            <label>
                                <span>自动刷新间隔</span>
                                <InputNumber v-model="settingsDraft.refreshIntervalSeconds" :min="10" :step="10" fluid />
                            </label>
                        </div>
                        <p class="settings-note">自选列表的实时行情按境内（A股+境内ETF）/ 港股（含港股ETF）/ 美股（含美股ETF）分三组走对应数据源，图表在所选源不支持时自动回退。</p>
                    </div>

                    <div class="settings-section">
                        <h4>运行状态</h4>
                        <div class="settings-meta-grid">
                            <article>
                                <span>行情源</span><strong>{{ runtime.quoteSource || "-" }}</strong>
                            </article>
                            <article>
                                <span>同步覆盖</span><strong>{{ runtime.livePriceCount }}/{{ itemCount }}</strong>
                            </article>
                            <article>
                                <span>上次成功同步</span><strong>{{ formatDateTime(runtime.lastQuoteRefreshAt) }}</strong>
                            </article>
                            <article>
                                <span>最近一次尝试</span><strong>{{ formatDateTime(runtime.lastQuoteAttemptAt) }}</strong>
                            </article>
                            <article class="full-span">
                                <span>同步问题</span><strong>{{ runtime.lastQuoteError || "无" }}</strong>
                            </article>
                            <article class="full-span">
                                <span>本地状态文件</span><strong>{{ storagePath || "-" }}</strong>
                            </article>
                        </div>
                    </div>
                </div>

                <div v-show="settingsTabProxy === 'display'" class="settings-pane">
                    <div class="settings-section">
                        <h4>外观与金额展示</h4>
                        <div class="settings-grid">
                            <label>
                                <span>外观模式</span>
                                <Select v-model="settingsDraft.themeMode" :options="themeModeOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>界面配色</span>
                                <Select v-model="settingsDraft.colorTheme" :options="colorThemeOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>全局字体</span>
                                <Select v-model="settingsDraft.fontPreset" :options="fontPresetOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>金额展示</span>
                                <Select v-model="settingsDraft.amountDisplay" :options="amountDisplayOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>币种显示</span>
                                <Select v-model="settingsDraft.currencyDisplay" :options="currencyDisplayOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>涨跌配色</span>
                                <Select v-model="settingsDraft.priceColorScheme" :options="priceColorOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                            <label>
                                <span>组合展示货币</span>
                                <Select v-model="settingsDraft.dashboardCurrency" :options="dashboardCurrencyOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                        </div>
                        <p class="settings-note">外观模式控制亮色、暗色或跟随系统。界面配色影响强调色、选中态与按钮层次；涨跌配色仍由下方的“涨跌配色”单独控制。多币种持仓会按当前汇率折算后统一展示。</p>
                    </div>

                    <div class="settings-section">
                        <h4>窗口</h4>
                        <label class="developer-toggle">
                            <div>
                                <span>使用原生标题栏</span>
                                <small>启用后可使用 macOS 原生双击标题栏缩放功能，但会失去自定义标题栏样式。修改后需重启应用生效。</small>
                            </div>
                            <ToggleSwitch v-model="settingsDraft.useNativeTitleBar" />
                        </label>
                    </div>
                </div>

                <div v-show="settingsTabProxy === 'region'" class="settings-pane">
                    <div class="settings-section">
                        <h4>语言与区域</h4>
                        <div class="settings-grid">
                            <label>
                                <span>语言与区域</span>
                                <Select v-model="settingsDraft.locale" :options="localeOptions" option-label="label" option-value="value" class="w-full" />
                            </label>
                        </div>
                    </div>
                </div>

                <div v-show="settingsTabProxy === 'developer'" class="settings-pane">
                    <div class="settings-section">
                        <h4>开发者模式</h4>
                        <label class="developer-toggle">
                            <div>
                                <span>在应用内启用调试视图</span>
                                <small>打开后会显示最近的前后端日志，方便排查运行时问题。</small>
                            </div>
                            <ToggleSwitch v-model="settingsDraft.developerMode" />
                        </label>
                    </div>

                    <div v-if="settingsDraft.developerMode" class="settings-section">
                        <div class="developer-toolbar">
                            <div class="developer-summary">
                                <strong>最近日志 {{ developerLogCount }} 条</strong>
                                <span>{{ loadingLogs ? "正在刷新日志…" : "日志会持续写入内存缓冲与本地日志文件。" }}</span>
                            </div>
                            <div class="developer-actions">
                                <Button text icon="pi pi-refresh" label="刷新" @click="$emit('refresh-logs')" />
                                <Button text icon="pi pi-copy" label="复制" @click="$emit('copy-logs')" />
                                <Button text severity="danger" icon="pi pi-trash" label="清空" @click="$emit('clear-logs')" />
                            </div>
                        </div>

                        <p class="settings-note">使用 `-dev` 构建出的二进制时，按 <kbd>F12</kbd> 会打开原生 Web Inspector，用来框选元素和查看样式。</p>

                        <div class="settings-meta-grid">
                            <article>
                                <span>前后端日志</span><strong>{{ developerLogCount }}</strong>
                            </article>
                            <article>
                                <span>日志文件</span><strong>{{ logFilePath || "-" }}</strong>
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

                            <div v-if="!developerLogs.length" class="developer-log-empty">还没有捕获到日志。你可以先执行一次刷新、保存设置，或等待下一次自动行情同步。</div>
                        </div>
                    </div>

                    <div v-else class="settings-section">
                        <p class="settings-note">开启后可在应用里查看运行日志，不再依赖终端输出。</p>
                    </div>
                </div>
            </section>
        </div>

        <template #footer>
            <Button text label="取消" @click="visibleProxy = false" />
            <Button label="保存" :loading="saving" @click="$emit('save')" />
        </template>
    </Dialog>
</template>
