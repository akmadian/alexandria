# 45 — The bind: real engine behind the rail contract

**Areas:** seam, backend. **Blocked by:** 40-backend-volume-folder-schema.md,
41-backend-folder-tree-deriver.md, 42-seam-wire-shapes.md, 43-frontend-browser-rail.md,
44-frontend-manage-sources.md. The join of both tracks — it proves 43/44 against reality, so
it runs last.
**References:** D41 (removeFolder semantics; smart-count placement), C8/C9 (events/jobs
untouched — the rail rides `catalog`/`changed`), DEFERRED §7 (`removeSource` +
`pickDirectory` rows close here), §18 (the source-name inspector row unblocks — dated note),
`docs/seam-contract.md` (bound-surface ledger — update it here).

## Scope

- **Go seam services** wrapping tasks 40/41's engine behind task 42's contract:
  `getFolderTree` (deriver + counts), `listCollections` (with the smart-collection compiled
  COUNTs inside the read),
  `createFolder` (resolver + outcome), `removeFolder`, `updateFolder`; `boundServices()`
  rows; ApiError normalization; wails adapter + `models.ts` regen.
- **The `removeFolder` engine verb** — cascade-via-soft-delete in the **user-action writer**
  (judgment-class; ingest's observation writer structurally cannot do this), folder row
  deleted after, `RESTRICT` still guarding hard deletes.
- **Real `pickDirectory`** via the Wails runtime directory dialog (if still unminted).
- **Retire the contract's `listSources`** — consumers (import folder-pick, inspector) move to
  `getFolderTree`; DEFERRED §18's source-name row gets its dated resolution note.
- **E2E verification** (`wails dev`, real catalog): the 43/44 acceptance flows re-run against
  real data; count freshness through a real import; the §8 NFD case exercised through the UI
  path.
- Update `docs/seam-contract.md` bound-surface ledger + fold this epic's remaining residue.

## Out of scope

New UI (none — this task proves 43/44 against reality), watcher supervision (§2), undo for
removeFolder (the undo/history milestone; soft-delete keeps it recoverable-by-design until
then).

## Acceptance

- `make check` green both sides; contract served entirely by real services under `wails dev`
  (mock untouched, still passing its own suite).
- The full loop live: add folder → import → rail counts tick via `catalog/changed` → remove
  folder (confirm shows real count) → assets leave scopes; judgments survive in soft-deleted
  rows (verified in the DB).
- Smart-collection badge matches its query's `total` against the real catalog.
- Logging: every service call at Info with outcome, per-node at Debug.
