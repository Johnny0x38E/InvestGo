import { appendClientLog } from "./devlog";
import { getI18nLocale, translate } from "./i18n";

const defaultTimeoutMs = 15000;

type ApiErrorPayload = {
    error?: string;
    debugError?: string;
};

function isApiErrorPayload(payload: unknown): payload is ApiErrorPayload {
    return typeof payload === "object" && payload !== null && ("error" in payload || "debugError" in payload);
}

// Use a unified error type to distinguish “timeout” from “manual abort”, allowing callers to decide whether to suppress the error.
export class ApiAbortError extends Error {
    readonly reason: "timeout" | "aborted";

    constructor(reason: "timeout" | "aborted") {
        super(reason === "timeout" ? translate("api.timeout") : translate("api.aborted"));
        this.name = "ApiAbortError";
        this.reason = reason;
    }
}

export type ApiRequestInit = RequestInit & {
    timeoutMs?: number;
};

// Unified frontend API request wrapper with timeout, cancellation, and error logging capabilities.
export async function api<T>(path: string, init?: ApiRequestInit): Promise<T> {
    const { timeoutMs = defaultTimeoutMs, signal: externalSignal, ...requestInit } = init ?? {};
    const controller = new AbortController();
    const timeoutID = window.setTimeout(() => controller.abort(new ApiAbortError("timeout")), timeoutMs);
    const abortFromExternalSignal = () => controller.abort(externalSignal?.reason ?? new ApiAbortError("aborted"));
    const headers = new Headers(requestInit.headers ?? {});
    headers.set("Content-Type", "application/json");
    headers.set("X-InvestGo-Locale", getI18nLocale());

    if (externalSignal) {
        // Bridge the caller-provided signal to the internal controller so either side can trigger abort.
        if (externalSignal.aborted) {
            abortFromExternalSignal();
        } else {
            externalSignal.addEventListener("abort", abortFromExternalSignal, { once: true });
        }
    }

    try {
        const response = await fetch(path, {
            headers,
            ...requestInit,
            signal: controller.signal,
        });

        const isJSON = response.headers.get("content-type")?.includes("application/json");
        const payload = isJSON ? ((await response.json()) as T | ApiErrorPayload) : null;

        if (!response.ok) {
            const apiError = isApiErrorPayload(payload) ? payload : null;
            const errorMessage = apiError?.error || translate("api.requestFailed", { status: response.status });
            const debugMessage = apiError?.debugError || errorMessage;
            const error = new Error(errorMessage) as Error & { debugMessage?: string };
            error.debugMessage = debugMessage;
            throw error;
        }

        return payload as T;
    } catch (error) {
        if (error instanceof ApiAbortError) {
            throw error;
        }
        if (error instanceof DOMException && error.name === "AbortError") {
            // fetch natively only throws AbortError; restore it to the application's internally distinguishable abort type.
            throw externalSignal?.aborted ? new ApiAbortError("aborted") : new ApiAbortError("timeout");
        }
        if (error instanceof Error) {
            const debugMessage = "debugMessage" in error && typeof error.debugMessage === "string" ? error.debugMessage : error.message;
            appendClientLog("error", "api", `${requestInit.method || "GET"} ${path} -> ${debugMessage}`);
        }
        throw error;
    } finally {
        window.clearTimeout(timeoutID);
        externalSignal?.removeEventListener("abort", abortFromExternalSignal);
    }
}
