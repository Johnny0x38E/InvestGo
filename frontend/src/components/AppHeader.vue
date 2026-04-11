<script setup lang="ts">
import { reactive } from "vue";

import { formatDateTime } from "../format";
import { isWindowMaximised, maximiseWindow, restoreWindow, startWindowDrag } from "../wails-runtime";
import type { StatusTone } from "../types";

defineProps<{
    statusText: string;
    statusTone: StatusTone;
    generatedAt: string;
}>();

defineEmits<{
    (event: "open-settings"): void;
}>();

const dragState = reactive({
    active: false,
    dragging: false,
    startX: 0,
    startY: 0,
});
let dragResetTimer = 0;

function isInteractiveTarget(target: EventTarget | null): boolean {
    if (!(target instanceof Element)) {
        return false;
    }

    return Boolean(target.closest("button, a, input, textarea, select, [role='button'], [data-window-dblclick-ignore='true']"));
}

function resetDragState(): void {
    if (dragResetTimer !== 0) {
        window.clearTimeout(dragResetTimer);
        dragResetTimer = 0;
    }
    dragState.active = false;
    dragState.dragging = false;
}

function scheduleDragStateReset(): void {
    if (dragResetTimer !== 0) {
        window.clearTimeout(dragResetTimer);
    }
    dragResetTimer = window.setTimeout(() => {
        resetDragState();
    }, 800);
}

function handleBarMouseDown(event: MouseEvent): void {
    if (isInteractiveTarget(event.target)) {
        return;
    }

    dragState.active = true;
    dragState.dragging = false;
    dragState.startX = event.clientX;
    dragState.startY = event.clientY;
}

function handleBarMouseMove(event: MouseEvent): void {
    if (!dragState.active || dragState.dragging || isInteractiveTarget(event.target)) {
        return;
    }

    const movedX = Math.abs(event.clientX - dragState.startX);
    const movedY = Math.abs(event.clientY - dragState.startY);
    if (movedX < 4 && movedY < 4) {
        return;
    }

    dragState.dragging = true;
    startWindowDrag();
    scheduleDragStateReset();
}

function handleBarMouseUp(): void {
    resetDragState();
}

function handleBarMouseLeave(): void {
    resetDragState();
}

// 双击标题栏时切换窗口缩放状态，避免依赖 macOS 对自定义拖拽区的默认行为。
async function handleBarDoubleClick(event: MouseEvent): Promise<void> {
    if (dragState.dragging || isInteractiveTarget(event.target)) {
        return;
    }

    event.preventDefault();
    const maximised = await isWindowMaximised();
    if (maximised) {
        restoreWindow();
        return;
    }

    maximiseWindow();
}
</script>

<template>
    <header class="window-bar" @mousedown="handleBarMouseDown" @mousemove="handleBarMouseMove" @mouseup="handleBarMouseUp" @mouseleave="handleBarMouseLeave" @dblclick="handleBarDoubleClick">
        <div class="window-bar-spacer" aria-hidden="true"></div>
        <div class="window-tools">
            <div class="window-status" :data-tone="statusTone">
                <span class="window-status-text">{{ statusText }}</span>
                <span class="window-status-time">最近刷新 {{ formatDateTime(generatedAt) }}</span>
            </div>
            <button type="button" class="window-settings-button" aria-label="设置" @click="$emit('open-settings')">
                <span class="pi pi-cog" aria-hidden="true"></span>
            </button>
        </div>
    </header>
</template>
