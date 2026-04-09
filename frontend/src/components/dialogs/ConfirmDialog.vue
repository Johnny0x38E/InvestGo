<script setup lang="ts">
import { computed } from "vue";
import Button from "primevue/button";
import Dialog from "primevue/dialog";

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
</script>

<template>
    <Dialog v-model:visible="visibleProxy" modal :closable="false" :header="title" :style="{ width: '460px' }" class="desk-dialog confirm-dialog">
        <p class="confirm-copy">{{ message }}</p>
        <template #footer>
            <Button text label="取消" @click="visibleProxy = false" />
            <Button severity="danger" :label="confirmLabel || '删除'" :loading="loading" @click="$emit('confirm')" />
        </template>
    </Dialog>
</template>
