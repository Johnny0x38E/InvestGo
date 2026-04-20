import { ref } from "vue";

import type { DeveloperLogEntry, DeveloperLogLevel, DeveloperLogSource } from "./types";

const maxClientLogs = 200;
const frontendSource: DeveloperLogSource = "frontend";
const clientLogsState = ref<DeveloperLogEntry[]>([]);

let installed = false;
let sequence = 0;

const mirrorQueue: Array<{ source: string; scope: string; level: DeveloperLogLevel; message: string }> = [];
let mirrorFlushTimer = 0;

export const clientLogs = clientLogsState;

export function installClientLogCapture(): void {
    if (installed) {
        return;
    }
    installed = true;

    const originalConsole = {
        debug: console.debug.bind(console),
        info: console.info.bind(console),
        log: console.log.bind(console),
        warn: console.warn.bind(console),
        error: console.error.bind(console),
    };

    console.debug = (...args: unknown[]) => {
        pushClientLog("debug", "console", args);
        originalConsole.debug(...args);
    };
    console.info = (...args: unknown[]) => {
        pushClientLog("info", "console", args);
        originalConsole.info(...args);
    };
    console.log = (...args: unknown[]) => {
        pushClientLog("info", "console", args);
        originalConsole.log(...args);
    };
    console.warn = (...args: unknown[]) => {
        pushClientLog("warn", "console", args);
        originalConsole.warn(...args);
    };
    console.error = (...args: unknown[]) => {
        pushClientLog("error", "console", args);
        originalConsole.error(...args);
    };

    window.addEventListener("error", (event) => {
        const errorText = event.error instanceof Error ? `${event.message}\n${event.error.stack ?? ""}`.trim() : event.message;
        appendClientLog("error", "window", errorText);
    });

    window.addEventListener("unhandledrejection", (event) => {
        appendClientLog("error", "promise", formatValue(event.reason));
    });

    window.addEventListener("beforeunload", () => {
        window.clearTimeout(mirrorFlushTimer);
        flushMirrorQueue();
    });
}

export function appendClientLog(level: DeveloperLogLevel, scope: string, message: string, source: DeveloperLogSource = frontendSource): void {
    const clean = redactSensitiveText(message).trim();
    if (!clean) {
        return;
    }

    const next: DeveloperLogEntry = {
        id: `client-${++sequence}`,
        source,
        scope,
        level,
        message: clean,
        timestamp: new Date().toISOString(),
    };

    const merged = [next, ...clientLogsState.value];
    clientLogsState.value = merged.slice(0, maxClientLogs);
    mirrorClientLog(next);
}

export function clearClientLogs(): void {
    clientLogsState.value = [];
}

function pushClientLog(level: DeveloperLogLevel, scope: string, args: unknown[]): void {
    appendClientLog(level, scope, args.map((value) => formatValue(value)).join(" "));
}

function mirrorClientLog(entry: DeveloperLogEntry): void {
    mirrorQueue.push({
        source: entry.source,
        scope: entry.scope,
        level: entry.level,
        message: entry.message,
    });
    window.clearTimeout(mirrorFlushTimer);
    mirrorFlushTimer = window.setTimeout(flushMirrorQueue, 1000);
}

function flushMirrorQueue(): void {
    if (mirrorQueue.length === 0) return;
    const batch = mirrorQueue.splice(0, mirrorQueue.length);
    // Send each entry individually (backend accepts one at a time)
    // Use keepalive for reliability on page unload
    for (const entry of batch) {
        void fetch("/api/client-logs", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(entry),
            keepalive: true,
        }).catch(() => {});
    }
}

function formatValue(value: unknown): string {
    if (value instanceof Error) {
        return `${value.name}: ${value.message}${value.stack ? `\n${value.stack}` : ""}`;
    }
    if (typeof value === "string") {
        return value;
    }
    if (typeof value === "number" || typeof value === "boolean" || value == null) {
        return String(value);
    }

    try {
        return redactSensitiveText(JSON.stringify(value));
    } catch {
        return redactSensitiveText(String(value));
    }
}

export function redactSensitiveText(message: string): string {
    return message
        .replace(/(alphaVantageApiKey|twelveDataApiKey|finnhubApiKey|polygonApiKey)\s*[:=]\s*["']?[^"'\s,;]+["']?/gi, "$1: ***")
        .replace(/([?&](?:apikey|api_key|key)=)[^&\s]+/gi, "$1***")
        .replace(/\b(?:apikey|api_key|key)=([^&\s]+)/gi, (match, _value, offset, input) => {
            const prefix = input.slice(Math.max(0, offset - 1), offset);
            if (prefix === "?" || prefix === "&") {
                return match;
            }
            return match.split("=")[0] + "=***";
        });
}
