# Debugging Playbook

## Fast triage

- Blank or stale UI after a backend change: verify `frontend/dist` was rebuilt before the Go binary was built.
- F12 does not open DevTools: check both `developerMode` in settings and whether the binary was built with `--dev`.
- Buttons or links that should open a browser do nothing: inspect `/api/open-external`, `internal/api/open_external.go`, and the platform-specific opener path for the current OS.
- Quote or history data is missing: inspect provider logs, fallback behavior, request parameters, and network assumptions before touching UI code.
- Overview is wrong while watchlist and holdings look right: inspect `/api/overview`, `overviewCalculator`, currency conversion, and history replay rules before assuming snapshot drift.
- US hot rows show ticker codes or fail only on large categories such as S&P 500: inspect `internal/core/hot/**`, US pool normalization, EastMoney naming enrichment, provider fallback, and secid chunking before changing frontend rendering.
- Logs panel is empty: check `/api/logs`, `LogBook` setup, and whether developer mode is actually enabled.
- Build or packaging fails early: identify the missing external tool first before editing scripts.
- One locale looks correct and another does not: inspect `frontend/src/i18n.ts` parity and any backend-localized error path before changing UI logic.

## Investigation order

1. Reproduce with the smallest command or UI path possible.
2. Decide which layer owns the symptom: frontend, API contract, store, market data provider, build, packaging, or platform.
3. Gather logs and exact failing output before changing code.
4. Fix the narrowest layer that explains the symptom.
5. Re-run the minimal reproducer and then the tightest relevant validation command set.

Use the smallest relevant command set from `AGENTS.md`; only switch to a dev-mode run or dev build when logs or DevTools are part of the repro.

## Common platform blockers

- `scutil`, `swift`, `sips`, `iconutil`, `hdiutil`, and `ditto` are macOS-specific assumptions in the current runtime or release toolchain; runtime proxy and window behavior now live under `internal/platform/**`.
- External URL opening now fans out by OS in `internal/api/open_external.go`, so failures may be command-availability or shell-integration issues rather than route bugs.
- `scripts/render-svg-icon.swift` depends on AppKit and cannot be reused for Windows x64 as-is.
- Shell layout and native title-bar behavior may differ once Windows support is added, so layout bugs near the window chrome may be platform-specific rather than generic CSS issues.

## Output expectation

- State the repro steps, the root cause, the fix layer, and the exact validation rerun after the change.
- If the fix touched comments while clarifying behavior, keep them in professional English.
