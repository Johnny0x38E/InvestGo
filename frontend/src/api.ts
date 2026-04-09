import { appendClientLog } from "./devlog";

const defaultTimeoutMs = 15000;

export class ApiAbortError extends Error {
    readonly reason: "timeout" | "aborted";

    constructor(reason: "timeout" | "aborted") {
        super(reason === "timeout" ? "请求超时" : "请求已取消");
        this.name = "ApiAbortError";
        this.reason = reason;
    }
}

export type ApiRequestInit = RequestInit & {
    timeoutMs?: number;
};

export async function api<T>(path: string, init?: ApiRequestInit): Promise<T> {
    const { timeoutMs = defaultTimeoutMs, signal: externalSignal, ...requestInit } = init ?? {};
    const controller = new AbortController();
    const timeoutID = window.setTimeout(() => controller.abort(new ApiAbortError("timeout")), timeoutMs);
    const abortFromExternalSignal = () => controller.abort(externalSignal?.reason ?? new ApiAbortError("aborted"));

    if (externalSignal) {
        if (externalSignal.aborted) {
            abortFromExternalSignal();
        } else {
            externalSignal.addEventListener("abort", abortFromExternalSignal, { once: true });
        }
    }

    try {
        const response = await fetch(path, {
            headers: {
                "Content-Type": "application/json",
                ...(requestInit.headers ?? {}),
            },
            ...requestInit,
            signal: controller.signal,
        });

        const isJSON = response.headers.get("content-type")?.includes("application/json");
        const payload = isJSON ? ((await response.json()) as T | { error?: string }) : null;

        if (!response.ok) {
            const message = typeof payload === "object" && payload !== null && "error" in payload ? payload.error : undefined;
            const errorMessage = message || `请求失败: ${response.status}`;
            throw new Error(errorMessage);
        }

        return payload as T;
    } catch (error) {
        if (error instanceof ApiAbortError) {
            throw error;
        }
        if (error instanceof DOMException && error.name === "AbortError") {
            throw externalSignal?.aborted ? new ApiAbortError("aborted") : new ApiAbortError("timeout");
        }
        if (error instanceof Error) {
            appendClientLog("error", "api", `${requestInit.method || "GET"} ${path} -> ${error.message}`);
        }
        throw error;
    } finally {
        window.clearTimeout(timeoutID);
        externalSignal?.removeEventListener("abort", abortFromExternalSignal);
    }
}
