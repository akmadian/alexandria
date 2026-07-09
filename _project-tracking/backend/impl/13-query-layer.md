# impl/13 — Query Layer (the AST → SQL compile authority)

**Status: ✅ DONE (2026-07-08).** Designed and built same day. `AssetFilter`/`List`/`buildFilterSQL`/`sortColumns` deleted; tests migrated.

**Scope:** new `internal/ast` (the whole query authority: grammar types, vocabulary, validation,
JSON wire form, SQL compilation); `internal/catalog` (interfaces grow the query/command surface;
`AssetFilter` is DELETED — verified zero production callers, tests only); `internal/sqlite`
(repo methods execute compiled statements; `buildFilterSQL` + `List` + `sortColumns` are absorbed
and deleted; new `collection_repo.go`; the `TODO(fts)` at `tag_repo.go:96`).
**Blocked by:** nothing. **Blocks:** the seam round (TS generation reads `internal/ast`), which
blocks the entire frontend rebuild; smart collections; the undo implementation; search that
includes keywords.
**References:** `../../seam/01-queries-and-commands.md` (the AST spec + §Additions — read it
FIRST, it is the contract this round implements), `../../frontend/09-ground-up-redesign-notes.md`
(§Token & AST drill-in — locked vocabulary and rationale), `../04-open-questions.md` #4,
`../../CONSTANTS.md` C6 (versioned AST), C7 (new method ⇢ new result shape, never new predicate),
C10 (registries dispatch, conditionals don't), C13 (Go types generate TS).

**Coding standards for this round (Ari, 2026-07-08, verbatim intent):** "Now is the time to NOT
be lazy in our design and implementation. The standards we set and abide by NOW will shape the
code long into the future." Full identifiers (no abbreviations beyond the repo's allowed set:
`i/j/k`, `err/ctx/ok/id/db/tx`, short receivers). Comprehensive logging: milestones/results at
Info, per-item at Debug, never error-only. Exhaustive tests are IN SCOPE, not gold-plating —
this package is the single query authority for the life of the product.

## 1. The problem

Today the backend answers exactly one shape of question: flat AND of simple conditions
(`catalog.AssetFilter` → `buildFilterSQL`). No OR, no NOT, no nesting, no two conditions on one
field — and the struct conflates predicate + sort + paging, violating the C1/C4 split the whole
frontend design is built on. Meanwhile the query AST is fully *designed* (seam/01): versioned,
typed, nested boolean tree over predicate leaves. Nothing compiles it. This round builds the ONE
compiler that `QueryAssets`, smart collections, system smart collections (Untagged, Unrated,
later Suggested Rejects), and Review projections all reuse — so a new filterable capability is
one registry entry, never a new query method (C7).

## 2. Locked decisions (2026-07-08, Ari — do not relitigate)

1. **`internal/ast` is its own package** — not `catalog` (that's the repository contract), not
   `domain` (that's what an asset *is*; the AST is how you *ask*). Precedent checked online:
   Go stdlib (`go/token` ← `go/ast` ← `go/parser`/`go/printer`) and cel-go (`common/ast` ←
   `checker` ← `interpreter`) both make the tree package the bottom of a consumer stack.
2. **The SQL compiler lives INSIDE `internal/ast`** (`compile.go`, exported `CompileToSQL` family)
   — the whole query authority in one package, files carve it up for readers. The
   dialect-agnosticism argument was considered and rejected: SQLite is load-bearing for this
   project ("tearing it out would basically mean a complete bottom up redesign of the entire
   backend" — Ari); a second dialect is flexibility we have explicitly decided never to use.
   Wails is irrelevant to this placement: TS is generated only from types reachable through
   *bound method signatures*; `CompileToSQL` is never bound and is invisible to the seam.
3. **Sealed interface for the node union** — `ast.Node` with an unexported marker method;
   `ast.Group` and `ast.Leaf` are the only implementations. Illegal states unrepresentable.
   (Rejected: one struct with both field sets + validate.)
4. **Vocabulary discipline** — `Token` is NOT a Go type. The locked triad (frontend/09): token =
   a *definition* (registry entry — lives frontend-side), leaf = an *instance* (AST node), pill =
   the *rendering*. The backend holds the shared spine: `ast.Field`, `ast.Operator`, and the
   per-field grammar in `vocabulary.go` ("tokens are the vocabulary, leaves are sentences").
   The seam round generates the TS `TokenField`/`TokenOperator` literal unions FROM
   `ast.Field`/`ast.Operator` — the generator maps the names; both sides keep their idiomatic form.
5. **`ast` may import `domain`, minimally** — the grammar (types, vocabulary, JSON, structural
   validation) stands alone on stdlib; `domain` appears ONLY where a leaf's value is checked
   against a catalog fact (enum membership: is `"raw"` a `domain.FileType`). The invariant to
   hold: if `domain` shows up in `types.go` or `json.go`, something is mislayered. Membership
   borrowed, never redeclared (redeclaring enum members in `ast` = silent drift when domain grows).
6. **Everything in `ast` is pure** — pure functions, deterministic, zero I/O, zero DB handle.
   `now` is a *parameter* to compilation, never `time.Now()` inside. This is what buys
   exhaustive testing.

## 3. The package

```
internal/ast/
  types.go        — Query, Scope, sealed Node (Group, Leaf), Arrangement, Page
  vocabulary.go   — Field + Operator constants; the per-field grammar table
  value.go        — value kinds; DateValue{Anchor, Duration}
  json.go         — wire round-trip: union dispatch, version gate
  validate.go     — structural + vocabulary + value validation
  compile.go      — CompileToSQL family + the per-field compiler registry (C10)
  *_test.go       — see §10 Acceptance
```

Dependency arrows, final: `sqlite → ast → domain` (and `catalog → ast` for method signatures).
Never backwards. `ast` imports domain + stdlib, nothing else.

## 4. types.go — the grammar

Wire shape is the seam/01 contract, restated:

```jsonc
{
  "version": 1,
  "scope": { "kind": "collection", "id": "…" },   // optional; absent = all
  "where": { "op": "and", "children": [
    { "field": "fileType", "cmp": "eq", "value": "raw" },
    { "field": "rating",   "cmp": "gte", "value": 3 },
    { "op": "not", "children": [ { "field": "tag", "cmp": "under", "value": "…tagID…" } ] }
  ]}
}
```

```go
package ast

const Version = 1 // C6: version field from day one; forward-only migrations ride catalog migrations

type Query struct {
    Version int
    Scope   *Scope // nil = everything
    Where   Node   // nil = no predicate (scope-only browse)
}

type Scope struct {
    Kind ScopeKind // all | collection | source | tag  (tag scope: seam ledger #2)
    ID   string    // empty for ScopeAll
}

// Node is the sealed predicate-tree interface. Only Group and Leaf implement it
// (unexported marker method) — the compiler's type switch is exhaustive by construction.
type Node interface{ isNode() }

type Group struct {
    Op       GroupOp // and | or | not
    Children []Node  // not: exactly one child, and it must be a Group (see §6 normalization rule)
}

type Leaf struct {
    Field Field
    Cmp   Operator
    Value any // wire-typed: string | float64 | bool | []string | DateValue — validated per value kind
}

// Arrangement and Page are the C1/C4 split made physical — AssetFilter's conflation, fixed.
type Arrangement struct {
    SortField SortField // captured | added | rating | filename | size (absorbs sqlite.sortColumns)
    SortDir   SortDir   // asc | desc
    // GroupBy lands with the grouping deep-dive (open question #7); the field slot is the
    // seam-visible anticipation (seam ledger #3), zero implementation now.
}

type Page struct {
    Limit  int
    Offset int
}
```

`Leaf.Value` as `any` is deliberate: full per-field typed leaves were already rejected in the
frontend round (frontend/09 — "a truly strict leaf is a parser," registries validate uniformly);
the same reasoning holds in Go. Strictness lives in `validate.go` + `compile.go`, which check
value kind and enum membership before any value reaches SQL — and values NEVER reach SQL as text,
only as bound parameters (§7).

## 5. vocabulary.go — fields, operators, the grammar table

```go
type Field string    // generates TS TokenField
type Operator string // generates TS TokenOperator
type ValueKind string

// fieldSpec is the grammar half of a token definition (the frontend registry holds the
// UX half: editors, formatters, aliases). Compile strategies live in compile.go's
// registry, NOT here — vocabulary answers "is this leaf grammatical?",
// compile answers "what SQL is it?".
type fieldSpec struct {
    Operators []Operator
    Kind      ValueKind
}

var vocabulary = map[Field]fieldSpec{ … } // the single grammar authority
```

**v1 field × operator table** (assembled from seam/01 v1 vocabulary + §Additions negated forms;
this table is the authoritative draft — finalize it in code with the completeness tests of §10):

| Field | Kind | Operators |
|---|---|---|
| `filename` | text | `contains`, `startsWith`, `eq`, `neq` |
| `fileType` | enum | `in`, `notIn` |
| `rating` | numeric | `eq`, `neq`, `gte`, `lte`, `empty`, `notEmpty` |
| `colorLabel` | enum | `in`, `notIn`, `empty`, `notEmpty` |
| `flag` | enum | `in`, `notIn`, `empty`, `notEmpty` |
| `tag` | tagReference | `has`, `lacks`, `under`, `notUnder`, `empty`, `notEmpty` |
| `capturedAt` | dateRange | `within`, `notWithin`, `empty`, `notEmpty` |
| `ingestedAt` | dateRange | `within`, `notWithin` |
| `source` | entityReference | `in`, `notIn` |
| `width`, `height` | numeric | `eq`, `gte`, `lte` |
| `cameraMake`, `cameraModel` | text | `eq`, `neq`, `contains`, `empty`, `notEmpty` |
| `lensModel`, `title`, `caption`, `creator`, `copyright` | text | `contains`, `startsWith`, `eq`, `empty`, `notEmpty` |
| `fileStatus` | enum | `in`, `notIn` |
| `text` | freeText | `matches` (FTS5) |

Notes carried from the design rounds:
- **`in`/`notIn` on enum kinds** — a multi-select enum pill is ONE leaf with a list value, not an
  OR-group (matches the frontend's per-kind enum editor; also exactly what `AssetFilter`'s slice
  fields expressed).
- **Absence = `empty`/`notEmpty` operators on base fields**, never separate absence fields —
  frontend parses "unrated" → `rating empty` (frontend/09 locked this; system smart collections
  Untagged/Unrated are stored one-leaf queries).
- **Negated operators exist at the leaf level BY DESIGN** (`neq`, `notIn`, `notEmpty`, `lacks`,
  `notUnder`, `notWithin`) alongside tree-level `not` over groups. The frontend assembler
  normalizes `not`-wrapping-a-single-negatable-leaf INTO the negated operator, so the compiler
  sees one canonical form per meaning — but the compiler must still handle raw `not` groups
  (persisted queries, hand-built callers).
- **Rating `empty` vs `eq 0`** are distinct: NULL = never rated, 0 = rated zero (schema CHECK
  allows both).

## 6. value.go, json.go, validate.go

**value.go** — the ~7 value kinds (mirrors frontend/09): `enum`, `numeric`, `dateRange`, `text`,
`tagReference`, `entityReference`, `freeText`. Plus the one structured value:

```go
// DateValue: {anchor, duration} half-open interval [anchor, anchor+duration) — or
// [anchor+duration, anchor) when duration is negative ("last 30 days" = anchor "now",
// duration -30d). AnchorNow resolves at COMPILE time, never parse time — this is what
// makes a stored "last 30 days" smart collection roll (frontend/09, locked).
type DateValue struct {
    Anchor   DateAnchor // concrete date OR AnchorNow
    Duration DateDuration
}
```

Calendar-unit durations and timezone semantics are §9 decisions.

**json.go** — custom `MarshalJSON`/`UnmarshalJSON` for the `Node` union: presence of `"op"`
dispatches Group, presence of `"field"` dispatches Leaf, BOTH or NEITHER is an error (never a
guess). `Query` unmarshal gates on `Version`: greater than `ast.Version` = a typed
`ErrVersionTooNew` (the caller decides UX); missing/zero = malformed. Unknown JSON keys rejected
(`DisallowUnknownFields`) — a typo'd hand-written query must fail loudly, not half-apply.

**validate.go** — `Validate(query) error`, pure, three layers: structure (`not` arity = exactly 1
and child is a Group — leaf negation is an operator concern, §5; empty `and`/`or` groups
rejected; depth cap as a sanity bound), grammar (field exists in vocabulary, operator allowed for
field), value (kind-shape match: numeric gets float64, enum/entity get string-or-[]string per
operator arity, dateRange gets DateValue). Enum *membership* (is `"raw"` a real
`domain.FileType`) is validated here too — this file and `compile.go` are the ONLY two places
`domain` may appear (§2.5). Errors are structured (`ErrUnknownField{Field}`, …) so the seam can
map them to codes, not strings.

## 7. compile.go — the authority

One internal core, a small exported family (all pure; `now` always a parameter):

```go
type Statement struct {
    SQL  string
    Args []any
}

// The workhorse: SELECT <AssetRow columns> … ORDER BY … LIMIT/OFFSET.
func CompileSelect(query Query, arrangement Arrangement, page Page, now time.Time) (Statement, error)

// total for the grid scrollbar — §9 decision picks the strategy.
func CompileCount(query Query, now time.Time) (Statement, error)

// ids-only window over the compiled ordering (range-selection materialization — seam §Additions).
func CompileIDSlice(query Query, arrangement Arrangement, fromIndex, toIndex int, now time.Time) (Statement, error)

// position of one asset in the compiled ordering (cursor keep-if-present / index remap).
// ROW_NUMBER() OVER (ORDER BY …) in a subquery, WHERE id = ?.
func CompileIndexOf(query Query, arrangement Arrangement, id string, now time.Time) (Statement, error)

// WHERE fragment + args for query-shaped UpdateAssets targets: the caller owns the UPDATE's
// SET half (sqlite already has buildTriageSQL); exceptIDs compiles to `AND id NOT IN (…)`
// in the SAME statement — never an id-materialized loop (seam §Additions, locked:
// "we don't want that storm of 300k+ asset patches").
func CompileWhere(query Query, exceptIDs []string, now time.Time) (Statement, error)

// distinct non-null values for suggestable fields (cameraMake, cameraModel, …) — powers
// parser/editor suggestions. Errors on fields not marked suggestable in the vocabulary.
func CompileDistinctValues(field Field) (Statement, error)
```

**The per-field compiler registry (C10)** — `map[Field]fieldCompiler`, one entry per vocabulary
field; a completeness test (§10) fails the build if vocabulary and registry ever diverge. The
tree walk is a type switch on the sealed `Node`; fragments compose with parenthesized
`AND`/`OR`/`NOT`. Strategies, with the schema facts verified against `0001_initial_schema.sql`:

| Field family | Strategy |
|---|---|
| column comparisons (rating, fileType, dims, camera, metadata text, fileStatus, source) | direct predicate on `assets`; `in` = `IN (?,…)`; text `contains`/`startsWith` = `LIKE` with escaped pattern |
| `tag has`/`lacks` | `EXISTS (SELECT 1 FROM asset_tags WHERE asset_id = assets.id AND tag_id = ? AND removed_at IS NULL)` — tombstones are load-bearing, never forget `removed_at IS NULL` |
| `tag under`/`notUnder` | subtree via materialized path: `EXISTS (… asset_tags JOIN tags ON … WHERE tags.path GLOB (SELECT path FROM tags WHERE id = ?) || '*' AND removed_at IS NULL)` — GLOB not LIKE, that's what `idx_tags_path` serves (schema comment) |
| `tag empty` (untagged) | `NOT EXISTS (SELECT 1 FROM asset_tags WHERE asset_id = assets.id AND removed_at IS NULL)` |
| `rating`/`colorLabel`/`flag`/`capturedAt`/metadata `empty` | `IS NULL` (per-field: absence compiles per-column, there is no generic absence) |
| `text matches` | `id IN (SELECT asset_id FROM assets_fts WHERE assets_fts MATCH ?)` — sanitize/quote the user string for FTS5 query syntax (a stray `"` must not be a syntax error) |
| dateRange `within` | resolve DateValue at compile time → `col >= ? AND col < ?` (half-open) |

**Always injected:** `is_deleted = 0` (the compiler owns it; no leaf can express deleted-state in
v1 — Review's deleted views get a scope or operator when they need one, not a backdoor).

**Scope compilation:** `all` = nothing; `source` = `source_id = ?`; `tag` = same subtree EXISTS as
`tag under`; `collection` (manual) = `EXISTS (SELECT 1 FROM collection_assets WHERE …)`. A SMART
collection as scope cannot be compiled purely (its stored AST lives in the DB) — the CALLER
resolves it first: fetch stored query, splice via a pure helper
`ast.MergeScope(outer Query, storedWhere Node) Query` (AND-composition), then compile. The
compiler errors on an unresolved smart scope rather than guessing.

**ORDER BY:** arrangement maps through the sort-field table (absorbing `sqlite.sortColumns` —
delete it there); the compiler ALWAYS appends `, id` as the unique tiebreaker (seam §Additions:
index slices are meaningless without deterministic total order). NULL `captured_at` fallback per
§9.

**Parameterization property:** no user-supplied value is EVER interpolated into SQL text — values
travel only in `Args`. Identifiers (columns, tables) come only from compiled-in strategy code and
the sort whitelist. §10 has a test enforcing this.

## 8. The surface: catalog interfaces + sqlite implementations

`AssetFilter`, `AssetReader.List`, `buildFilterSQL`, `sortColumns` — DELETED (zero production
callers; migrate the repo tests to the new methods).

```go
// catalog — new projection beside PathStatus (repo projections live in catalog, not domain):
// AssetRow is the slim grid-card projection (~15 fields: id, filename, fileType, fileStatus,
// rating, colorLabel, flag, width, height, aspectRatio, capturedAt, sourceID, thumbnail
// presence, …). Full *domain.Asset stays Get-only (seam/01). Exact field list: finalize
// against frontend/01-flows-and-views.md grid card during implementation.
type AssetRow struct { … }

// TriageState is the prior-state projection undo captures (frontend/09 undo design:
// before-images for value writes).
type TriageState struct {
    ID         string
    Rating     *int
    ColorLabel *domain.ColorLabel
    Flag       *domain.Flag
    Note       *string
}

// AssetReader grows (C7-clean: each is a new result SHAPE):
QueryAssets(ctx, query ast.Query, arrangement ast.Arrangement, page ast.Page) ([]AssetRow, int, error) // rows, total
AssetIDSlice(ctx, query ast.Query, arrangement ast.Arrangement, fromIndex, toIndex int) ([]string, error)
IndexOfAsset(ctx, query ast.Query, arrangement ast.Arrangement, id string) (*int, error) // nil = not in set
DistinctValues(ctx, field ast.Field) ([]string, error)
ReadTriageStates(ctx, ids []string) ([]TriageState, error) // undo's before-image read

// AssetJudgmentWriter grows the query-shaped target (ids form already exists):
ApplyTriagePatchByQuery(ctx, query ast.Query, exceptIDs []string, p TriagePatch) (affectedIDs []string, error)
```

Implementation notes:
- Each sqlite method: `ast.Compile*` → execute → scan. `QueryAssets` runs select + count (per §9
  strategy). Log at Info: query compiled (field count, scope kind, duration, row count); at
  Debug: the SQL text + arg count (never arg *values* — they're user content).
- `ApplyTriagePatchByQuery` = ONE `UPDATE assets SET … WHERE <CompileWhere>` reusing
  `buildTriageSQL` for SET, inside `InTx` with the `RETURNING id` (or pre-read) needed to hand
  undo its affected set + `ReadTriageStates` before-images. This round ships the PRIMITIVES
  (single-statement apply, bulk prior-state read, batched restore path); the undo history
  service itself is a later milestone.
- **Smart-collection evaluation** = load `collections.query` JSON → `ast` parse+validate →
  `MergeScope` → same `QueryAssets` path. System smart collections (Untagged, Unrated) are
  code-constructed one-leaf queries through the identical path — no special cases.

**Collections CRUD** (`sqlite/collection_repo.go`, implementing the existing
`catalog.CollectionRepository` — declared with zero implementations until now): straightforward
CRUD against `collections`/`collection_assets` (schema exists, including `position` for manual
ordering — `AddAsset` appends `MAX(position)+1`). `Create`/`Update` of a smart collection MUST
`ast.Validate` the query JSON and reject invalid trees — never persist garbage. Emit the C8
collection-changed events when the event bus lands (note the hook point; don't build the bus).

**FTS⋈tags slice** (the `TODO(fts)` at `sqlite/tag_repo.go:96`): recompose `assets_fts.tags` for
an asset whenever its tag set changes — space-joined display names of ACTIVE (`removed_at IS
NULL`) tags, including ancestor names for hierarchical hits ("wedding" matches assets tagged
"weddings/2026"). Wire into `ImportKeywords` now (`SetAssetTags` inherits it with impl/10);
extend `RebuildFTS` to backfill. The FTS tier this round builds is its first consumer — land it
here so `text:` matches keywords the day search ships.

## 9. Decisions to make DURING implementation (all pre-scoped, none blocking start)

| Decision | Options | Recommendation |
|---|---|---|
| COUNT strategy for `total` | separate `CompileCount` query vs `COUNT(*) OVER()` window per page | **Separate count.** The frontend's infinite row model fetches total once per (query, arrangement) change and caches; paying the window function on every block fetch taxes the hot path to save one round-trip on the cold one. |
| NULL `captured_at` sort fallback (open question #2) | `COALESCE(captured_at, mtime)` vs NULLS LAST vs plain NULL ordering | **`COALESCE(captured_at, mtime)`** — a file with no EXIF still has a meaningful date; matches LrC behavior. Requires an expression index (`ON assets(COALESCE(captured_at, mtime)) WHERE is_deleted = 0`) replacing `idx_assets_captured_at` — edit `0001_initial_schema.sql` in place per the repo's pre-1.0 convention. |
| Calendar-unit durations | fixed-length units only vs calendar-aware months/years | **Calendar-aware via `time.AddDate`** — "last 3 months" meaning "90 days" is a lie users notice; Go does the calendar math for free at compile time. |
| Timezone for date boundaries | UTC vs machine-local | **Machine-local for resolving `AnchorNow` and day boundaries** ("today" means the user's today), converted to the stored representation (UTC RFC3339 — match `formatTime`) for comparison. Document in `value.go`; single-user desktop app, no multi-zone concern. |
| `AssetRow` exact field list | — | Finalize against the grid-card needs in `frontend/01-flows-and-views.md`; ~15 fields, err toward fewer (it's the hot projection; `Get` exists for depth). |

Record each resolution in this doc's status block + `02-decision-log.md` when made.

## 10. Acceptance

The `ast` package is pure, so these are all fast, hermetic unit tests — exhaustive is the point.

- **Vocabulary × operator golden tests:** every field × every allowed operator compiles to the
  expected SQL + args (golden per pair); every field × a disallowed operator fails validation
  with the typed error.
- **Completeness (C10):** a test iterates `vocabulary` and asserts a compiler-registry entry
  exists for every field (and vice versa) — adding a field without its compiler fails CI, not
  code review.
- **JSON round-trip property:** marshal∘unmarshal = identity across a generated corpus of valid
  trees; ambiguous nodes (both/neither of `op`/`field`), unknown keys, unknown fields/operators,
  version 0, and version `Version+1` all reject with the right typed error.
- **Sealed-union exhaustiveness:** the compiler's type switch handles every `Node` implementation
  (a `default: panic` branch + the completeness of Group/Leaf tests covers it).
- **Parameterization property:** for a corpus of queries with hostile string values
  (`'; DROP TABLE assets; --`, GLOB/LIKE/FTS metacharacters), the compiled SQL text contains no
  user value substring; values appear only in `Args`; LIKE patterns are escaped; FTS input is
  quoted (a stray `"` in a text search is a hit for a weird filename, not a syntax error).
- **Determinism:** same query + same `now` = identical Statement, byte for byte. `AnchorNow`
  with two different `now` values = different args (rolling proven).
- **Tiebreaker invariant:** every compiled ORDER BY ends in `, id` — asserted across all
  arrangements, including the COALESCE fallback.
- **Nesting semantics against a real DB** (sqlite package tests, seeded via `testutil`):
  `(A OR B) AND NOT (C)` returns exactly the right rows; `tag under` respects hierarchy and
  tombstones; `empty` operators distinguish NULL from zero-value; scope × where compose.
- **Index slices:** `AssetIDSlice(q, a, i, j)` equals the ids of `QueryAssets` rows `[i, j)`;
  `IndexOfAsset` inverts it; both stable under re-execution.
- **Single-statement bulk apply:** `ApplyTriagePatchByQuery` on a large seeded set with
  `exceptIDs` executes ONE UPDATE (assert via affected count + absence of per-id loops) and
  returns the exact affected-id set.
- **Bulk-undo acceptance (seam §Additions):** 300k-row synthetic catalog — triage patch by
  query, `ReadTriageStates` before-image capture, batched restore — assert wall-clock budget
  (no perceptible stall; set a concrete threshold when the harness exists, target < 1s apply).
- **Collections:** CRUD round-trip; smart collection with invalid AST JSON rejected at
  `Create`/`Update`; stored smart query evaluates through `QueryAssets` identically to the same
  ad-hoc query; manual `position` ordering survives add/remove.
- **FTS⋈tags:** after `ImportKeywords`, a `text matches` query finds the asset by keyword
  (including by ancestor tag name); after tombstoning, it doesn't; `RebuildFTS` backfills a
  catalog with pre-existing tags.
- **Dependency check:** `go list -deps ./internal/ast` contains no `internal/*` except
  `internal/domain` (and no third-party imports at all).

## 11. Doc maintenance on landing (same change, per the master-head contract)

- `00-START-HERE.md`: retire frontier Pick B → seam round becomes frontier (with impl/06 status).
- `../04-open-questions.md` #4: mark resolved, note decisions; #2 (captured_at fallback) resolved.
- `../../seam/01-queries-and-commands.md`: mark §Additions items landed; ledger #1 (AssetFilter
  split) done.
- `../02-decision-log.md`: entries for the §2 locked decisions + §9 resolutions.
- This file: status block updated with what shipped and any deviations, impl/11-style.
