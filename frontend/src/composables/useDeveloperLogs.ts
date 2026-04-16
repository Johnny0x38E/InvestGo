import { computed, ref } from "vue";

import { api } from "../api";
import { appendClientLog, clearClientLogs, clientLogs } from "../devlog";
import { translate } from "../i18n";
import type { DeveloperLogEntry, DeveloperLogSnapshot, StatusTone } from "../types";

type StatusReporter = (message: string, tone: StatusTone) => void;

// Unified management of frontend and backend developer logs, preventing the root component from handling polling, merging, copying, and cleanup all at once.
export function useDeveloperLogs(setStatus: StatusReporter) {
    const backendLogs = ref<DeveloperLogEntry[]>([]);
    const logFilePath = ref("");
    const loadingLogs = ref(false);

    // Frontend and backend logs share a single reverse-chronological list for unified viewing on the settings page.
    const developerLogs = computed<DeveloperLogEntry[]>(() =>
        [...backendLogs.value, ...clientLogs.value].sort((left, right) => new Date(right.timestamp).getTime() - new Date(left.timestamp).getTime()).slice(0, 250),
    );

    // Silent mode is primarily for polling refresh; on failure it does not disrupt the current page state.
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

        // Use plain text format when copying for easy pasting into issues, IM, or terminal.
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
