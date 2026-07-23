# 44 — Manage sources: add / remove / sync-mode

**Areas:** frontend. **Blocked by:** 42-seam-wire-shapes.md. Parallel to 43.
**References:** D41 (graceful-merge outcomes; cascade-via-soft-delete confirm; sync_mode
ships), D24 (never mutate user files), C5 (targets displayed explicitly), C14, DEFERRED §1
(the guardrail paragraph gets its dated note when this lands), §2 (watcher supervision
rider — recorded, not solved here).

## Scope

The epic's eponymous surface — folder tracking management. If frontend-import's minimal add
form exists by execution time, this supersedes it; if not, that epic reuses this.

- **Add folder:** `pickDirectory` → `createFolder`, then render the outcome (D41 — never a
  bare error):
  - `created` → select the new root in the rail.
  - `alreadyTrackedWithin` → "already tracked, inside *X*" → navigate/select *X*. (The
    exact-duplicate case returns self — copy reads "already tracked", no "inside".)
  - `absorbed` → QUIET, like LrC's Add Parent Folder: no dialog; the rail simply shows the
    new combined tree (select the new root so the result is visible in place).
  - `needsConfirmation` → the one dialog in this flow, only when sync behavior would change:
    name the watched/scheduled folder and what changes ("'2024' is watched; combining under
    'Photos' (manual) stops watching it — continue?"), then re-call with confirm.
- **Remove folder:** confirm showing the asset count and the semantics in plain copy —
  catalog rows soft-deleted with judgments preserved, **files on disk untouched** (D24);
  then `removeFolder`.
- **Sync mode:** the three-way control (manual / watched / scheduled) per tracked root via
  `updateFolder`. Watched carries the §2 rider honestly in copy if a watch dies (no false
  "all good" state).
- All copy i18n (C14); every destructive/absorbing action shows its target explicitly (C5).

## Out of scope

Per-subtree sync overrides (DEFERRED §19), volume removal (volumes are derived away when
empty — D41), scheduled-sync scheduling UI beyond the mode choice (poll interval stays a
settings-level knob), watcher supervision (§2).

## Acceptance

- Against the mock: all four add outcomes exercisable end-to-end, each resolving per D41
  (created selects; redirect selects the existing; quiet absorb just shows the merged tree;
  the behavior-change case shows the one dialog, and confirm/cancel both behave).
- Remove: confirm displays the correct count; after confirm the folder leaves the rail and
  its assets leave every scope; cancel is a true no-op.
- Sync-mode change round-trips through the contract and re-renders the root's state.
- Forced failure paths (picker cancelled, createFolder error) visibly handled.
