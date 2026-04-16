<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Checkbox from "primevue/checkbox";
import Dialog from "primevue/dialog";
import InputNumber from "primevue/inputnumber";
import InputText from "primevue/inputtext";
import Select from "primevue/select";

import { getAlertConditionOptions } from "../../constants";
import { useI18n } from "../../i18n";
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

const { t } = useI18n();
const alertConditionOptions = computed(() => getAlertConditionOptions());
</script>

<template>
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="form.id ? t('dialogs.alert.editTitle') : t('dialogs.alert.addTitle')" :style="{ width: '680px' }" class="desk-dialog">
        <div class="form-grid">
            <label class="full-span">
                <span>{{ t("common.name") }}</span>
                <InputText v-model.trim="form.name" />
            </label>
            <label>
                <span>{{ t("watchlist.table.item") }}</span>
                <Select v-model="form.itemId" :options="itemOptions" option-label="label" option-value="value" />
            </label>
            <label>
                <span>{{ t("common.rule") }}</span>
                <Select v-model="form.condition" :options="alertConditionOptions" option-label="label" option-value="value" />
            </label>
            <label>
                <span>{{ t("common.threshold") }}</span>
                <InputNumber v-model="form.threshold" :min="0.01" :step="0.01" :min-fraction-digits="2" :max-fraction-digits="2" fluid />
            </label>
            <label class="checkbox-field">
                <span>{{ t("common.enabled") }}</span>
                <div class="checkbox-wrap">
                    <Checkbox v-model="form.enabled" binary />
                </div>
            </label>
        </div>
        <template #footer>
            <Button size="small" text :label="t('common.cancel')" @click="visibleProxy = false" />
            <Button size="small" :label="t('common.save')" :loading="saving" @click="$emit('save')" />
        </template>
    </Dialog>
</template>
