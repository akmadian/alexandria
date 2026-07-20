# 34 — Judgment writes: the mutation lane, inspector editors, triage keys

**Areas:** seam, frontend. **Blocked by:** nothing (runs parallel to 35 by decision; the
orchestrating session owns the one merge point — the optimistic row-patch helper rebases onto
block-shaped list caches when 35 lands first).

**References:** `docs/frontend-architecture.md` §Optimistic mutation × undo (LOCKED — implement
it verbatim, don't invent), `_project-tracking/DEFERRED.md` §7 (the `TriagePatchInput` wire
reconciliation — this task's seam phase is its named trigger), `epics/frontend-keyboard-actions.md`
(verb grammar; only the triage sliver lands here), `docs/seam-contract.md`. Binding law: C5
(targets = selection if non-empty, else cursor), C7 (UpdateAssets absorbs every triage write —
never a new method), C8 (the `catalog/changed` emit already exists; no ad-hoc events), C10
(actions registry completeness), C14 (all display text i18n), D8 (writer classes — the frontend
only ever calls the sanctioned user-action path).

## Scope

1. **Seam wire reconciliation.** `TriagePatchInput`'s raw-JSON three-state fields (absent =
   don't touch, null = clear, value = set) get their final generated-TS shape; `UpdateTarget` +
   `TriagePatchInput` join the generate manifest (C13/C15 — crosswalk row if the pattern fits).
   `contract.ts` gains `updateAssets(target, patch)`; mock and Wails adapter implement it
   (mock applies patches to seeded assets; unknown ids → `not_found`).
2. **The mutation lane.** The architecture record's locked discipline: ONE ordered FIFO for all
   catalog-editing calls; cancel-on-mutate + the invalidation gate (mark-stale while mutations
   are in flight, refetch at zero); optimistic cache patch for ids-targets (prior values saved,
   rollback on failure after 1–2 quiet retries); patches carry absolute values, never deltas.
   Both caches patch: the list cache and `["asset", id]`.
3. **Inspector editors.** The Judgment section's read-only Rating / label swatch / flag / note
   become interactive in place. Grid cells stay read-only — cell-face editing is not this round.
4. **Triage keys.** The first `actions/` registry sliver + grid-context dispatch: 0–5 rate
   (0 clears), 6–9 label (− clears), P pick / X reject / U clear. Palette, presets, rebindable
   keybindings, and the full context system stay in the epic.

## In-round rulings (carried from the round decision, 2026-07-20)

- **Ids-shaped targets only.** `all`-shaped / by-query targets stay gated until the undo round
  lands — no mass write ships without the net. The seam accepts the query form; the frontend
  does not send it yet.
- Failure is loud, never silent: rollback + a visible rendered surface (a minimal notice
  primitive is in scope if nothing suitable exists; mark its ceiling with `ponytail:`).

## Acceptance

- Rate / label / flag / note apply from the inspector AND from the keys, against the mock and
  the real catalog (`wails dev`); feedback is keystroke-speed (optimistic), and a forced failure
  visibly rolls back.
- Interleaved rapid writes settle in dispatch order (the lane is tested, not assumed).
- `make check` green; DEFERRED §7's TriagePatchInput paragraph closed by a dated note; i18n
  complete; fold-and-delete of this file prepared in the working tree (uncommitted).
