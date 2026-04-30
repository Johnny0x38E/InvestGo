---
name: investgo-debugging
description: Investigate InvestGo regressions across Vue, Go, Wails runtime, market data providers, build scripts, and packaging scripts. Use when requests mention debugging, bug fixes, regressions, broken quote or history refresh, startup failures, API mismatches, build failures, packaging failures, or platform-specific problems.
---

# InvestGo Debugging

## Workflow

1. Read `references/debugging-playbook.md` before changing code.
2. Reproduce the issue with the smallest command or UI path possible. Do not start by refactoring.
3. Classify the failure first: frontend render or state, API contract, store runtime, overview analytics, market data provider, build chain, packaging chain, or platform-specific behavior.
4. Collect evidence before editing. Prefer logs, failing requests, exact command output, and a narrow repro over speculation.
5. Fix the smallest layer that explains the symptom. If the root cause is contract drift, update both sides together instead of patching around it.
6. Re-run the minimal reproducer and the tightest relevant validation commands after the fix.
7. If the issue reveals a future Windows x64 blocker rather than a current runtime bug, say so explicitly and point to the blocking macOS-only assumption.
8. When the bug involves user-visible copy, locale mismatches, or missing translations, treat `frontend/src/i18n.ts` parity as part of the fix instead of a follow-up.
9. If debugging touches comments while clarifying a fix, keep the touched comments in professional English only.

## Validation

- Re-run only the smallest relevant checks from `AGENTS.md` that cover the failing layer.
- Use a dev-mode run or dev build only when the issue depends on logs or DevTools behavior.

## References

- Use `references/debugging-playbook.md` for symptom triage, logging hooks, and platform-specific failure patterns.
