import { computed, ref } from "vue";

import { api } from "../api";
import { appendClientLog, clearClientLogs, clientLogs } from "../devlog";
import { translate } from "../i18n";
import type { DeveloperLogEntry, DeveloperLogSnapshot, StatusTone } from "../types";

type StatusReporter = (message: string, tone: StatusTone) => void;

// 统一管理前后端开发日志，避免根组件同时承担轮询、合并、复制和清理职责。
export function useDeveloperLogs(setStatus: StatusReporter) {
    const backendLogs = ref<DeveloperLogEntry[]>([]);
    const logFilePath = ref("");
    const loadingLogs = ref(false);

    // 前后端日志共用一个时间倒序列表，方便在设置页里统一查看。
    const developerLogs = computed<DeveloperLogEntry[]>(() =>
        [...backendLogs.value, ...clientLogs.value].sort((left, right) => new Date(right.timestamp).getTime() - new Date(left.timestamp).getTime()).slice(0, 250),
    );

    // 静默模式主要用于轮询刷新，失败时不打断当前页面状态。
    async function loadBackendLogs(silent = false): Promise<void> {
        if (loadingLogs.value) {
            return;
        }

        loadingLogs.value = true;
        try {
            const snapshot = await api<DeveloperLogSnapshot>("/api/logs?limit=160");
            backendLogs.value = snapshot.entries;
            logFilePath.value = snapshot.logFilePath;
        } catch (error) {
            if (!silent) {
                setStatus(error instanceof Error ? error.message : translate("developerLogs.loadFailed"), "error");
            }
        } finally {
            loadingLogs.value = false;
        }
    }

    async function clearDeveloperLogs(): Promise<void> {
        try {
            await api<{ ok: boolean }>("/api/logs", { method: "DELETE" });
            backendLogs.value = [];
            clearClientLogs();
            setStatus(translate("developerLogs.cleared"), "success");
        } catch (error) {
            setStatus(error instanceof Error ? error.message : translate("developerLogs.clearFailed"), "error");
        }
    }

    async function copyDeveloperLogs(): Promise<void> {
        if (!developerLogs.value.length) {
            setStatus(translate("developerLogs.nothingToCopy"), "warn");
            return;
        }

        // 复制时使用纯文本格式，便于直接贴到 issue、IM 或终端里。
        const payload = developerLogs.value.map((entry) => `[${entry.timestamp}] ${entry.level.toUpperCase()} ${entry.source}/${entry.scope} ${entry.message}`).join("\n\n");

        try {
            await navigator.clipboard.writeText(payload);
            setStatus(translate("developerLogs.copied"), "success");
        } catch (error) {
            appendClientLog("error", "clipboard", error instanceof Error ? error.message : "Failed to copy logs.");
            setStatus(translate("developerLogs.copyFailed"), "error");
        }
    }

    return {
        developerLogs,
        loadingLogs,
        logFilePath,
        loadBackendLogs,
        clearDeveloperLogs,
        copyDeveloperLogs,
    };
}
