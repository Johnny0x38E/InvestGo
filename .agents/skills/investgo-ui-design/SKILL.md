---
name: investgo-ui-design
description: Design and refine InvestGo desktop UI, interaction flows, module layouts, theming, and visual polish for the Wails plus Vue application. Use when requests involve redesign, UI polish, layout, spacing, typography, theme changes, dialog flow, dashboard presentation, or when editing frontend/src/App.vue, frontend/src/components/**, frontend/src/style.css, or frontend assets.
---

# InvestGo UI Design

## Workflow

1. Read `references/ui-context.md` before proposing or implementing UI changes.
2. Inspect the touched shell, module, or dialog first. `App.vue` owns orchestration, `components/modules/` own feature surfaces, and `components/dialogs/` own focused edit flows.
3. Treat InvestGo as a desktop monitoring tool, not a generic website. Favor dense but readable information, short action paths, and clear price and status hierarchy.
4. Reuse PrimeVue controls and existing CSS variables before adding one-off styling. Change global tokens in `frontend/src/style.css` before scattering local overrides.
5. Preserve theme and typography plumbing through root `data-*` attributes. Do not break `themeMode`, `colorTheme`, `priceColorScheme`, or `fontPreset`.
6. Handle window-chrome-sensitive layouts carefully. The current shell reserves macOS title-bar space; avoid baking in assumptions that would block a later Windows x64 shell.
7. Keep changes local and shippable. Do not invent a new design system unless the user explicitly asks for a full redesign.
8. If the design change affects user-facing copy or labels, update both Simplified Chinese and English entries in `frontend/src/i18n.ts`.
9. Any new or revised explanatory comments in Vue or CSS must be professional English.

## Deliverables

- Explain the user-facing problem being solved and the views affected.
- Call out changes to readability, information density, or interaction cost.
- Mention follow-up work if a visual change depends on backend or API changes.

## Validation

- Run the standard frontend checks from `AGENTS.md`.
- Add a visual pass or contract check only when the change touches shell layout, assets, or backend-driven UI state.

## References

- Use `references/ui-context.md` for module ownership, visual tokens, shell constraints, and Windows x64 preparation notes.
