---
name: investgo-frontend-dev
description: Implement and modify InvestGo Vue plus TypeScript frontend features, API clients, composables, dialogs, and module views. Use when requests involve frontend development, UI behavior, state flow, data fetching, charts, settings, PrimeVue integration, or when editing frontend/src/**/*.vue, frontend/src/api.ts, frontend/src/forms.ts, frontend/src/format.ts, frontend/src/types.ts, or frontend/src/style.css.
---

# InvestGo Frontend Dev

## Workflow

1. Read `references/frontend-context.md` before changing the client.
2. Start from the contract and shared helpers first. Check `types.ts`, `api.ts`, `forms.ts`, and `format.ts` before touching view code.
3. Keep orchestration in `App.vue`. Extract reusable behavior into components or composables instead of bloating a single module.
4. Use strict TypeScript and PrimeVue components. Keep formatting logic out of templates and preserve the existing status and refresh flows.
5. Preserve request timeout, cancellation, and client logging behavior in `frontend/src/api.ts` unless the task explicitly changes networking semantics.
6. If an endpoint or state shape changes, update the matching Go snapshot or API contract in the same change. Do not let frontend and backend drift, especially across `StateSnapshot`, `OverviewAnalytics`, and history payloads.
7. Remember that the UI runs inside a Wails desktop webview. Avoid browser assumptions that do not make sense for desktop unless the app already relies on them.
8. Any user-visible copy change must update both Simplified Chinese and English entries in `frontend/src/i18n.ts`. Do not ship one-sided locale changes.
9. New or revised comments in Vue, TypeScript, and CSS must be professional English. Translate touched Chinese comments instead of leaving mixed-language notes behind.

## Validation

- Use the standard frontend and backend checks from `AGENTS.md`.
- When contracts or snapshot shapes change, verify both sides in one pass instead of broad repeated validation.

## References

- Use `references/frontend-context.md` for file ownership, state flow, endpoint touchpoints, and desktop-platform notes.
