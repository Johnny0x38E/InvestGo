<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";

import { formatDateTime } from "../format";
import { useI18n } from "../i18n";
import { closeWindow, isWindowMaximised, maximiseWindow, minimiseWindow, restoreWindow, startWindowDrag, toggleMaximiseWindow } from "../wails-runtime";
import type { StatusTone } from "../types";

defineProps<{
    statusText: string;
    statusTone: StatusTone;
    generatedAt: string;
    showWindowControls: boolean;
}>();

const { t } = useI18n();
const maximised = ref(false);

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

// Toggle window maximize/restore on title bar double-click, avoiding reliance on macOS default behavior for custom drag regions.
async function handleBarDoubleClick(event: MouseEvent): Promise<void> {
    if (dragState.dragging || isInteractiveTarget(event.target)) {
        return;
    }

    event.preventDefault();
    const maximised = await isWindowMaximised();
    if (maximised) {
        await restoreWindow();
        await refreshMaximisedState();
        return;
    }

    await maximiseWindow();
    await refreshMaximisedState();
}

async function refreshMaximisedState(): Promise<void> {
    maximised.value = await isWindowMaximised();
}

async function handleMinimise(): Promise<void> {
    await minimiseWindow();
}

async function handleToggleMaximise(): Promise<void> {
    await toggleMaximiseWindow();
    await refreshMaximisedState();
}

async function handleClose(): Promise<void> {
    await closeWindow();
}

onMounted(() => {
    void refreshMaximisedState();
});
</script>

<template>
    <header class="window-bar" @mousedown="handleBarMouseDown" @mousemove="handleBarMouseMove" @mouseup="handleBarMouseUp" @mouseleave="handleBarMouseLeave" @dblclick="handleBarDoubleClick">
        <div class="window-bar-spacer" aria-hidden="true"></div>
        <div class="window-tools">
            <div class="window-status" :data-tone="statusTone">
                <span class="window-status-text">{{ statusText }}</span>
                <span class="window-status-separator">·</span>
                <span class="window-status-time">{{ t("app.recentRefresh", { time: formatDateTime(generatedAt) }) }}</span>
            </div>
            <div v-if="showWindowControls" class="window-controls" data-window-dblclick-ignore="true">
                <button type="button" class="window-control-button" :aria-label="t('windowControls.minimise')" @click="handleMinimise">
                    <i class="pi pi-minus" aria-hidden="true"></i>
                </button>
                <button type="button" class="window-control-button" :aria-label="maximised ? t('windowControls.restore') : t('windowControls.maximise')" @click="handleToggleMaximise">
                    <i class="pi" :class="maximised ? 'pi-window-minimize' : 'pi-window-maximize'" aria-hidden="true"></i>
                </button>
                <button type="button" class="window-control-button window-control-close" :aria-label="t('windowControls.close')" @click="handleClose">
                    <i class="pi pi-times" aria-hidden="true"></i>
                </button>
            </div>
        </div>
    </header>
</template>

<style scoped>
.window-bar {
    min-height: 40px;
    padding: 4px 10px 2px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    cursor: default;
    user-select: none;
    -webkit-user-select: none;
}

.window-bar-spacer {
    flex: 1 1 auto;
    min-width: 0;
}

.window-tools {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
}

.window-status {
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 2px 4px;
    border: none;
    background: none;
}

.window-status-text,
.window-status-time {
    max-width: min(42vw, 520px);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.window-status-text {
    font-size: 10px;
    color: var(--muted);
}

.window-status-time {
    font-size: 10px;
    color: var(--muted);
}

.window-status-separator {
    font-size: 10px;
    color: var(--muted);
    opacity: 0.7;
}

.window-status[data-tone="error"] .window-status-text {
    color: var(--fall);
}

.window-status[data-tone="warn"] .window-status-text {
    color: var(--warn);
}

.window-status[data-tone="success"] .window-status-text {
    color: var(--accent);
}

.window-controls {
    height: 32px;
    display: inline-flex;
    align-items: stretch;
    gap: 1px;
    border-radius: 8px;
    overflow: hidden;
}

.window-control-button {
    width: 42px;
    height: 32px;
    padding: 0;
    border: none;
    border-radius: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--muted);
    background: transparent;
    cursor: default;
    transition:
        color 120ms ease,
        background 120ms ease;
}

.window-control-button i {
    font-size: 11px;
}

.window-control-button:hover {
    color: var(--ink);
    background: var(--panel-soft);
}

.window-control-close:hover {
    color: #ffffff;
    background: #d93f3f;
}

@media (max-width: 880px) {
    .window-bar {
        padding-left: 12px;
    }

    .window-tools {
        align-items: flex-end;
        flex-direction: column;
    }

    .window-status {
        flex-wrap: wrap;
        justify-content: flex-end;
    }
}
</style>
