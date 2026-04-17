<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Dialog from "primevue/dialog";
import { useI18n } from "../../i18n";

const props = defineProps<{
    visible: boolean;
    title: string;
    message: string;
    confirmLabel?: string;
    loading?: boolean;
}>();

const emit = defineEmits<{
    (event: "update:visible", value: boolean): void;
    (event: "confirm"): void;
}>();

const visibleProxy = computed({
    get: () => props.visible,
    set: (value: boolean) => emit("update:visible", value),
});

const { t } = useI18n();
</script>

<template>
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="title" :style="{ width: '460px' }" class="desk-dialog confirm-dialog">
        <p class="confirm-copy">{{ message }}</p>
        <template #footer>
            <Button size="small" text :label="t('common.cancel')" @click="visibleProxy = false" />
            <Button size="small" severity="danger" :label="confirmLabel || t('common.delete')" :loading="loading" @click="$emit('confirm')" />
        </template>
    </Dialog>
</template>

<style scoped>
.confirm-copy {
    margin: 0;
    color: var(--muted);
    line-height: 1.7;
}
</style>
