# Agent Notes for InvestGo

## Tech Stack & Boundaries

- **Backend**: Go 1.24, Wails **v3 alpha.54** (not v2). Entrypoint is `main.go`.
- **Frontend**: Vue 3 + TypeScript + PrimeVue 4 + Vite 8 + Chart.js 4.
- No monorepo. Go module root is the repo root. Frontend lives under `frontend/` and builds to `frontend/dist`, which `main.go` embeds via `//go:embed`.

## Exact Developer Commands

```bash
# Frontend only (port 5173). Wails runtime is absent here; the app runs in a browser dev server.
npm run dev

# Typecheck frontend (does not run tests).
npm run typecheck

# Build frontend bundle to frontend/dist. Required before Go build.
npm run build

# Run Go tests. GOCACHE path matches the build script default.
env GOCACHE=/tmp/go-build-cache go test ./...
```

## Desktop Build & Package

- macOS build and packaging scripts target **Apple Silicon** (`GOOS=darwin GOARCH=arm64`, `CGO_ENABLED=1`) and **Intel** (`GOOS=darwin GOARCH=amd64`, `CGO_ENABLED=1`).
- Windows build script targets **Windows 11 x64** (`GOOS=windows GOARCH=amd64`, `CGO_ENABLED=0`) and produces a runnable `.exe`, not an installer.
- macOS prerequisites: macOS 13+, `go`, `npm`, `swift`, `sips`, `iconutil`, `hdiutil`, `ditto`.
- Windows prerequisites: `go`, `npm`, and Microsoft Edge WebView2 Runtime.
- Build macOS binary:
    ```bash
    ./scripts/build-darwin-aarch64.sh
    VERSION=1.0.0 ./scripts/build-darwin-aarch64.sh
    ./scripts/build-darwin-aarch64.sh --dev
    ./scripts/build-darwin-x86_64.sh
    VERSION=1.0.0 ./scripts/build-darwin-x86_64.sh
    ./scripts/build-darwin-x86_64.sh --dev
    ```
    Outputs `build/bin/investgo-darwin-aarch64` or `build/bin/investgo-darwin-x86_64`.
- Build Windows binary from PowerShell:
    ```powershell
    .\scripts\build-windows-amd64.ps1
    $env:VERSION="1.0.0"; .\scripts\build-windows-amd64.ps1
    .\scripts\build-windows-amd64.ps1 -Dev
    ```
    Outputs `build/bin/investgo-windows-amd64.exe`.
- Package app bundle + DMG:
    ```bash
    ./scripts/package-darwin-aarch64.sh
    VERSION=1.0.0 ./scripts/package-darwin-aarch64.sh
    ./scripts/package-darwin-x86_64.sh
    VERSION=1.0.0 ./scripts/package-darwin-x86_64.sh
    ```
    Outputs `build/macos/InvestGo.app` and an architecture-specific DMG under `build/bin/`.

## Critical Architecture Notes

- **Frontend-backend communication uses standard HTTP `fetch()` to `/api/*` paths**, not Wails JS bindings/events. See `frontend/src/api.ts`.
- `main.go` mounts `/api/` to `internal/api` handlers and `/` to `application.BundledAssetFileServer(frontendFS)`.
- The Wails runtime is safely wrapped in `frontend/src/wails-runtime.ts`; it may be `null` when running under Vite dev server.
- `UseNativeTitleBar` is a cross-platform setting. When disabled, macOS uses Wails' hidden inset title bar and keeps the system red/yellow/green controls; Windows and Linux use `Frameless` and render custom window controls in `AppHeader.vue`.
- Custom title-bar layout is platform-sensitive in `AppShell.vue`: macOS reserves left inset space for traffic-light controls, while Windows/Linux do not; Windows/Linux also use a tighter sidebar shell radius to match the system window frame.
- State and logs are persisted to `os.UserConfigDir()/investgo/` (macOS: `~/Library/Application Support/investgo/`).

## Build & Runtime Gotchas

- `--dev` build flag enables terminal logging and F12 Web Inspector **support**, but the inspector still requires the `developerMode` app setting to be enabled at runtime.
- macOS custom title-bar double-click maximize/restore is implemented in the frontend title-bar handler because the native double-click behavior is not provided automatically when the visible title bar is hidden.
- Version is injected at link time: `-X main.appVersion=$APP_VERSION`. Without `VERSION` or `APP_VERSION`, the app shows `"dev"`.
- The macOS build scripts render `frontend/src/assets/app-dock.svg` to `build/appicon.png` via a Swift script before compiling.
- The Windows build script copies `frontend/src/assets/appicon.png` to `build/appicon.png` when that file is missing. It does not yet embed a Windows `.ico`, version resource, or manifest.
- Windows installer packaging is not implemented yet. Use the Windows build script only for a runnable `.exe`.
- `.gitignore` ignores `AGENTS.md`, `CLAUDE.md`, `.agents/`, and `.claude/`.

## Testing

- There are **no frontend tests**. Go tests exist in `internal/**`.
- Run focused Go tests normally: `go test ./internal/core/store/...`.

## Code Conventions

- Frontend uses PrimeVue's Composition API and custom CSS files in `frontend/src/styles/`.
- Dark mode is toggled via `.app-dark` class on `<html>`, driven by `dataset.themeMode` and `dataset.theme`.
- Backend uses a custom structured logger (`internal/logger`) with `logBook` and `slog` adapters.
