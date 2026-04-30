---
name: investgo-wails-v3
description: Implement and modify InvestGo's Wails v3 host integration, including app lifecycle, embedded frontend assets, desktop window setup, and runtime bridge behavior. Use when requests involve Wails v3 app wiring, main.go desktop shell changes, internal/platform/**, asset embedding, window lifecycle, devtools or dev-mode behavior, or desktop-specific frontend constraints in this repository.
---

# InvestGo Wails v3

## Workflow

1. Read `references/wails-v3-context.md` before changing the desktop host layer.
2. Treat `main.go` as the Wails composition root. Keep domain logic in `internal/**`, platform runtime details in `internal/platform/**`, and frontend UI logic in `frontend/src/**` instead of pushing behavior into the host shell.
3. Preserve the embed contract: `main.go` serves `frontend/dist` through Wails, and `/api/*` routes still need to resolve through the same asset handler.
4. Keep lifecycle work explicit. Startup should initialise logging, proxy setup, providers, store, and handlers in a predictable order, and shutdown must still flush durable state safely.
5. Handle window and platform options carefully. Changes to title bar, frameless mode, background, DevTools, or key bindings must continue to respect settings-driven behavior and the helper boundaries in `internal/platform/window.go` and `internal/platform/proxy.go`.
6. Remember the frontend runs inside a desktop webview, not a normal browser tab. Avoid adding browser assumptions that fight the desktop shell, local routing model, or Windows/Linux frameless support.
7. If the change also affects build scripts, packaging, or frontend contracts, update the adjacent layer in the same change instead of patching only the Wails host.
8. If shell-level behavior surfaces user-visible copy or locale-sensitive defaults, keep the frontend i18n layer aligned in both Chinese and English.
9. New or revised comments in `main.go` and other host-layer files must be professional English.

## Validation

- Use the smallest relevant checks from `AGENTS.md` for the touched layers.
- Rebuild `frontend/dist` before any final Go build that depends on embedded assets.
- Use dev-mode or a dev build only when the task depends on logs, F12 DevTools, or runtime desktop behavior.

## References

- Use `references/wails-v3-context.md` for host responsibilities, lifecycle order, routing, and current platform-sensitive runtime behavior.
