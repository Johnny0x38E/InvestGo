# Frontend Context

## File ownership

- `frontend/src/App.vue`: bootstrap, top-level snapshot application, module switching, refresh scheduling, dialog visibility, selected item state, settings draft preview, and status messaging.
- `frontend/src/components/AppShell.vue` and `frontend/src/components/AppWorkspace.vue`: shell layout, sidebar behavior, shared chrome, and module staging.
- `frontend/src/api.ts`, `types.ts`, `forms.ts`, `format.ts`, `theme.ts`, `constants.ts`, and `i18n.ts`: contract and helper layer that must stay coherent.
- `frontend/src/components/modules/`: feature surfaces for overview, watchlist, holdings, hot, alerts, and settings.
- `frontend/src/components/dialogs/`: focused edit flows for items, DCA details, alerts, and confirms.
- `frontend/src/composables/`: reusable history, developer-log, and sidebar behavior.

## Safe edit rules

- Keep shared state in `App.vue` until there is a clear reuse boundary.
- Update `types.ts` and `forms.ts` together when payload or form shapes change.
- Update both Chinese and English entries in `frontend/src/i18n.ts` whenever visible copy changes.
- Keep formatting and serialization logic out of SFC templates.
- Reuse PrimeVue controls and current state primitives instead of introducing a second UI pattern.
- Preserve auto-refresh, history reload, chart-cache, and status-text behavior unless the task explicitly changes them.
- Use `item.position?.hasPosition` to determine whether an item is an active holding.
- Keep touched comments in Vue, TypeScript, and CSS in professional English only.

## Backend touchpoints

- `/api/state`: initial `StateSnapshot`.
- `/api/overview`: backend-computed `OverviewAnalytics`.
- `/api/refresh`: live quote refresh; returns updated `StateSnapshot`.
- `/api/items/{id}/refresh`: selected-item live quote refresh; returns updated `StateSnapshot`.
- `/api/history`: chart history for a single item (`?itemId=&interval=`).
- `/api/hot`: hot list data (`?category=&sort=&q=&page=&pageSize=`).
- `/api/settings`: persisted settings update; returns updated `StateSnapshot`.
- `/api/items`, `/api/items/{id}/refresh`, `/api/items/{id}/pin`, and `/api/alerts`: CRUD, selected refresh, and pinning flows; writes return updated `StateSnapshot`.
- `/api/logs` and `/api/client-logs`: developer log read/write.
- `/api/open-external`: open links through the backend.

## Current UI data flows

- App boot uses `/api/state`, then triggers a silent `/api/refresh`.
- Watchlist chart data is not bundled into the snapshot. `useHistorySeries()` loads and caches `/api/history` responses by `itemId + interval`.
- Overview analytics are fetched separately from `/api/overview`, so overview-only regressions can come from a different backend path than watchlist or holdings regressions.
- Hot US rows may display Chinese names even when the live quote source is Yahoo, because the backend can apply an extra EastMoney naming pass after quote fetch.

## Desktop and platform notes

- The frontend runs inside a Wails webview rather than a general browser SPA deployment.
- Avoid browser-only routing or storage assumptions unless the app already relies on them.
- External links go through `/api/open-external`, which fans out to OS-specific opener commands in the backend.
- Theme and shell layout must keep working with both custom and native title bars on macOS, Windows, and Linux.
- `AppShell.vue` owns platform-specific window chrome spacing and shell radii. macOS custom chrome reserves traffic-light space; Windows/Linux frameless chrome does not.
- `AppHeader.vue` owns custom window controls and double-click maximize/restore behavior. Use `frontend/src/wails-runtime.ts` for window operations so browser dev server mode stays safe.
- Prefer cross-platform font fallbacks and shell-safe interactions so Windows/Linux frameless support does not require a redesign.
