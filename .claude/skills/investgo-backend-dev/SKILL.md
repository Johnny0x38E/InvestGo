---
name: investgo-backend-dev
description: Implement and modify InvestGo Go backend behavior, including Store mutations, runtime state, API handlers, market data providers, persistence, logging, and platform-sensitive host integration. Use when requests involve backend development, API changes, quote or history logic, alerts, persistence, refresh behavior, or when editing internal/api/**, internal/core/**, internal/logger/**, internal/platform/**, or main.go.
---

# InvestGo Backend Dev

## Workflow

1. Read `references/backend-context.md` before changing backend code.
2. Keep HTTP handlers thin. Put business rules, state mutation, and coordination logic into `Store` methods, `internal/core/store`, `internal/core/hot`, `internal/core/marketdata`, or dedicated providers.
3. When state or API payloads change, update the matching frontend TypeScript contract in the same change so `StateSnapshot`, `OverviewAnalytics`, and history payloads stay in sync.
4. Preserve quote, history, and hot-list fallback behavior. External market data is fragile, so prefer additive fixes over rewriting provider order.
5. Add or update tests under `internal/core/**` or `internal/api/**` for any non-trivial parsing, state, API, cache, or provider change.
6. Prefer the current backend boundaries: shared contracts in `internal/core/model.go`, persistence and mutation logic in `internal/core/store`, provider registration in `internal/core/marketdata`, provider implementations in `internal/core/provider`, hot-list orchestration in `internal/core/hot`, platform runtime code in `internal/platform`, and HTTP orchestration in `internal/api`.
7. Isolate OS-specific behavior behind helpers or runtime guards. Current hot spots include `internal/api/open_external.go`, `internal/platform/proxy.go`, `internal/platform/window.go`, and build-time flags.
8. Keep slow or network work outside long-held store locks and avoid persistence changes that silently break existing state files or repository compatibility.
9. If the change affects user-visible copy, API-localized errors, or default labels consumed by the frontend, keep the Chinese and English i18n paths aligned instead of updating only one locale.
10. New or revised code comments must be professional English. Do not leave touched backend comments in Chinese or introduce mixed-language annotations.

## Validation

- Use the standard Go checks from `AGENTS.md`.
- Add frontend type checks only when backend payloads or contracts changed.

## References

- Use `references/backend-context.md` for the route surface, store boundaries, provider hierarchy, and platform hot spots.
