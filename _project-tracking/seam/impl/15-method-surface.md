# impl/15 — Seam Method Surface

**Status: spec ready (2026-07-09), not started.** Second seam-round doc; runs after impl/14,
in parallel with impl/16.

**Scope:** the synchronous request/response surface — `internal/seam` services wrapping the
catalog interfaces, the ApiError normalization, and the reconciliation of `contract.ts` against
the engine. **Claims reconciliation-ledger rows #1–6 and #8–10** (`../01-queries-and-commands.md`
§Reconciliation — check them off there in the same change); #7 belongs to impl/16.
**Blocked by:** impl/14. **Blocks:** the frontend rebuild.
**References (read FIRST, in order):** `../01-queries-and-commands.md` (THE contract this
implements — AST, workhorses, §Additions, ledger), `../../CONSTANTS.md` C6/C7/C13/C14,
`frontend/src/api/contract.ts` (the design artifact being reconciled — its header conventions
are standing law), `../../backend/impl/13-query-layer.md` §8 (the catalog surface this wraps),
`../../frontend/09-ground-up-redesign-notes.md` (consumer expectations: id-anchored selection,
optimistic-mutation rules).

## 1. The problem

`contract.ts` still speaks pre-AST (`AssetFilter`, `AssetSort`, hand-written `models/`, stale
`Settings`, thin jobs). The engine speaks `ast.Query`/`Arrangement`/`Page` through
`catalog.AssetReader`/`AssetJudgmentWriter`. This doc makes the TS side *generated from* and
*bound to* the Go side, method by method, retiring every hand-maintained parallel type.

## 2. Standing rules (adopted, not re-decided)

From contract.ts's header, now seam law: surface grows with **entities not features**; envelopes
absorb field growth; binaries never cross the seam (URL builders only); **codes cross the seam,
not strings** (C14); consumers tolerate unknown enum values. From C7: a new method needs a new
**result shape** — a method name containing a predicate is an AST leaf trying to escape.
**Destructive disk ops take `{ids}`, never a query** — enforce in the Go signature, not just TS.

## 3. The work, by ledger row

| # | Work |
|---|---|
| 1 | `ListQuery` dies → bound `QueryAssets(query, arrangement, page)` passing `ast.Query` (already the wire JSON shape). The Go seam method validates (`ast.Validate`) before touching the repo and maps `ErrVersionTooNew`/`ErrUnknownField{…}` to typed codes. |
| 2 | `Scope` already carries `tag` in `ast.ScopeKind` — generated types pick it up; no engine work. |
| 3 | `Arrangement` replaces `AssetSort` (GroupBy slot rides along, unimplemented per impl/13). |
| 4 | Settings surface regenerated from `internal/settings` types (three files; machine-scoped stays out of the catalog-settings method — decide the machine.json exposure minimally, it has no UI yet). |
| 5 | Keybindings: file-based via `settings.Keybindings`; contexts per `../../frontend/04-keyboard-and-actions.md` (`global/grid/loupe/compare/cull/import/review/palette`); preset-set selection = one small method pair (list presets, apply preset). |
| 6 | `SourceStatus` → `enabled` + `connectivity` — models regeneration picks up the impl/01 split; `SourcePatch` follows. |
| 8 | `frontend/src/models/` DELETED; contract.ts imports from `wailsjs` models + `_generated-types` only. |
| 9 | Smart-collection CRUD bound: create/list/update/delete AST-bearing collections over the impl/13 `collection_repo` (which already `ast.Validate`s on write). |
| 10 | Thumbnail/preview/original URL builders gain a content token (cache-busting). The URLs are served by the asset handler (impl/12 notes: `assetserver.Options`); this doc owns the URL *shape* + the token choice. |

**Plus the §Additions exposure** (engine side already built, impl/13): bind `AssetIDSlice`,
`IndexOfAsset`, `DistinctValues`, and `UpdateAssets` with the
`{ids} | {scope, where, exceptIds}` target compiled to one statement
(`ApplyTriagePatchByQuery`). Plus the rest of the existing contract surface, unchanged in shape:
sources/tags/collections CRUD, folder tree, open-in verbs, `startImport`/`cancelJob`,
undo/redo stubs (history *service* is a later milestone; bind the verbs against it when it
exists — don't fake them).

## 4. ApiError normalization

One mapping layer at the seam boundary (every bound method returns through it):

```
domain/catalog error        → { kind: "domain",     code: "not_found" | "keybinding_conflict" | … }
ast validation error        → { kind: "domain",     code: "query_invalid" | "query_version_too_new" | … }
dependency.Status missing   → { kind: "degraded",   code: "exiftool_missing" | … }
anything unrecognized       → { kind: "unexpected" } (logged at Error with stack; never a raw string across)
```

The code catalog is a constants file in `internal/seam`, exported through the generator so TS
switches on generated literals, not strings. Display text is frontend-owned (C14).

## 5. Decisions to make DURING implementation (pre-scoped)

| Decision | Recommendation |
|---|---|
| Bound-struct granularity | A few per-entity services (`AssetService`, `SourceService`, `TagService`, `CollectionService`, `SettingsService`) — Wails binds each; the frontend adapter composes them behind the single `AlexandriaAPI` interface. One 40-method struct is the alternative; per-entity keeps files and tests small. |
| `AssetRow` final field list | Already an impl/13 open item — finalize against the grid card (`../../frontend/01-flows-and-views.md`), err toward fewer. |
| Thumbnail content token | Recommend truncated content hash of the thumbnail file (stable across restarts, changes exactly when bytes change); mtime is the cheap fallback. |
| contract.ts disposition | Reconcile in place as the *shape record* (it stays design-authoritative for the rebuild), but don't rewrite the mock against it — the frontend rebuild owns its mock. |

## 6. Acceptance

- **C7 audit:** the final bound-method list contains no predicate-shaped names; reviewed
  against the ~30–50 budget.
- Per-method tests at the seam layer: happy path + error mapping (each repo error surfaces as
  its typed code, never a string match).
- `QueryAssets` round-trip from TS under `wails dev`: a hand-built AST JSON returns correct
  rows; an invalid tree returns `query_invalid`; a `version+1` tree returns
  `query_version_too_new`.
- Destructive-op rule: `deleteFromDisk` signature takes ids only (uncompilable otherwise).
- `frontend/src/models/` is gone; frontend typecheck passes against generated types alone.
- Ledger rows #1–6/#8–10 checked off in `../01-queries-and-commands.md` (same change).

## 7. Doc maintenance on landing (same change)

- `../01-queries-and-commands.md`: ledger rows marked done; §Additions marked bound.
- Master head status table; `../00-START-HERE.md` sequencing.
- `../../backend/02-decision-log.md`: entries for §5 resolutions.
- This file: status block → shipped + deviations.
