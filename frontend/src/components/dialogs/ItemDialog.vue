<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Dialog from "primevue/dialog";
import InputNumber from "primevue/inputnumber";
import InputText from "primevue/inputtext";
import Select from "primevue/select";
import Textarea from "primevue/textarea";

import { currencyOptions, marketOptions } from "../../constants";
import type { ItemFormModel } from "../../types";

const props = defineProps<{
    visible: boolean;
    form: ItemFormModel;
    saving: boolean;
}>();

const emit = defineEmits<{
    (event: "update:visible", value: boolean): void;
    (event: "save"): void;
}>();

const visibleProxy = computed({
    get: () => props.visible,
    set: (value: boolean) => emit("update:visible", value),
});
</script>

<template>
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="form.id ? '编辑' : '添加'" :style="{ width: '760px' }" class="desk-dialog">
        <div class="form-grid">
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
            <label>
                <span>持仓数量</span>
                <InputNumber v-model="form.quantity" :min="0" :step="0.01" fluid />
            </label>
            <label>
                <span>成本价</span>
                <InputNumber v-model="form.costPrice" :min="0" :step="0.01" fluid />
            </label>
            <label>
                <span>手动兜底价</span>
                <InputNumber v-model="form.currentPrice" :min="0" :step="0.01" fluid />
            </label>
            <label>
                <span>标签</span>
                <InputText v-model.trim="form.tagsText" />
            </label>
            <label class="full-span">
                <span>策略备注</span>
                <Textarea v-model="form.thesis" auto-resize rows="5" />
            </label>
        </div>
        <template #footer>
            <Button text label="取消" @click="visibleProxy = false" />
            <Button label="保存" :loading="saving" @click="$emit('save')" />
        </template>
    </Dialog>
</template>
