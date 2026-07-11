# Queries and Commands

**Status:** design locked 2026-07-07 (C6/C7); grounded against the actual code state during the
docs reconciliation pass (contract.ts, `internal/catalog/asset_query.go`). The filter-bar /
pill / NL *UX* lives in the frontend-search-filter-ux epic record; this doc is the contract both
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
methods. Example: `sharpness` (the frontend-culling-signals epic record).

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
`frontend-architecture.md`; also flagged in the open-questions ledger
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

## Contract reconciliation — current state

Grounded 2026-07-07; reconciled through the seam round (2026-07-09/10) and the frontend
rebuild's read surface (2026-07-10). The contract's bones — surface grows with entities not
features; envelopes absorb field growth; one job envelope; binaries never cross the seam; codes
not strings; forward-compatible enum handling — are standing seam conventions.

**Where the reconciliation stands (truth: the code):**

- The Go seam surface (`internal/seam` services), the `ApiError` code catalog, the C9 job
  envelope, and the generated unions/models (`cmd/generate` → `_generated-types/`, `errors.ts`,
  `events.ts`, `models.ts`) are the reconciled reality — the flat `AssetFilter` is gone; queries
  travel as the AST triple (`query`, `arrangement`, `page`).
- `frontend/src/api/contract.ts` re-expresses the read surface as `AlexandriaAPI` over the AST
  query model, with the mock engine (`mock.ts`) as the SQL stand-in; hand-written provisional
  wire types are deleted as the generator emits their replacements (C13).
- Everything still pending has a trigger row in `../_project-tracking/DEFERRED.md` §7: the
  unbacked engine methods (tag management, folder tree, open-in, undo/redo, source removal,
  hard delete, presets, machine.json exposure), the `wailsjs/` method-binding regeneration +
  event-pump wiring (the `wails dev` pass), and thumbnail URL cache-busting (needs its asset
  handler; the URL must carry a content token — thumbnails regenerate in place at P2).
