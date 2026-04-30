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
- The shell can run with a custom title bar or a native title bar depending on `useNativeTitleBar`.
- `.window-bar`, the shell top bars, and the sidebar chrome currently reserve `76px` of leading space for macOS title-bar controls. Treat that value as a current implementation detail, not a permanent cross-platform design truth.
- Keep font stacks resilient. The current CSS prefers macOS fonts first, so any typography change should keep good fallbacks for non-macOS systems.

## Cross-platform preparation

- Avoid visual assumptions that only make sense on macOS, especially around title-bar spacing, translucency, and font availability.
- If a design change introduces a shell-specific affordance, note what would need to change for a Windows x64 webview host.
- Prefer layout rules driven by data attributes or container structure over fixed offsets tied to one platform.
