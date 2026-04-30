# Wails v3 Context

Use `AGENTS.md` for the standard build, test, and run commands. This reference only captures Wails v3 host details that are easy to miss when changing InvestGo's desktop shell.

## Current host responsibilities

- `main.go` is the Wails v3 entrypoint and composition root.
- It embeds `frontend/dist` and `build/appicon.png`.
- It builds a single `http.ServeMux` that serves both `/api/*` and the bundled frontend assets.
- It initialises logging, proxy detection, quote and history providers, the store, and the hot list service before starting the desktop app.
- It delegates platform proxy and window option details to `internal/platform/**` instead of holding that logic inline.

## Routing and asset contract

- `frontend/dist` is embedded with `//go:embed frontend/dist`, then narrowed with `fs.Sub(..., "frontend/dist")`.
- `/api/` is registered with `api.NewHandler(...)`.
- `/` is served through `application.BundledAssetFileServer(frontendFS)`.
- Wails serves both through `application.AssetOptions{Handler: mux}`, so route changes must keep API and static asset delivery compatible.

## Lifecycle and runtime behavior

- Startup order currently matters: configure logging, create the proxy-aware HTTP client, build the provider registry, create the store, apply persisted proxy settings, create handlers, create the application, configure the window, then `Run()`.
- `OnShutdown` persists store state and should remain safe even when shutdown is triggered from the desktop shell.
- Panic reporting is wired through `PanicHandler` into the app log book.

## Window and settings integration

- The main window is created once through `app.Window.NewWithOptions(...)`.
- Window defaults come from the desktop shell, but `UseNativeTitleBar` is read from the store snapshot and fed into `internal/platform.BuildMainWindowOptions(...)`, which changes macOS title-bar behavior.
- `BuildMainWindowOptions` currently sets `1200x828`, `MinWidth=1200`, `MinHeight=828`, a desktop background colour, Windows theme inheritance, and macOS translucent backdrop defaults.
- F12 only opens DevTools when both developer mode is enabled in settings and the binary was built with devtools support.
- The shell currently sets macOS backdrop and termination behavior, plus a desktop background color that should stay aligned with the frontend shell.

## Platform-sensitive behavior

- `internal/platform.ApplySystemProxy` intentionally respects explicit proxy environment variables and only probes `scutil --proxy` on macOS.
- `internal/platform.BuildMainWindowOptions` centralises the `application.MacOptions` and `MacWindow` hot spots that used to live inline in `main.go`.
- `internal/api/open_external.go` is another shell-level seam because it maps URL opening to different OS commands.
- Avoid assuming browser-only features such as direct history routing, multi-tab navigation, or web deployment paths that do not exist inside the bundled desktop app.
- Keep future Windows support in mind when touching shell layout, key bindings, build flags, or OS-specific commands.
- If host-layer behavior changes what the user sees by default, keep the frontend Chinese and English i18n paths aligned.
- Keep touched host-layer comments in professional English only.
