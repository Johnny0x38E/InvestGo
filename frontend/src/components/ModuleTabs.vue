<script setup lang="ts">
import { computed } from "vue";

import { appendClientLog } from "../devlog";
import { getModuleTabs } from "../constants";
import type { ModuleKey } from "../types";

defineProps<{
    activeModule: ModuleKey;
}>();

const emit = defineEmits<{
    (event: "switch", value: ModuleKey): void;
}>();

const moduleTabs = computed(() => getModuleTabs());

function switchTab(next: ModuleKey): void {
    appendClientLog("info", "tabs", `tab click -> ${next}`);
    emit("switch", next);
}
</script>

<template>
    <nav class="module-nav">
        <button v-for="tab in moduleTabs" :key="tab.key" class="module-pill" :class="{ active: activeModule === tab.key }" type="button" @click="switchTab(tab.key)">
            <i :class="tab.icon"></i>
            <span>{{ tab.label }}</span>
        </button>
    </nav>
</template>
