# Queries and Commands

**Status:** design locked 2026-07-07 (C6/C7); grounded against the actual code state during the
docs reconciliation pass (contract.ts, `internal/catalog/asset_query.go`). The filter-bar /
pill / NL *UX* lives in `../frontend/03-search-and-filter-ux.md`; this doc is the contract both
sides implement.

## The AST

The query as **typed structs**, never stringly key-value maps (KV can't express nesting, OR/NOT,
or two conditions on one field, and forfeits the compiler). A tree of boolean nodes over
predicate leaves:

```jsonc
// persisted form (smart collection); version field from day one (C6)
{
  "version": 1,
  "scope": { "kind": "collection", "id": "…" },        // extensional root; optional
  "where": { "op": "and", "children": [
    { "field": "type",   "cmp": "eq",  "value": "raw" },
    { "field": "rating", "cmp": "gte", "value": 3 },
    { "op": "not", "children": [ { "field": "tag", "cmp": "under", "value": "wip" } ] }
  ]}
}
```

Defined in Go, generated to TS (C13). The backend query-layer round (open question #4) builds the
one SQL compiler — the single query authority that `QueryAssets`, smart collections, system smart
collections (Untagged, Unrated, Suggested Rejects), and Review projections all reuse.

**Pattern lineage:** interpreter pattern (GoF). Living relatives: Prisma/Mongo filter objects,
Elasticsearch query DSL, JIRA JQL, Notion filters — and LrC smart collections internally. The
GraphQL comparison, settled: GraphQL solves client-chosen *result shape* and drags in transport/
middleware; we take only the filter-DSL half — client-chosen *predicate*, fixed result shape,
plain structs over Wails bindings.

## The token registry

A **token type** is one registry entry:

```
{ name, field, operators, value parser/validator, pill renderer, SQL compiler }
```

Frontend owns parse/render; backend owns compile. Registry rules and completeness enforcement per
C10. **Extension flow** (the whole point): new capability = new column at ingest/enrichment →
backend registers the compiler → frontend registers the token → it appears in the filter bar,
the smart collection editor, and the NL vocabulary *automatically*. Zero new views, zero new seam
methods. Example: `sharpness` (`../frontend/05-culling-and-signals.md`).

v1 vocabulary: filename, file type, tag (`under` for hierarchy), rating (exact + min), color
label, flag, capture/ingest date ranges, source, dimensions, camera make/model, file status,
absence queries (unrated/unflagged/unlabeled/untagged — triage filters on "what I haven't done
yet"), LrC-style metadata text fields (contains / starts-with / is-empty). FTS free-text is
itself a token (`text:`).

## Workhorse methods

| Method | Shape |
|---|---|
| `QueryAssets(query, arrangement, page)` | → `{ items: AssetRow[], total }`. Absorbs every predicate: browse, filter, search, smart collections, missing-files, sharpness thresholds. `total` sizes the grid scrollbar (COUNT strategy = query-round decision). |
| `UpdateAssets(target, patch)` | Sparse triage patch (rating/label/flag/note; absent = don't touch, null = clear — `Opt[T]` on the Go side, mirroring `catalog.TriagePatch`). Target = explicit ids or `{scope, filter}`; **destructive disk ops require ids, never a query** (existing contract rule, kept). |

**The rule (C7):** new method ⇢ new *result shape*, never new *predicate*. `GetSharpAssets` is
the smell; sharpness is a token. Structural methods (trees, CRUD, jobs control, Review actions,
settings) are justified precisely because their result shapes differ. Honest sizing: **30–50
typed methods** through P2 is healthy; the number that matters is shape stability, not count.

`AssetRow` stays the slim grid-card projection (~15 fields); full `Asset` is `getAsset` only.
It gains a `kind` discriminator when asset groups land (already anticipated in contract.ts).

## Additions from the 2026-07-08 frontend redesign round

Requirements the query/seam rounds must absorb (rationale in
`../frontend/09-ground-up-redesign-notes.md`; also flagged in `../backend/04-open-questions.md`
#4):

- **`AssetIDSlice(query, arrangement, fromIndex, toIndex) → []id`** — ids-only window over the
  compiled ordering (range-selection materialization).
- **`IndexOfAsset(query, arrangement, id) → index | null`** — cursor keep-if-present across
  query changes; cursor index remap across arrangement changes.
- **`UpdateAssets` target grows `exceptIds`**: `{ids} | {scope, where, exceptIds}` — compiled to
  ONE statement (`… AND id NOT IN (…)`), never an id-materialized loop.
- **Deterministic total order**: the compiler always appends a unique tiebreaker (`…, id`) to
  ORDER BY — index slices are meaningless without it.
- **Distinct-values lookup** for suggestable fields (camera make/model, …) — powers parser and
  editor suggestions.
- **Operator vocabulary includes negated forms** (`neq`, `notEmpty`, tag `lacks`/`not-under`):
  negation over a single leaf is an operator concern; tree `not` survives only over groups (the
  frontend assembler normalizes, so the compiler sees one canonical form per meaning).
- **Date values are `{anchor: date | "now", duration}`**, half-open intervals `[min, max)`;
  `"now"` resolves at **compile time** (rolling smart collections re-evaluate every open).
  Calendar-unit durations + timezone semantics for capture dates are query-round decisions.
- **Bulk-undo acceptance test**: triage patch on 300k assets, undone, redone — no perceptible
  stall (single-statement apply; batched-transaction restore; history byte budget).

## Reconciliation ledger — contract.ts ↔ this design ↔ the engine

Grounded 2026-07-07. The contract's bones are **good** — its header conventions (surface grows
with entities not features; envelopes absorb field growth; one job envelope; binaries never cross
the seam; codes not strings; forward-compatible enum handling) all survive and are adopted as
standing seam conventions. Known deltas to apply in the seam round:

| # | Delta | Detail |
|---|---|---|
| 1 | `AssetFilter` (flat struct) → AST `where` | The flat optional-field filter is exactly the "flat pill row, implicit AND" subset. Evolve: `ListQuery.filter` becomes the boolean tree; the flat struct's fields become the v1 token vocabulary (they already match). **Backend side DONE (impl/13):** `catalog.AssetFilter` deleted; replaced by `ast.Query` (predicate tree) + `ast.Arrangement` + `ast.Page` per C1/C4. Seam side (contract.ts) still pending. |
| 2 | `AssetScope` gains `{ kind: "tag"; id }` | Current contract + `deriveListQuery` treat tags as filter fields; the locked state model makes a sidebar tag selection a *scope* (durable, navigational). Tag-as-token also remains (filtering by tag within another scope). |
| 3 | `AssetSort` → `Arrangement` | Add grouping (group-by key) alongside sort field + direction, per C4. Sort fields keep the ingest/capture distinction (already present as `added`/`captured`). |
| 4 | `Settings` shape is stale | impl/11 made settings three JSON files and YAGNI-dropped `undoStackSize`, `catalogBackupCount`, `updateCheckEnabled`, `defaultSortField/Dir`; `xmpConflictResolution`/`xmpWriteBack` return with impl/06. Regenerate from `internal/settings` types when bindings land. |
| 5 | Keybindings are file-based | impl/11: `keybindings.json`, not catalog KV. `KeybindingContext` grows to `global/grid/loupe/compare/cull/import/review/palette` per `../frontend/04-keyboard-and-actions.md`. Preset-set selection is a new small surface. |
| 6 | `SourceStatus` → `enabled` + `connectivity` | Long-known pending change (impl/01 split the columns); models regeneration picks it up. |
| 7 | Job envelope reconciliation | **DONE (impl/16, 2026-07-10).** Go `JobProgress`/`JobDone` in `internal/seam/events.go` carry the C9 shape (engine `TotalKnown` spinner→bar upgrade + `label` i18n-key, `state`, `cancelable`, optional `message`). The topic/type/`JobState` unions are generated to `events.ts`; the payload *interfaces* stay hand-written in contract.ts until the wails-dev TS pass (DEFERRED §7) — the Go structs are shaped to match contract.ts so that pass is mechanical. |
| 8 | `models/*.ts` retire | Generated from Go (C13). |
| 9 | Smart collection CRUD | New small surface: save/list/update AST-bearing collections (persisted with `version`). |
| 10 | Thumbnail URL cache-busting | Carried over from open question #5: URL must include a content token — thumbnails regenerate in place at P2 auto-refresh. |

Everything else in contract.ts (sources/tags/collections CRUD, folder tree, open-in, undo/redo
+ history events, error shape, binary URL builders) stands as designed.

### impl/15 Phase 1 status (2026-07-09) — Go side bound; TS reconciliation + unbacked methods deferred

impl/15 landed the **backed** synchronous surface as thin `internal/seam` services
(`AssetService`, `CollectionService`, `SettingsService`, `SourceService`) + the `ApiError`
normalization layer (§4) with a generated `errors.ts` code catalog. Ledger disposition:

- **#1 `QueryAssets`** — bound; validates (`ast.Validate`) before the repo, maps `ErrVersionTooNew`
  → `query_version_too_new` and the grammar/value/structure errors → `query_invalid`. *(engine +
  Go seam done; contract.ts/TS side pending the wails pass)*
- **#3 `Arrangement`** — bound as `ast.Arrangement` (GroupBy still unimplemented per impl/13). *(done)*
- **#4 Settings** — bound over `internal/settings` types verbatim (whole-object `UpdateSettings`, not
  a partial patch — read-modify-write on the frontend; see decision log). `machine.json` **not**
  exposed (no UI — DEFERRED §7). *(Go side done)*
- **#5 Keybindings** — file-based get/set/reset bound; **presets deferred** (DEFERRED §7); conflict
  detection stays frontend-owned (backend never interprets chords). *(core done)*
- **#6 `SourceStatus` → `enabled`+`connectivity`** — the split model is generated; `SourceService`
  gained `Create`/`Update`; `SourcePatch` carries `enabled` (judgment), never connectivity. *(done)*
- **#9 Smart-collection CRUD** — `CollectionService` binds create/list/update/delete + membership over
  the impl/13 repo (which `ast.Validate`s smart queries; the service validates early for a clean code). *(done)*
- **#2 / #8 / #10 deferred:** #2 tag-as-scope rides `ast.ScopeKind` already, but tag *management* is
  unbacked (DEFERRED §7); #8 (`models/*.ts` retire) and the TS side of #1/#3 wait for the `wails dev`
  reconciliation pass; #10 thumbnail URL builders wait for their asset handler (unbacked — DEFERRED §7).

**§Additions — bound:** `AssetIDSlice`, `IndexOfAsset`, `DistinctValues` (validates field is known +
suggestable), and `UpdateAssets` with the `{ids} | {query, exceptIds}` target compiled to one
statement via `ApplyTriagePatchByQuery`. Bulk-undo/history-service verbs are **not faked** (deferred).

The full deferred list (unbacked engines + the TS reconciliation pass) is `../backend/impl/DEFERRED.md`
§7. Ledger row #7 (job envelope) belongs to impl/16.
