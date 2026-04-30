# Backend Context

Use `AGENTS.md` for exact commands. This reference keeps the backend boundaries that are easy to forget while implementing or debugging.

## Route surface

- `GET /api/state`
- `GET /api/overview`
- `GET /api/logs`
- `DELETE /api/logs`
- `POST /api/client-logs`
- `GET /api/hot`
- `GET /api/history`
- `POST /api/refresh`
- `POST /api/open-external`
- `PUT /api/settings`
- `POST /api/items`
- `POST /api/items/{id}/refresh`
- `PUT /api/items/{id}`
- `PUT /api/items/{id}/pin`
- `DELETE /api/items/{id}`
- `POST /api/alerts`
- `PUT /api/alerts/{id}`
- `DELETE /api/alerts/{id}`

## Current runtime shape

- `GET /api/state` returns a localized `StateSnapshot` for startup and normal resync.
- `POST /api/refresh` is the full live quote path. `Store.Refresh()` groups items by effective per-market quote source, fetches quotes, refreshes FX when stale, evaluates alerts, persists state, and returns a rebuilt `StateSnapshot`.
- `POST /api/items/{id}/refresh` refreshes one tracked item and returns a full `StateSnapshot`.
- `GET /api/history` is the on-demand chart path. `Store.ItemHistory()` finds one item and delegates to the history router.
- `GET /api/overview` is a separate analytics path. The store converts holdings into the configured dashboard currency, builds breakdown slices, and replays history plus DCA entries into a portfolio trend series.
- `GET /api/hot` is independent from the watchlist snapshot. It accepts `category`, `sort`, `q`, `page`, `pageSize`, and optional `force`.

## Provider boundaries

- Contracts live in `internal/core/model.go`: `QuoteProvider`, `HistoryProvider`, `StateSnapshot`, `WatchlistItem`, `AppSettings`, `OverviewAnalytics`, `HotListResponse`, and history payloads.
- Provider registration lives in `internal/core/marketdata/registry.go`. It is the single source of truth for provider capabilities and settings UI options.
- Provider implementations live in `internal/core/provider/**`. Current sources include EastMoney, Yahoo Finance, Sina Finance, Xueqiu, Tencent Finance, Alpha Vantage, Twelve Data, Finnhub, and Polygon.
- `HistoryRouter` in `internal/core/marketdata/history_router.go` routes only across history-capable providers and derives preference from `CNQuoteSource`, `HKQuoteSource`, and `USQuoteSource`, plus market defaults.
- Hot-list orchestration lives in `internal/core/hot/**`. It handles category pools, cache TTL, sorting, quote overlays, and enrichment.

## Working rules

- Keep handlers thin and move business rules into `Store` methods, `internal/core/store`, `internal/core/hot`, `internal/core/marketdata`, or provider helpers.
- Treat `internal/core/model.go` as the frontend-facing contract layer, `internal/core/store` as the state and persistence boundary, and `internal/core/provider` as external data adapters.
- Update frontend contracts whenever `StateSnapshot`, `OverviewAnalytics`, settings, item, alert, runtime, or history payloads change.
- Keep locale-sensitive backend text aligned with the frontend i18n layer when API-visible copy or localized errors change.
- Preserve current fallback semantics unless the task explicitly changes them. External market data is fragile, so additive fixes are safer than broad provider-order rewrites.
- Keep slow or network work outside long-held store locks.
- Prefer explicit, compatible persistence changes over silent schema breaks. `Store` persists through the `store.Repository` interface and `JSONRepository`.
- Keep touched backend comments in professional English only.

## WatchlistItem semantics

- `WatchlistItem` backs both watch-only entries and active holdings.
- Watch-only means `Quantity == 0` and no valid `DCAEntries`. Server sanitization clears `AcquiredAt` for watch-only items so they do not leak into overview trend calculations.
- Active holding means `Quantity > 0` or non-empty `DCAEntries`. The backend computes `Position *PositionSummary` and `DCASummary *DCASummary`; the frontend should consume these derived fields instead of re-deriving them.

## Settings and compatibility

- Current frontend-facing settings are per-market quote sources, hot cache TTL, appearance, locale, proxy, provider API keys, dashboard currency, developer mode, and native-title-bar behavior.
- There is no separate frontend-supported history-source setting. History routing follows the current quote-source preference plus history-provider capability.
- Legacy quote-source fields can appear in compatibility paths for older state files and normalization logic; do not treat them as new frontend product knobs.

## Platform hot spots

- `internal/api/open_external.go` dispatches external URL opening to `open`, `rundll32`, or `xdg-open`.
- `internal/platform/proxy.go` applies system proxy settings and currently only probes `scutil --proxy` on `darwin`.
- `internal/platform/window.go` builds `application.WebviewWindowOptions` and keeps macOS window behavior out of `main.go`.
- Embedded `frontend/dist` means a stale frontend build can surface as a backend-looking bug.

## Cross-platform preparation

- Keep macOS-only behavior isolated under `internal/platform` or other platform helpers before adding Windows x64 support.
- Treat external-link opening, proxy detection, title-bar behavior, and release metadata as platform-specific seams.
- Keep API and store logic platform-neutral where possible so only shell integration needs conditional handling.
