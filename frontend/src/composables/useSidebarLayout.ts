import { onBeforeUnmount, ref } from "vue";

export function useSidebarLayout() {
    const appShellRef = ref<HTMLElement | null>(null);
    const sidebarWidth = ref(220);
    const sidebarHidden = ref(false);

    let sidebarResizeActive = false;

    function clampSidebarWidth(value: number): number {
        return Math.min(Math.max(Math.round(value), 220), 380);
    }

    function toggleSidebar(): void {
        sidebarHidden.value = !sidebarHidden.value;
    }

    function handleSidebarResize(event: MouseEvent): void {
        if (!sidebarResizeActive) {
            return;
        }
        const shellLeft = appShellRef.value?.getBoundingClientRect().left ?? 0;
        sidebarWidth.value = clampSidebarWidth(event.clientX - shellLeft);
    }

    function stopSidebarResize(): void {
        if (!sidebarResizeActive) {
            return;
        }
        sidebarResizeActive = false;
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
        window.removeEventListener("mousemove", handleSidebarResize);
        window.removeEventListener("mouseup", stopSidebarResize);
    }

    function startSidebarResize(): void {
        sidebarHidden.value = false;
        if (sidebarResizeActive) {
            return;
        }
        sidebarResizeActive = true;
        document.body.style.cursor = "col-resize";
        document.body.style.userSelect = "none";
        window.addEventListener("mousemove", handleSidebarResize);
        window.addEventListener("mouseup", stopSidebarResize);
    }

    onBeforeUnmount(() => {
        stopSidebarResize();
    });

    return {
        appShellRef,
        sidebarWidth,
        sidebarHidden,
        toggleSidebar,
        startSidebarResize,
    };
}
