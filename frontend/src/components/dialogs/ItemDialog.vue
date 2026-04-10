<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Button from "primevue/button";
import Dialog from "primevue/dialog";
import InputNumber from "primevue/inputnumber";
import InputText from "primevue/inputtext";
import Select from "primevue/select";
import Textarea from "primevue/textarea";

import { currencyOptions, marketOptions } from "../../constants";
import { formatMoney, formatPercent, formatUnitPrice } from "../../format";
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

watch(
    () => props.visible,
    (v) => {
        if (v) activeTab.value = props.initialTab ?? "basic";
    },
);

// ── DCA 汇总 ────────────────────────────────────────────────────────────────

const dcaSummary = computed(() => {
    const valid = props.form.dcaEntries.filter((e) => (e.amount ?? 0) > 0 && (e.shares ?? 0) > 0);
    const totalAmount = valid.reduce((s, e) => s + (e.amount ?? 0), 0);
    const totalShares = valid.reduce((s, e) => s + (e.shares ?? 0), 0);
    const totalFees = valid.reduce((s, e) => s + (e.fee ?? 0), 0);
    // 与后端 sanitiseItem 保持一致：优先使用手动买入价
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

// 有效定投记录存在时，持仓数量/成本价由记录推算，锁定手动输入
const hasDCA = computed(() => props.form.dcaEntries.some((e) => (e.amount ?? 0) > 0 && (e.shares ?? 0) > 0));

// ── 条目 CRUD ────────────────────────────────────────────────────────────────

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

// 涨跌色调
function pnlTone(value: number | null): string {
    if (value === null) return "";
    return value > 0 ? "tone-rise" : value < 0 ? "tone-fall" : "";
}
</script>

<template>
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="form.id ? '编辑标的' : '添加标的'" :style="{ width: '1060px' }" class="desk-dialog">
        <!-- ── Tab 切换栏 ─────────────────────────────────────────────────── -->
        <div class="item-dialog-tabs">
            <button class="item-dialog-tab" :class="{ active: activeTab === 'basic' }" type="button" @click="activeTab = 'basic'">基础信息</button>
            <button class="item-dialog-tab" :class="{ active: activeTab === 'dca' }" type="button" @click="activeTab = 'dca'">
                定投记录
                <span v-if="form.dcaEntries.length" class="dca-count-badge">{{ form.dcaEntries.length }}</span>
            </button>
        </div>

        <!-- ── Tab 1：基础信息 ──────────────────────────────────────────────-->
        <div v-if="activeTab === 'basic'" class="form-grid">
            <label>
                <span>股票代码</span>
                <InputText v-model.trim="form.symbol" />
            </label>
            <label>
                <span>标的名称</span>
                <InputText v-model.trim="form.name" />
            </label>
            <label>
                <span>市场</span>
                <Select v-model="form.market" :options="marketOptions" option-label="label" option-value="value" />
            </label>
            <label>
                <span>币种</span>
                <Select v-model="form.currency" :options="currencyOptions" option-label="label" option-value="value" />
            </label>

            <!-- 持仓数量：有定投时只读，无定投时手动填 -->
            <label v-if="!hasDCA">
                <span>持仓数量</span>
                <InputNumber v-model="form.quantity" :min="0" :step="0.001" :max-fraction-digits="3" fluid />
            </label>
            <div v-else class="dca-derived-field">
                <span>持仓数量</span>
                <div class="dca-derived-value">{{ dcaSummary.totalShares.toLocaleString("zh-CN", { maximumFractionDigits: 4 }) }} 份（定投计算）</div>
            </div>

            <!-- 成本价：有定投时只读，无定投时手动填 -->
            <label v-if="!hasDCA">
                <span>成本价</span>
                <InputNumber v-model="form.costPrice" :min="0" :step="0.001" :max-fraction-digits="3" fluid />
            </label>
            <div v-else class="dca-derived-field">
                <span>成本价</span>
                <div class="dca-derived-value">{{ formatUnitPrice(dcaSummary.avgCost, form.currency) }}（加权均价）</div>
            </div>

            <!-- 有定投记录时给出提示 -->
            <p v-if="hasDCA" class="dca-derived-hint">
                <i class="pi pi-info-circle" style="margin-right: 5px" />
                已存在定投记录，「持仓数量」与「成本价」由定投数据计算得出。
            </p>

            <label>
                <span>标签</span>
                <InputText v-model.trim="form.tagsText" />
            </label>
            <label class="full-span">
                <span>策略备注</span>
                <Textarea v-model="form.thesis" auto-resize rows="5" />
            </label>
        </div>

        <!-- ── Tab 2：定投记录 ────────────────────────────────────────────── -->
        <div v-else class="dca-panel">
            <!-- 条目表格 -->
            <div class="dca-table">
                <!-- 表头 -->
                <div class="dca-table-head">
                    <span class="dca-col-label">日期</span>
                    <span class="dca-col-label">投入金额</span>
                    <span class="dca-col-label">买入股/份</span>
                    <span class="dca-col-label">买入价</span>
                    <span class="dca-col-label">手续费/佣金</span>
                    <span class="dca-col-label">备注</span>
                    <span />
                </div>

                <!-- 空状态 -->
                <div v-if="form.dcaEntries.length === 0" class="dca-empty-hint">还没有定投记录。点击「添加一笔」开始录入。</div>

                <!-- 条目行 -->
                <div v-for="(entry, idx) in form.dcaEntries" :key="entry.id" class="dca-entry-row">
                    <!-- 日期 -->
                    <input v-model="entry.date" type="date" class="dca-date-input" />

                    <!-- 投入金额 -->
                    <InputNumber v-model="entry.amount" :min="0" :step="100" :max-fraction-digits="3" fluid placeholder="金额" />

                    <!-- 买入股/份 -->
                    <InputNumber v-model="entry.shares" :min="0" :step="0.001" :max-fraction-digits="4" fluid placeholder="股/份" />

                    <!-- 买入价（手动录入） -->
                    <InputNumber v-model="entry.price" :min="0" :step="0.001" :max-fraction-digits="3" fluid placeholder="买入价" />

                    <!-- 手续费/佣金 -->
                    <InputNumber v-model="entry.fee" :min="0" :step="1" :max-fraction-digits="3" fluid placeholder="手续费/佣金" />

                    <!-- 备注 -->
                    <InputText v-model="entry.note" placeholder="备注（可选）" style="font-size: 12px" />

                    <!-- 删除 -->
                    <Button text severity="danger" icon="pi pi-trash" size="small" aria-label="删除" @click="removeEntry(idx)" />
                </div>
            </div>

            <!-- 添加按钮 -->
            <div class="dca-add-row">
                <Button text icon="pi pi-plus" label="添加一笔" size="small" @click="addEntry" />
            </div>

            <!-- 汇总栏（至少有一条有效记录才展示） -->
            <div v-if="dcaSummary.count > 0" class="dca-summary-bar">
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">定投期数</span>
                    <span class="dca-summary-value">{{ dcaSummary.count }} 笔</span>
                </div>
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">总投入</span>
                    <span class="dca-summary-value">{{ formatMoney(dcaSummary.totalAmount) }}</span>
                </div>
                <div v-if="dcaSummary.totalFees > 0" class="dca-summary-cell">
                    <span class="dca-summary-label">总手续费/佣金</span>
                    <span class="dca-summary-value">{{ formatMoney(dcaSummary.totalFees) }}</span>
                </div>
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">累计份额</span>
                    <span class="dca-summary-value">
                        {{ dcaSummary.totalShares.toLocaleString("zh-CN", { maximumFractionDigits: 4 }) }}
                    </span>
                </div>
                <div class="dca-summary-cell">
                    <span class="dca-summary-label">加权均价</span>
                    <span class="dca-summary-value">{{ formatUnitPrice(dcaSummary.avgCost, form.currency) }}</span>
                </div>
                <template v-if="dcaSummary.hasCurPrice">
                    <div class="dca-summary-cell">
                        <span class="dca-summary-label">当前资产</span>
                        <span class="dca-summary-value">{{ formatUnitPrice(dcaSummary.currentValue, form.currency) }}</span>
                    </div>
                    <div class="dca-summary-cell">
                        <span class="dca-summary-label">浮动盈亏</span>
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

        <!-- ── 底部操作按钮 ────────────────────────────────────────────────── -->
        <template #footer>
            <Button text label="取消" @click="visibleProxy = false" />
            <Button label="保存" :loading="saving" @click="$emit('save')" />
        </template>
    </Dialog>
</template>
