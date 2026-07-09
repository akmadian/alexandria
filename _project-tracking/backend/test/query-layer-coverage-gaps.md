# Query Layer (impl/13) — Integration Test Coverage Gaps

**Date:** 2026-07-08. Written during impl/13 build; update as gaps are closed.

The AST package has compile-level tests (SQL string shape, parameterization, escaping) and the
sqlite package has integration tests (real queries against in-memory SQLite). The two sets don't
fully overlap — the gaps below are paths tested only at the string level, never executed against
real data.

## Gaps

### 1. Date-range predicates (`within` / `notWithin`) against real rows

**What's missing:** No integration test inserts assets with `captured_at` values and queries with
a `DateValue` (anchor + duration) to confirm correct matches. The rolling-`now` resolution and
calendar-unit arithmetic (`time.AddDate`) are tested only at the compile level (args differ for
different `now` values, SQL shape is correct).

**What to test:** Assets spanning a date boundary; a rolling "last 30 days" query that includes/
excludes correctly; a concrete-anchor query; a `notWithin` query (the negated form).

### 2. LIKE operators (`contains` / `startsWith`) against real rows

**What's missing:** `CompileLIKEEscaping` verifies that `%` and `_` are backslash-escaped in the
args, but no integration test runs a `contains` or `startsWith` query against actual filenames.

**What to test:** Filename containing LIKE metacharacters (`photo_100%.jpg`); `startsWith` on a
path prefix; `contains` on a substring.

### 3. FTS quoting edge cases

**What's missing:** The compile test escapes hostile strings, but no integration test searches
for text containing FTS5 special characters (`*`, `-`, `"`, `NEAR`).

**What to test:** Asset with filename or tag containing FTS operators; confirm the query matches
without FTS syntax errors.

### 4. Sort tiebreaker correctness under value collisions

**What's missing:** Pagination is integration-tested (pages don't overlap), but never with
colliding sort values. If 10 assets share the same `rating`, the `id` tiebreaker must produce a
deterministic total order — no row appears on two pages or is skipped.

**What to test:** Insert N assets with identical sort-column values, paginate through all of them,
confirm the union of pages equals the full set with no duplicates or omissions.

### 5. Tag-scoped queries (`ScopeTag`) against real data

**What's missing:** The compile test checks that `ScopeTag` produces an `asset_tags` subquery.
The integration tests cover `has`/`under`/`empty` as *predicates*, but no test uses
`Scope{Kind: ScopeTag, ID: "..."}` as the query scope and confirms the result set.

**What to test:** Assets tagged with a tag, queried via `ScopeTag`; confirm only tagged assets
returned, and that additional predicates compose correctly on top.

### 6. `MergeScope` with a real scope + stored query

**What's missing:** `MergeScope` is tested structurally (tree shape) and the smart-collection
integration test evaluates a merged predicate, but that test doesn't set a scope on the outer
query. The interaction of scope + stored predicate + user predicate is untested end-to-end.

**What to test:** Smart collection (stored query) opened inside a source scope, with an
additional user filter — confirm all three constrain the results.

### 7. Multi-field compound queries

**What's missing:** The nested boolean integration test uses `rating + flag`. No test exercises
three or more fields together (e.g., rating + date range + tag + text search).

**What to test:** A realistic "power-user" query combining several field types; confirm correct
intersection.
