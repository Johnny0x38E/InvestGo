# UI Context

## Surface map

- Use `frontend/src/App.vue` for app orchestration, `AppShell.vue` and `AppWorkspace.vue` for shell layout, `components/modules/` for feature surfaces, and `components/dialogs/` for focused edit flows.
- `AppHeader.vue`, `AppSidebar.vue`, `ModuleTabs.vue`, and `SummaryStrip.vue` define the shared chrome and information hierarchy.
- `frontend/src/style.css` is the main visual system. Prefer changing tokens there before adding local overrides.

## Visual system

- Preserve the CSS variable model built around `--app-bg`, `--panel-*`, `--accent`, `--rise`, and `--fall`.
- Preserve root theme switches driven by `data-theme`, `data-color-theme`, `data-price-color-scheme`, and `data-font-preset`.
- Reuse PrimeVue controls instead of introducing ad hoc HTML widgets when a matching control already exists.
- Favor a desktop-native feel: compact spacing, stable module rhythm, and clear emphasis on prices, state, and actions.
- If labels, helper text, or status copy change, keep both Chinese and English i18n entries in sync.
- Keep touched design comments in professional English only.

## Desktop shell constraints

- `main.go` sets the main window size to `1200x828` with a minimum width of `1200` and minimum height of `828`.
- The shell can run with a custom title bar or a native title bar depending on `useNativeTitleBar`, and the setting applies on macOS, Windows, and Linux.
- macOS custom title-bar mode keeps the system red/yellow/green controls through Wails' hidden inset title bar; `.window-bar`, shell top bars, and sidebar chrome reserve `76px` of leading space only in that mode.
- Windows and Linux custom title-bar mode uses Wails `Frameless` plus custom window controls in `AppHeader.vue`. These platforms do not reserve macOS leading space, and the collapsed sidebar toggle should stay close to the left content edge.
- `AppShell.vue` owns platform chrome classes and radius variables. Keep Windows/Linux sidebar shell radii tighter than macOS unless the outer window frame radius is also intentionally changed.
- Keep font stacks resilient. The current CSS prefers macOS fonts first, so any typography change should keep good fallbacks for non-macOS systems.

## Cross-platform preparation

- Avoid visual assumptions that only make sense on macOS, especially around title-bar spacing, translucency, large internal shell radii, and font availability.
- If a design change introduces a shell-specific affordance, note what would need to change for Windows x64 and Linux frameless webview hosts.
- Prefer layout rules driven by data attributes or container structure over fixed offsets tied to one platform.
