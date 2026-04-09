<script setup lang="ts">
import Button from "primevue/button";

import { formatDateTime } from "../format";
import appMark from "../assets/app-mark.svg";
import type { StatusTone } from "../types";

defineProps<{
    statusText: string;
    statusTone: StatusTone;
    generatedAt: string;
}>();

defineEmits<{
    (event: "open-settings"): void;
}>();
</script>

<template>
    <header class="window-bar">
        <div class="brand-block">
            <div class="brand-mark" aria-hidden="true">
                <img :src="appMark" alt="" />
            </div>
            <div class="brand-copy">
                <strong>Invest Monitor</strong>
                <span>Daily Market Desk</span>
            </div>
        </div>
        <div class="window-tools">
            <div class="window-status" :data-tone="statusTone">
                <span class="window-status-text">{{ statusText }}</span>
                <span class="window-status-time">最近渲染 {{ formatDateTime(generatedAt) }}</span>
            </div>
            <Button text icon="pi pi-cog" label="设置" @click="$emit('open-settings')" />
        </div>
    </header>
</template>
