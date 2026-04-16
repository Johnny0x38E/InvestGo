type WailsWindowRuntime = {
    WindowIsMaximised(): Promise<boolean>;
    WindowMaximise(): void;
    WindowUnmaximise(): void;
};

type WailsBridge = {
    invoke(message: string): void;
};

function getWindowRuntime(): WailsWindowRuntime | null {
    return (window as Window & { runtime?: WailsWindowRuntime }).runtime ?? null;
}

function getWailsBridge(): WailsBridge | null {
    return (window as Window & { _wails?: WailsBridge })._wails ?? null;
}

// The frontend can also run under the browser dev server, so the Wails runtime is safely wrapped here.
export async function isWindowMaximised(): Promise<boolean> {
    const runtime = getWindowRuntime();
    if (!runtime) {
        return false;
    }

    return runtime.WindowIsMaximised();
}

// Maximize the window to the current available workspace.
export function maximiseWindow(): void {
    getWindowRuntime()?.WindowMaximise();
}

// Restore the window from maximized state to its original size.
export function restoreWindow(): void {
    getWindowRuntime()?.WindowUnmaximise();
}

// Trigger native Wails window dragging.
export function startWindowDrag(): void {
    getWailsBridge()?.invoke("wails:drag");
}
