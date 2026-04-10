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

// 前端在浏览器开发服务器下也能运行，所以这里对 Wails runtime 做一层安全封装。
export async function isWindowMaximised(): Promise<boolean> {
    const runtime = getWindowRuntime();
    if (!runtime) {
        return false;
    }

    return runtime.WindowIsMaximised();
}

// 将窗口放大到当前可用工作区。
export function maximiseWindow(): void {
    getWindowRuntime()?.WindowMaximise();
}

// 从放大状态恢复到原始窗口尺寸。
export function restoreWindow(): void {
    getWindowRuntime()?.WindowUnmaximise();
}

// 触发 Wails 原生窗口拖拽。
export function startWindowDrag(): void {
    getWailsBridge()?.invoke("wails:drag");
}
