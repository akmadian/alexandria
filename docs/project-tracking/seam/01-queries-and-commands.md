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

## Reconciliation ledger — contract.ts ↔ this design ↔ the engine

Grounded 2026-07-07. The contract's bones are **good** — its header conventions (surface grows
with entities not features; envelopes absorb field growth; one job envelope; binaries never cross
the seam; codes not strings; forward-compatible enum handling) all survive and are adopted as
standing seam conventions. Known deltas to apply in the seam round:

| # | Delta | Detail |
|---|---|---|
| 1 | `AssetFilter` (flat struct) → AST `where` | The flat optional-field filter is exactly the "flat pill row, implicit AND" subset. Evolve: `ListQuery.filter` becomes the boolean tree; the flat struct's fields become the v1 token vocabulary (they already match). Backend `catalog.AssetFilter` likewise: it currently conflates predicate + sort + paging in one struct — the query round splits it into query / arrangement / page per C1/C4. |
| 2 | `AssetScope` gains `{ kind: "tag"; id }` | Current contract + `deriveListQuery` treat tags as filter fields; the locked state model makes a sidebar tag selection a *scope* (durable, navigational). Tag-as-token also remains (filtering by tag within another scope). |
| 3 | `AssetSort` → `Arrangement` | Add grouping (group-by key) alongside sort field + direction, per C4. Sort fields keep the ingest/capture distinction (already present as `added`/`captured`). |
| 4 | `Settings` shape is stale | impl/11 made settings three JSON files and YAGNI-dropped `undoStackSize`, `catalogBackupCount`, `updateCheckEnabled`, `defaultSortField/Dir`; `xmpConflictResolution`/`xmpWriteBack` return with impl/06. Regenerate from `internal/settings` types when bindings land. |
| 5 | Keybindings are file-based | impl/11: `keybindings.json`, not catalog KV. `KeybindingContext` grows to `global/grid/loupe/compare/cull/import/review/palette` per `../frontend/04-keyboard-and-actions.md`. Preset-set selection is a new small surface. |
| 6 | `SourceStatus` → `enabled` + `connectivity` | Long-known pending change (impl/01 split the columns); models regeneration picks it up. |
| 7 | Job envelope reconciliation | See `02-events-jobs-and-binary.md` — contract adopts the engine's `TotalKnown` (spinner→bar upgrade); the seam envelope gains `label`, `state`, `cancelable`, optional `message` (C9). |
| 8 | `models/*.ts` retire | Generated from Go (C13). |
| 9 | Smart collection CRUD | New small surface: save/list/update AST-bearing collections (persisted with `version`). |
| 10 | Thumbnail URL cache-busting | Carried over from open question #5: URL must include a content token — thumbnails regenerate in place at P2 auto-refresh. |

Everything else in contract.ts (sources/tags/collections CRUD, folder tree, open-in, undo/redo
+ history events, error shape, binary URL builders) stands as designed.
