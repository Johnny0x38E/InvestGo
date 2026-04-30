# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## InvestGo — Desktop Investment Tracker

## Tech Stack & Boundaries

- **Backend**: Go 1.24, Wails **v3 alpha.54** (not v2). Entrypoint is `main.go`.
- **Frontend**: Vue 3 + TypeScript + PrimeVue 4 + Vite 8 + Chart.js 4.
- No monorepo. Go module root is the repo root. Frontend lives under `frontend/` and builds to `frontend/dist`, which `main.go` embeds via `//go:embed`.

## Developer Commands

```bash
# Frontend dev server (port 5173). Wails runtime is absent here — app runs in browser dev server.
npm run dev

# Typecheck frontend (no frontend tests exist).
npm run typecheck

# Build frontend bundle to frontend/dist. Required before Go build.
npm run build

# Run all Go tests.
env GOCACHE=/tmp/go-build-cache go test ./...

# Run focused Go tests (e.g., store layer).
env GOCACHE=/tmp/go-build-cache go test ./internal/core/store/...
```

## Desktop Build & Package

- Build scripts support **macOS Apple Silicon**, **macOS Intel**, and **Windows x64**.
- macOS prerequisites: `go`, `npm`, `swift`, `sips`, `iconutil`, `hdiutil`, `ditto`.
- Windows prerequisites: `go`, `npm`, PowerShell, Microsoft Edge WebView2 Runtime.

```bash
# macOS Apple Silicon
./scripts/build-darwin-aarch64.sh
VERSION=1.0.0 ./scripts/build-darwin-aarch64.sh
./scripts/build-darwin-aarch64.sh --dev   # F12 Web Inspector support

# macOS Intel
./scripts/build-darwin-x86_64.sh
VERSION=1.0.0 ./scripts/build-darwin-x86_64.sh
./scripts/build-darwin-x86_64.sh --dev

# Windows x64 (PowerShell)
.\scripts\build-windows-amd64.ps1
$env:VERSION="1.0.0"; .\scripts\build-windows-amd64.ps1
.\scripts\build-windows-amd64.ps1 -Dev

# Package macOS app bundle + DMG
./scripts/package-darwin-aarch64.sh
./scripts/package-darwin-x86_64.sh
```

Outputs: `build/bin/investgo-darwin-{aarch64,x86_64}` (binary), `build/macos/InvestGo.app` (app bundle), `build/bin/investgo-<version>-darwin-{aarch64,x86_64}.dmg` (installer).

## Architecture

### Frontend-Backend Communication

- **Uses standard HTTP `fetch()` to `/api/*` paths**, NOT Wails JS bindings/events. See `frontend/src/api.ts`.
- `main.go` mounts `/api/` to `internal/api` handlers and `/` to `application.BundledAssetFileServer(frontendFS)`.
- The Wails runtime (`frontend/src/wails-runtime.ts`) may be `null` when running under Vite dev server — always guard access.
- The `api()` wrapper in `frontend/src/api.ts` handles timeouts, cancellation, error redaction, and logs API errors to the developer log system.

### Desktop Shell & Window Chrome

- `UseNativeTitleBar` is a cross-platform setting read from persisted settings before `internal/platform.BuildMainWindowOptions(...)` creates the main window.
- When `UseNativeTitleBar=false`, macOS uses Wails' hidden inset title bar so the system red/yellow/green controls remain available; Windows and Linux use `Frameless` and render custom minimize/maximize/close controls in `frontend/src/components/AppHeader.vue`.
- macOS custom title-bar double-click maximize/restore is implemented in the frontend title-bar handler; do not assume native double-click behavior exists when the visible title bar is hidden.
- `frontend/src/components/AppShell.vue` owns platform-specific title-bar layout: macOS reserves left inset space for the traffic-light controls, Windows/Linux do not, and Windows/Linux use a tighter sidebar shell radius so the internal chrome aligns with the system frame.
- Window operations should go through `frontend/src/wails-runtime.ts`, which wraps Wails v3 runtime calls and safely no-ops under the Vite browser dev server.

### HTTP API Routes (`internal/api/http.go`)

Routes use Go 1.22+ pattern matching with path parameters (e.g., `{id}`). Key endpoints:

- `GET /state` — full application snapshot
- `POST /refresh` — quote refresh for all items
- `POST /items/{id}/refresh` — single-item refresh
- `PUT /settings` — update settings (also syncs proxy transport)
- `POST|PUT|DELETE /items/{id}` — CRUD for watchlist items
- `POST|PUT|DELETE /alerts/{id}` — CRUD for alerts
- `GET /history?itemId=&interval=` — historical price series
- `GET /overview?force=` — portfolio analytics
- `GET /hot?category=&sort=` — hot list
- `GET|DELETE /logs` — developer logs

All API responses are localized based on the `X-InvestGo-Locale` header. Error messages go through `internal/api/i18n` for translation.

### Key Go Packages

- `internal/api/handler.go` — HTTP handler mux for all `/api/*` endpoints (items, alerts, settings, hot list, history, overview, logs).
- `internal/core/store/` — Central state management: CRUD for `WatchlistItem`, alerts, settings, persistence to `state.json`, quote refresh orchestration, overview analytics, and alert evaluation.
- `internal/core/provider/` — Market data providers (EastMoney, Yahoo, Sina, Tencent, Xueqiu, Finnhub, TwelveData, Polygon, AlphaVantage). All implement the `QuoteProvider` interface.
- `internal/core/hot/` — Hot list / trending stocks service with caching and multi-provider pooling.
- `internal/core/marketdata/` — Provider registry and history routing.
- `internal/platform/` — Desktop window configuration, proxy transport management.
- `internal/logger/` — Custom structured logger with `logBook` and `slog` adapters.

### State Persistence

- State and logs are persisted to `os.UserConfigDir()/investgo/` (macOS: `~/Library/Application Support/investgo/`).
- `StateSnapshot` in `internal/core/model.go` is the single source of truth — all API responses return a localized snapshot.
- Settings are updated via `handleUpdateSettings` which also syncs the proxy transport at runtime.

### Core Domain Model (`internal/core/model.go`)

- `WatchlistItem` — unified tracked item (watch-only or held position with DCA entries).
- `AlertRule` — price alert conditions (`above`/`below`).
- `AppSettings` — all app settings including quote sources, theme, locale, proxy, API keys.
- `Quote` / `QuoteProvider` — real-time market data abstraction.
- `HistoryInterval` / `HistoryProvider` — historical price series (1h, 1d, 1w, 1mo, 1y, 3y, all).

### Caching Strategy

The `Store` maintains several TTL caches to avoid redundant network calls:

- `refreshCache` — full quote refresh results (TTL from `HotCacheTTLSeconds`).
- `itemRefreshCache` — per-item refresh results.
- `historyCache` — historical OHLCV series (per-interval TTL, longer than quote TTL).
- `overviewCache` — portfolio analytics (derived cache TTL), keyed by `holdingsUpdatedAt` stamp.
- `snapshotCache` — atomic pointer cache for repeated `Snapshot()` calls when state hasn't changed.

Price refreshes only invalidate price-related caches; structural mutations (item CRUD, settings changes) invalidate all caches.

### Quote Provider Architecture

- `marketdata.DefaultRegistry()` registers all providers and builds a `HistoryRouter`.
- Each market (CN, HK, US) has its own configurable quote source in settings.
- `Store.Refresh()` batches items by their active market-specific provider so multi-market lists still respect per-market source settings.
- `core.ResolveQuoteTarget()` normalizes symbol/market pairs into canonical keys used across all quote providers.

### Frontend Architecture

- `App.vue` is the root component. It holds global reactive state (items, alerts, settings, runtime) and coordinates module switching, auto-refresh, and dialog flows.
- Business logic is extracted into composables: `useItemDialog`, `useAlertDialog`, `useConfirmDialog`, `useHistorySeries`, `useDeveloperLogs`.
- `frontend/src/api.ts` is the single API client. All backend communication goes through the `api<T>()` wrapper.
- `frontend/src/i18n.ts` contains bilingual copy (zh-CN / en-US). User-facing text changes must update both locales.
- `frontend/src/theme.ts` defines the PrimeVue preset and color theme seeds. Dark mode is toggled via `.app-dark` class on `<html>`, driven by `dataset.themeMode`.

## Build & Runtime Gotchas

- `--dev` build flag enables terminal logging and F12 Web Inspector **support**, but the inspector still requires the `developerMode` app setting to be enabled at runtime.
- Version is injected at link time: `-X main.appVersion=$APP_VERSION`. Without `VERSION` or `APP_VERSION`, the app shows `"dev"`.
- The build script renders `frontend/src/assets/app-dock.svg` to `build/appicon.png` via a Swift script before compiling.
- There are **no frontend tests**. Go tests exist in `internal/**`.
- Proxy mode can be `none`, `system`, or `custom`. System proxy detection currently probes `scutil --proxy` only on macOS.
- Windows builds require WebView2 Runtime on the target machine.

## Code Conventions

- Frontend uses PrimeVue's Composition API. Custom CSS lives in `frontend/src/styles/`.
- Dark mode is toggled via `.app-dark` class on `<html>`, driven by `dataset.themeMode` and `dataset.theme`.
- Backend uses a custom structured logger (`internal/logger`) with `logBook` and `slog` adapters.
