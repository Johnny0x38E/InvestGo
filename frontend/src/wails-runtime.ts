type WailsWindowRuntime = {
    WindowIsMaximised?: () => Promise<boolean>;
    WindowMaximise?: () => void;
    WindowUnmaximise?: () => void;
    WindowMinimise?: () => void;
    WindowClose?: () => void;
};

type WailsBridge = {
    invoke(message: string): void;
    environment?: {
        OS?: string;
    };
};

function getWindowRuntime(): WailsWindowRuntime | null {
    return (window as Window & { runtime?: WailsWindowRuntime }).runtime ?? null;
}

function getWailsBridge(): WailsBridge | null {
    return (window as Window & { _wails?: WailsBridge })._wails ?? null;
}

type DesktopPlatform = "darwin" | "windows" | "linux" | "browser" | "unknown";

const windowRuntimeObject = 6;
const windowMethodClose = 2;
const windowMethodIsMaximised = 14;
const windowMethodMaximise = 16;
const windowMethodMinimise = 17;
const windowMethodToggleMaximise = 39;
const windowMethodUnMaximise = 42;

async function callWailsWindowMethod<T>(method: number): Promise<T | null> {
    if (!getWailsBridge()) {
        return null;
    }

    const response = await fetch("/wails/runtime", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            object: windowRuntimeObject,
            method,
        }),
    });

    if (!response.ok) {
        throw new Error(await response.text());
    }

    const contentType = response.headers.get("Content-Type") ?? "";
    if (contentType.includes("application/json")) {
        return (await response.json()) as T;
    }

    const text = await response.text();
    return (text as T) ?? null;
}

export function getDesktopPlatform(): DesktopPlatform {
    const os = getWailsBridge()?.environment?.OS;
    if (os === "darwin" || os === "windows" || os === "linux") {
        return os;
    }
    if (!os) {
        return "browser";
    }
    return "unknown";
}

export function shouldShowCustomWindowControls(useNativeTitleBar: boolean): boolean {
    const platform = getDesktopPlatform();
    return !useNativeTitleBar && (platform === "windows" || platform === "linux");
}

export function shouldReserveMacWindowControls(useNativeTitleBar: boolean): boolean {
    return !useNativeTitleBar && getDesktopPlatform() === "darwin";
}

// The frontend can also run under the browser dev server, so the Wails runtime is safely wrapped here.
export async function isWindowMaximised(): Promise<boolean> {
    const runtime = getWindowRuntime();
    if (runtime?.WindowIsMaximised) {
        return runtime.WindowIsMaximised();
    }

    return (await callWailsWindowMethod<boolean>(windowMethodIsMaximised)) ?? false;
}

// Maximize the window to the current available workspace.
export async function maximiseWindow(): Promise<void> {
    const runtime = getWindowRuntime();
    if (runtime?.WindowMaximise) {
        runtime.WindowMaximise();
        return;
    }

    await callWailsWindowMethod<void>(windowMethodMaximise);
}

// Restore the window from maximized state to its original size.
export async function restoreWindow(): Promise<void> {
    const runtime = getWindowRuntime();
    if (runtime?.WindowUnmaximise) {
        runtime.WindowUnmaximise();
        return;
    }

    await callWailsWindowMethod<void>(windowMethodUnMaximise);
}

export async function toggleMaximiseWindow(): Promise<void> {
    await callWailsWindowMethod<void>(windowMethodToggleMaximise);
}

export async function minimiseWindow(): Promise<void> {
    const runtime = getWindowRuntime();
    if (runtime?.WindowMinimise) {
        runtime.WindowMinimise();
        return;
    }

    await callWailsWindowMethod<void>(windowMethodMinimise);
}

export async function closeWindow(): Promise<void> {
    const runtime = getWindowRuntime();
    if (runtime?.WindowClose) {
        runtime.WindowClose();
        return;
    }

    await callWailsWindowMethod<void>(windowMethodClose);
}

// Trigger native Wails window dragging.
export function startWindowDrag(): void {
    getWailsBridge()?.invoke("wails:drag");
}
