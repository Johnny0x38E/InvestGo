<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Checkbox from "primevue/checkbox";
import Dialog from "primevue/dialog";
import InputNumber from "primevue/inputnumber";
import InputText from "primevue/inputtext";
import Select from "primevue/select";

import { alertConditionOptions } from "../../constants";
import type { AlertFormModel, OptionItem } from "../../types";

const props = defineProps<{
    visible: boolean;
    form: AlertFormModel;
    itemOptions: OptionItem<string>[];
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
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="form.id ? '编辑' : '添加'" :style="{ width: '680px' }" class="desk-dialog">
        <div class="form-grid">
            <label class="full-span">
                <span>名称</span>
                <InputText v-model.trim="form.name" />
            </label>
            <label>
                <span>标的</span>
                <Select v-model="form.itemId" :options="itemOptions" option-label="label" option-value="value" />
            </label>
            <label>
                <span>规则</span>
                <Select v-model="form.condition" :options="alertConditionOptions" option-label="label" option-value="value" />
            </label>
            <label>
                <span>阈值</span>
                <InputNumber v-model="form.threshold" :min="0.01" :step="0.01" :min-fraction-digits="2" :max-fraction-digits="2" fluid />
            </label>
            <label class="checkbox-field">
                <span>启用</span>
                <div class="checkbox-wrap">
                    <Checkbox v-model="form.enabled" binary />
                </div>
            </label>
        </div>
        <template #footer>
            <Button text label="取消" @click="visibleProxy = false" />
            <Button label="保存" :loading="saving" @click="$emit('save')" />
        </template>
    </Dialog>
</template>
