# 26 — Row + PanelSection: the structural primitives

**Areas:** frontend. **Blocked by:** 24-frontend-design-library-button.md.
**References:** `docs/design-constitution.md` §8 (row density is an intent), §12 (the one
row grammar), §13 (truncation/counts); `frontend/design/registries.json` `rowIntents`;
`frontend/CLAUDE.md` §6; C10.

The structural decision from the 2026-07-17 review: no generic Stack/Box layout zoo — the
app frame is one fixed §12 zone grid, free-form spacing is already gated by stylelint's
token rule, and the piece worth a component is the **registered row grammar**, which most
systems don't have and ours already tokenized. These two primitives make it true by
construction. (Trigger to revisit the generic-Stack question: feature CSS sprawling
near-identical ad-hoc flex containers.)

## Deliverables

1. **Row** (`components/row/`): `intent: control | list | text` binding — by construction,
   not convention — the height token (`row-control` 28 / `row-list` 24 / `row-text` 16),
   the shared `row-inset`, and the intent's permitted type roles per `rowIntents`
   (violations unrepresentable in the API, or lint-caught; body type in a text row must
   not be expressible). `text` rows are non-interactive by type; interactive intents meet
   the hit-target floor. Slots for the §12 grammar: label left, value/count right (mono,
   muted, tabular).
2. **PanelSection** (`components/panel-section/`): section head (sentence-case bold at
   body size, §9) + disclosure chevron + a run of Rows; density switches at the section
   boundary only (§8) — the section, not the row, chooses the intent.
3. Both land their matrices in the design library (all intents × states × themes), per the
   ratified build method.
4. **Acceptance:** `make check` green; matrices verified in the real browser on all four
   themes; truncation behavior per §13 (end-truncate names; counts tabular, exact ≤4
   digits).

## Non-goals

The source tree (RAC Tree, its own round), inspector features, any consumer wiring. After
this task the leaf ladder resumes (ToggleButton → Checkbox → Switch → field), one primitive
per round.
