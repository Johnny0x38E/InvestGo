<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Button from "primevue/button";
import Dialog from "primevue/dialog";
import InputNumber from "primevue/inputnumber";
import InputText from "primevue/inputtext";
import Select from "primevue/select";
import Textarea from "primevue/textarea";

import { currencyOptions, getMarketOptions } from "../../constants";
import { formatMoney, formatPercent, formatUnitPrice, resolvedLocale } from "../../format";
import { useI18n } from "../../i18n";
import type { DCAEntryRow, ItemFormModel } from "../../types";

const props = defineProps<{
    visible: boolean;
    form: ItemFormModel;
    saving: boolean;
    initialTab?: "basic" | "dca";
}>();

const emit = defineEmits<{
    (event: "update:visible", value: boolean): void;
    (event: "save"): void;
}>();

const visibleProxy = computed({
    get: () => props.visible,
    set: (value: boolean) => emit("update:visible", value),
});

type TabKey = "basic" | "dca";
const activeTab = ref<TabKey>("basic");
const { t } = useI18n();
const marketOptions = computed(() => getMarketOptions());

watch(
    () => props.visible,
    (v) => {
        if (v) activeTab.value = props.initialTab ?? "basic";
    },
);

// DCA summary

const dcaSummary = computed(() => {
    const valid = props.form.dcaEntries.filter((e) => (e.amount ?? 0) > 0 && (e.shares ?? 0) > 0);
    const totalAmount = valid.reduce((s, e) => s + (e.amount ?? 0), 0);
    const totalShares = valid.reduce((s, e) => s + (e.shares ?? 0), 0);
    const totalFees = valid.reduce((s, e) => s + (e.fee ?? 0), 0);
    // Match backend sanitiseItem behavior by preferring the manually entered buy price.
    let totalEffectiveCost = 0;
    for (const e of valid) {
        const price = e.price ?? 0;
        const fee = e.fee ?? 0;
        const amount = e.amount ?? 0;
        const shares = e.shares ?? 0;
        if (price > 0) {
            totalEffectiveCost += price * shares;
        } else {
            totalEffectiveCost += Math.max(amount - fee, 0);
        }
    }
    const avgCost = totalShares > 0 ? totalEffectiveCost / totalShares : 0;
    const curPrice = props.form.currentPrice ?? 0;
    const currentValue = totalShares * curPrice;
    const pnl = curPrice > 0 ? currentValue - totalEffectiveCost : null;
    const pnlPct = totalEffectiveCost > 0 && pnl !== null ? (pnl / totalEffectiveCost) * 100 : null;
    return {
        count: valid.length,
        totalAmount,
        totalShares,
        totalFees,
        avgCost,
        currentValue,
        pnl,
        pnlPct,
        hasCurPrice: curPrice > 0,
    };
});

// When valid DCA records exist, derive quantity and cost price from those records and lock manual input.
const hasDCA = computed(() => props.form.dcaEntries.some((e) => (e.amount ?? 0) > 0 && (e.shares ?? 0) > 0));

// DCA entry CRUD helpers

function addEntry(): void {
    props.form.dcaEntries.push({
        id: `tmp-${Date.now()}`,
        date: "",
        amount: null,
        shares: null,
        price: null,
        fee: null,
        note: "",
    } as DCAEntryRow);
}

function removeEntry(index: number): void {
    props.form.dcaEntries.splice(index, 1);
}

// Profit and loss tone
function pnlTone(value: number | null): string {
    if (value === null) return "";
    return value > 0 ? "tone-rise" : value < 0 ? "tone-fall" : "";
}
</script>

<template>
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="form.id ? t('dialogs.item.editTitle') : t('dialogs.item.addTitle')" :style="{ width: '1060px' }" class="desk-dialog">
        <!-- Tab switcher -->
        <div class="item-dialog-tabs">
            <button class="item-dialog-tab" :class="{ active: activeTab === 'basic' }" type="button" @click="activeTab = 'basic'">{{ t("dialogs.item.tabs.basic") }}</button>
            <button class="item-dialog-tab" :class="{ active: activeTab === 'dca' }" type="button" @click="activeTab = 'dca'">
                {{ t("dialogs.item.tabs.dca") }}
                <span v-if="form.dcaEntries.length" class="dca-count-badge">{{ form.dcaEntries.length }}</span>
            </button>
        </div>

        <!-- Tab 1: Basic information -->
        <div v-if="activeTab === 'basic'" class="form-grid">
            <label>
                <span>{{ t("dialogs.item.labels.symbol") }}</span>
                <InputText v-model.trim="form.symbol" />
            </label>
            <label>
                <span>{{ t("dialogs.item.labels.itemName") }}</span>
                <InputText v-model.trim="form.name" />
            </label>
            <label>
                <span>{{ t("dialogs.item.labels.market") }}</span>
                <Select v-model="form.market" :options="marketOptions" option-label="label" option-value="value" />
            </label>
            <label>
                <span>{{ t("dialogs.item.labels.currency") }}</span>
                <Select v-model="form.currency" :options="currencyOptions" option-label="label" option-value="value" />
            </label>

            <!-- Quantity is read-only when DCA records exist; otherwise it remains manually editable. -->
            <label v-if="!hasDCA">
                <span>{{ t("dialogs.item.labels.quantity") }}</span>
                <InputNumber v-model="form.quantity" :min="0" :step="0.001" :max-fraction-digits="3" fluid />
            </label>
            <div v-else class="dca-derived-field">
                <span>{{ t("dialogs.item.labels.quantity") }}</span>
                <div class="dca-derived-value">{{ t("dialogs.item.derived.shares", { value: dcaSummary.totalShares.toLocaleString(resolvedLocale(), { maximumFractionDigits: 4 }) }) }}</div>
            </div>

            <!-- Cost price is read-only when DCA records exist; otherwise it remains manually editable. -->
            <label v-if="!hasDCA">
                <span>{{ t("dialogs.item.labels.costPrice") }}</span>
                <InputNumber v-model="form.costPrice" :min="0" :step="0.001" :max-fraction-digits="3" fluid />
            </label>
            <div v-else class="dca-derived-field">
                <span>{{ t("dialogs.item.labels.costPrice") }}</span>
                <div class="dca-derived-value">{{ t("dialogs.item.derived.weightedAverage", { price: formatUnitPrice(dcaSummary.avgCost, form.currency) }) }}</div>
            </div>

            <!-- Show a hint when DCA records are driving the derived fields. -->
            <p v-if="hasDCA" class="dca-derived-hint">
                <i class="pi pi-info-circle" style="margin-right: 5px" />
                {{ t("dialogs.item.derived.hint") }}
            </p>

            <label>
                <span>{{ t("dialogs.item.labels.tags") }}</span>
                <InputText v-model.trim="form.tagsText" />
            </label>
            <label class="full-span">
                <span>{{ t("dialogs.item.labels.thesis") }}</span>
                <Textarea v-model="form.thesis" auto-resize rows="5" />
            </label>
        </div>

        <!-- Tab 2: DCA records -->
        <div v-else class="dca-panel">
            <!-- Entry table -->
            <div class="dca-table">
                <!-- Header row -->
                <div class="dca-table-head">
                    <span class="dca-col-label">{{ t("dialogs.item.labels.date") }}</span>
                    <span class="dca-col-label">{{ t("dialogs.item.labels.investedAmount") }}</span>
                    <span class="dca-col-label">{{ t("dialogs.item.labels.boughtShares") }}</span>
                    <span class="dca-col-label">{{ t("dialogs.item.labels.buyPrice") }}</span>
                    <span class="dca-col-label">{{ t("dialogs.item.labels.fee") }}</span>
                    <span class="dca-col-label">{{ t("dialogs.item.labels.note") }}</span>
                    <span />
                </div>

                <!-- Empty state -->
                <div v-if="form.dcaEntries.length === 0" class="dca-empty-hint">{{ t("dialogs.item.empty") }}</div>

                <!-- Entry rows -->
                <div v-for="(entry, idx) in form.dcaEntries" :key="entry.id" class="dca-entry-row">
                    <!-- Date -->
                    <input v-model="entry.date" type="date" class="dca-date-input" />

                    <!-- Invested amount -->
                    <InputNumber v-model="entry.amount" :min="0" :step="100" :max-fraction-digits="3" fluid :placeholder="t('dialogs.item.placeholders.amount')" />

                    <!-- Shares purchased -->
                    <InputNumber v-model="entry.shares" :min="0" :step="0.001" :max-fraction-digits="4" fluid :placeholder="t('dialogs.item.placeholders.shares')" />

                    <!-- Buy price, when entered manually -->
                    <InputNumber v-model="entry.price" :min="0" :step="0.001" :max-fraction-digits="3" fluid :placeholder="t('dialogs.item.placeholders.buyPrice')" />

                    <!-- Fee or commission -->
                    <InputNumber v-model="entry.fee" :min="0" :step="1" :max-fraction-digits="3" fluid :placeholder="t('dialogs.item.placeholders.fee')" />

                    <!-- Note -->
                    <InputText v-model="entry.note" :placeholder="t('dialogs.item.placeholders.note')" style="font-size: 12px" />

                    <!-- Delete -->
                    <Button text severity="danger" icon="pi pi-trash" size="small" :aria-label="t('common.delete')" @click="removeEntry(idx)" />
                </div>
            </div>

            <!-- Add-entry action -->
            <div class="dca-add-row">
                <Button text icon="pi pi-plus" :label="t('common.add')" size="small" @click="addEntry" />
            </div>

            <!-- Summary bar, shown only when at least one valid record exists -->
            <div v-if="dcaSummary.count > 0" class="dca-summary-bar">
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">{{ t("dialogs.item.labels.dcaPeriods") }}</span>
                    <span class="dca-summary-value">{{ t("dialogs.dcaDetail.summary.countValue", { count: dcaSummary.count }) }}</span>
                </div>
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">{{ t("dialogs.item.labels.totalInvested") }}</span>
                    <span class="dca-summary-value">{{ formatMoney(dcaSummary.totalAmount) }}</span>
                </div>
                <div v-if="dcaSummary.totalFees > 0" class="dca-summary-cell">
                    <span class="dca-summary-label">{{ t("dialogs.item.labels.totalFees") }}</span>
                    <span class="dca-summary-value">{{ formatMoney(dcaSummary.totalFees) }}</span>
                </div>
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">{{ t("dialogs.item.labels.totalShares") }}</span>
                    <span class="dca-summary-value">
                        {{ dcaSummary.totalShares.toLocaleString(resolvedLocale(), { maximumFractionDigits: 4 }) }}
                    </span>
                </div>
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">{{ t("dialogs.item.labels.weightedAvgPrice") }}</span>
                    <span class="dca-summary-value">{{ formatUnitPrice(dcaSummary.avgCost, form.currency) }}</span>
                </div>
                <template v-if="dcaSummary.hasCurPrice">
                    <div class="dca-summary-cell">
                        <span class="dca-summary-label">{{ t("dialogs.item.labels.currentValue") }}</span>
                        <span class="dca-summary-value">{{ formatUnitPrice(dcaSummary.currentValue, form.currency) }}</span>
                    </div>
                    <div class="dca-summary-cell">
                        <span class="dca-summary-label">{{ t("dialogs.item.labels.unrealizedPnL") }}</span>
                        <span class="dca-summary-value" :class="pnlTone(dcaSummary.pnl)">
                            {{ formatMoney(dcaSummary.pnl ?? 0, true) }}
                            <span style="font-weight: 400; font-size: 11px; margin-left: 4px">
                                {{ dcaSummary.pnlPct !== null ? formatPercent(dcaSummary.pnlPct) : "" }}
                            </span>
                        </span>
                    </div>
                </template>
            </div>
        </div>

        <!-- Footer actions -->
        <template #footer>
            <Button size="small" text :label="t('common.cancel')" @click="visibleProxy = false" />
            <Button size="small" :label="t('common.save')" :loading="saving" @click="$emit('save')" />
        </template>
    </Dialog>
</template>
