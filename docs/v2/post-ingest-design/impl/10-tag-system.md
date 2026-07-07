# impl/10 — Tag System

**Status: design complete; not yet implemented. Unblocks impl/06 keyword union and impl/09 LrC import.**
**Scope:** new tag repository in `internal/sqlite`, a `KeywordImporter` seam in `internal/catalog`,
and edits to `internal/migrations/0001_initial_schema.sql` (pre-release, in place).
**References:** D8 (classification), D22, `03-data-model.md` §1, FR "Tags" / "Tag Colors" (P0/P2),
`internal/domain/tag.go`.

## Why now

The `tags`/`asset_tags` tables and `catalog.TagRepository` interface have existed since impl/01–02,
but the repository was deliberately left unimplemented: impl/02 deferred it ("build with the
consumer — method shapes would be guesses without a caller"). Two consumers now exist —
**impl/06 keyword union** (the immediate driver) and **impl/09 LrC migration** (reuses impl/06's
keyword path unmodified) — so the shapes are no longer guesses. This doc builds only what those
consumers need; the tag-management UI backend (`Tree`/`Update`/`Delete`) stays deferred until *it*
is the consumer.

## Data model

Two schema edits ride into `0001` in place (pre-release policy — no stacked migrations):
`tags` gains `color_mode` and `path`; `asset_tags` gains `removed_at` and its reverse index goes
partial. Everything else already exists.

### `tags`

```sql
CREATE TABLE tags (
    id          TEXT PRIMARY KEY,          -- UUIDv7
    name        TEXT NOT NULL,             -- display form ("New York"); first-seen wins
    slug        TEXT NOT NULL,             -- normalized match key (see "Slug")
    parent_id   TEXT REFERENCES tags(id) ON DELETE CASCADE,   -- structural truth (adjacency)
    color       TEXT,                      -- hex "#RRGGBB"; meaningful only when color_mode='custom'
    color_mode  TEXT NOT NULL DEFAULT 'inherit'
                CHECK(color_mode IN ('inherit','custom','none')),
    path        TEXT NOT NULL,             -- [derived] materialized ancestry, '/rootId/…/selfId/'
    created_at  TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_tags_slug_parent ON tags(slug, IFNULL(parent_id, ''));  -- sibling uniqueness
CREATE INDEX idx_tags_parent ON tags(parent_id);
CREATE INDEX idx_tags_path   ON tags(path);   -- GLOB-prefix descendant queries (see Performance)
```

Column notes:
- **`parent_id`** is the structural source of truth (FK, cascade). Adjacency list.
- **`path`** is *derived from* `parent_id` — a materialized ancestry string of tag IDs, self
  included, slash-delimited and slash-terminated. Root tag `Travel` → `/travelId/`; its child
  `Japan` → `/travelId/japanId/`. It exists purely to make subtree queries an indexed prefix scan
  instead of recursion (Lightroom's `AgLibraryKeyword.genealogy` plays the same role). Because it is
  derived, it carries a rebuild path (`RebuildTagPaths`, below) — the standard rule for derived state.
- **`color` + `color_mode`** — tri-state color (see "Color inheritance"). A single nullable `color`
  cannot distinguish "inherit from parent" from "explicitly no color", so the mode is explicit.
- **`slug` uniqueness is per-parent** (`UNIQUE(slug, IFNULL(parent_id,''))`): two siblings can't
  share a slug, but the same name may appear under different parents (`Travel|Japan|Tokyo` and
  `Places|Tokyo` are distinct rows). This is what makes find-or-create a single indexed lookup.

### `asset_tags`

```sql
CREATE TABLE asset_tags (
    asset_id    TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    tag_id      TEXT NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    source      TEXT NOT NULL DEFAULT 'user'
                CHECK(source IN ('user','xmp','lr')),   -- +'ai' when P4 CLIP tagging lands
    removed_at  TEXT,                                   -- [jdg] null = active; non-null = user-suppressed
    created_at  TEXT NOT NULL,
    PRIMARY KEY (asset_id, tag_id)
);
CREATE INDEX idx_asset_tags_tag ON asset_tags(tag_id) WHERE removed_at IS NULL;  -- partial reverse hot path
```

- **Direct attachments only** — one row per *explicitly* applied tag. Implied ancestors are NOT
  stored (no write amplification, no implied/explicit bookkeeping). Ancestry is resolved at read
  time via `path` (see Performance).
- **`source`** is the per-row class marker (D8): `user` rows are **judgment** (user-declared),
  `xmp`/`lr` rows are **observation** (copied from a file/catalog). Both live in one table because,
  per the classification rule, mixed-class tables carry the class as an explicit column.
- **`removed_at`** is a **judgment** tombstone — see "User-deletion overrides".

### Domain structs (`internal/domain/tag.go`)

```go
type ColorMode string
const (
    ColorInherit ColorMode = "inherit"
    ColorCustom  ColorMode = "custom"
    ColorNone    ColorMode = "none"
)

type Tag struct {
    ID        string
    Name      string
    Slug      string
    ParentID  *string
    Color     *string      // hex; used only when ColorMode == ColorCustom
    ColorMode ColorMode
    Path      string
    CreatedAt time.Time
}

type AssetTag struct {
    AssetID   string
    TagID     string
    Source    string
    RemovedAt *time.Time   // nil = active
    CreatedAt time.Time
}
```

(`Path`, `ColorMode` are added to the existing `Tag`; `RemovedAt` to the existing `AssetTag`.)

## Performance & access patterns — read this before touching the queries

The whole model is shaped around these. The junction is deliberately kept thin because two indexes
carry both hot directions.

| Access path | When | Query shape | What makes it cheap |
|---|---|---|---|
| **asset → its tags** | inspector, per selected asset | `asset_tags WHERE asset_id=? AND removed_at IS NULL` ⋈ tags | PK `(asset_id, tag_id)` — rows for one asset are **contiguous** under `asset_id`; a seek returns just that asset's ~dozens of rows, never a scan |
| **tag → assets, incl. descendants** | click a tag in the sidebar | `tags WHERE path GLOB :node||'*'` → tag IDs → `asset_tags WHERE tag_id IN(…) AND removed_at IS NULL` | prefix scan over the **small** tags table (idx_tags_path) + reverse index on the big table |
| **Untagged** smart collection | occasional | `assets a WHERE NOT EXISTS(SELECT 1 FROM asset_tags WHERE asset_id=a.id AND removed_at IS NULL)` | same `asset_id`-leading seek |
| tag tree render | sidebar (always visible) | all tags, build tree from `parent_id` | small table, full read is fine |

**The load-bearing facts, stated plainly:**

1. **asset→tags is not a scan.** The composite PK is a b-tree ordered by `(asset_id, tag_id)`, so one
   asset's tags are a contiguous run. Filtering by `asset_id` uses the leading column and seeks to
   that run. The *only* way this becomes a scan is filtering the junction by a non-leading column.
2. **Hierarchy expansion runs over the tag tree, not the assets.** `path GLOB 'X*'` returns X's
   subtree tag IDs from the small `tags` table. A tag with 30k assets under it still costs a small
   prefix scan for the IDs; the 30k-row read then rides `idx_asset_tags_tag`. That 30k read is
   **result-bound** — inherent to returning 30k assets, identical under any hierarchy strategy
   (recursive CTE, materialized path, or a closure table). We do not pretend a schema choice makes
   it cheaper; we just avoid making it *worse* (no recursion, no per-asset closure maintenance).
3. **GLOB, never LIKE, for the path prefix.** SQLite optimizes `col GLOB 'literal*'` into an indexed
   range scan (GLOB is binary/case-sensitive by default). `LIKE` is case-insensitive by default and
   will **not** use the index without a `NOCASE` index or `PRAGMA case_sensitive_like`. Tag IDs are
   UUIDs with no GLOB metacharacters (`* ? [`), so `GLOB path||'*'` is safe and fast.
4. **Indexes are tombstone-aware** (`WHERE removed_at IS NULL`) so suppressed rows never bloat the
   hot reverse lookup.

## Hierarchy semantics

- **Attach leaf-only.** `Travel|Japan|Tokyo` creates the parent chain and attaches **Tokyo**;
  `Travel`/`Japan` exist as ancestors but are not directly on the asset. Filtering `Travel` includes
  Tokyo-tagged assets via the `path` subtree expansion. If a user *explicitly* also assigned Japan,
  LrC emits a second path `Travel|Japan`, whose leaf `Japan` attaches too — so explicit mid-level
  assignment is preserved while implied ancestry is not duplicated onto every asset.
- **Reparent** (perf-critical, don't skim): moving node `X` under `newParent` is **two writes**, both
  bounded to X's subtree of *tags* (never assets):
  1. `UPDATE tags SET parent_id=:newParent WHERE id=:X` (one row).
  2. Rewrite `path` for X and every descendant — found by the old prefix — swapping the old ancestry
     prefix for the new one:
     ```sql
     UPDATE tags
     SET path = :newPrefix || substr(path, length(:oldPath) + 1)
     WHERE path GLOB :oldPath || '*';
     ```
     where `oldPath = X.path` and `newPrefix = newParent.path || X.id || '/'` (or `/X.id/` at root).
  **Cycle guard:** reject the move if `newParent.path GLOB X.path || '*'` (newParent is inside X's
  own subtree) — otherwise the paths become self-referential.
  Rename does **not** touch `path` (it is IDs, not names) — only `name`/`slug` and the sibling-slug
  uniqueness re-check.
- **Delete** cascades the subtree via the `parent_id` FK. A non-cascading "delete tag, keep/adopt
  children" is a verb-level choice for the tag UI later, not a storage concern.
- **`RebuildTagPaths`** (derived-state rebuild path): recompute every `path` from `parent_id` by
  walking roots downward (BFS/topological). Used to repair `path` or after any bulk structural
  change. Mirrors `RebuildFTS`'s role for its index.

## Color inheritance

The `color_mode` tri-state stores the user's inherit/don't-inherit choice that a bare nullable
`color` cannot express:

- **`inherit`** (default) — effective color = nearest ancestor's effective color.
- **`custom`** — use this tag's own `color` hex.
- **`none`** — explicitly neutral, and it **breaks the chain** for its subtree.

Effective color is **derived, never stored**:

```
resolve(tag):
    custom  → tag.color
    none    → null
    inherit → tag.parent ? resolve(tag.parent) : null
```

Not storing an `effective_color` is deliberate: recolor or reparent then propagates automatically to
every `inherit` descendant on the next render, with no cascade UPDATE and no race against
reparenting. The tree is small, so the walk is free. New tags default to `inherit`, so "color the
parent, children follow" needs zero extra clicks — the P2 requirement.

## XMP / LrC keyword mapping

Both consumers feed the same two XMP fields through one path (impl/06 field map):

- `dc:subject` — flat keyword names (interop/search mirror).
- `lr:hierarchicalSubject` — pipe-delimited paths (`"Travel|Japan|Tokyo"`), the structure.

**Dedupe rule — `hierarchicalSubject` is authoritative:**
1. Process hierarchical paths first: split on `|`, `EnsureTag` each node in the chain (parent-linked),
   collect the set of every node **name** seen.
2. Process `dc:subject`: skip any flat name already in that set (it's the hierarchical one); create
   the rest as root tags.

`Travel|Japan|Tokyo` + subjects `[Travel, Japan, Tokyo, Sunrise]` → tree `Travel>Japan>Tokyo` (attach
Tokyo) + root `Sunrise` (attach). No duplicate flat Travel/Japan/Tokyo. Matching is by **slug**, so
`Tokyo`/`tokyo` collapse; a genuinely ambiguous flat name present in two branches is skipped (the
hierarchical versions already carry it).

**Slug normalization:** lowercase, trim, collapse internal whitespace to single `-`. **Keep
non-ASCII** (CJK keywords like `赤` must survive — do not ASCII-strip). This is Lightroom's `lc_name`
idea. `name` keeps the display form (first-seen wins); `slug` is the match key.

## User-deletion overrides (the tombstone)

A user removing a tag is a **judgment**; sync is an **observation-writer**; judgments beat
observations (D8). So a user removal must survive re-sync forever:

- **User removes a tag from an asset** → set `removed_at = now` (do **not** delete the row). The row
  survives as the override record.
- **Sync union** (`ImportKeywords`/`AddAssetTags`) uses `INSERT … ON CONFLICT(asset_id,tag_id) DO
  NOTHING`. A suppressed row's PK already exists → the sync no-ops and **never clears `removed_at`**
  (a judgment column the observation-class writer cannot touch). The keyword stays gone even if the
  sidecar keeps re-asserting it.
- **User re-adds** later → the judgment path clears `removed_at` (the only writer permitted to).
- Membership, "Untagged", and any future FTS text all filter `removed_at IS NULL`.

This is the classic XMP round-trip resurrection bug, closed structurally by the writer classes.

## Repository surface

Build only the consumer-driven methods now. One concrete `sqlite.TagRepo{ DB DBTX }`; scoping and
atomicity via the existing `Store`/`Repos`/`InTx` seam (add `Tags *TagRepo` to `Repos`).

```go
// EnsureTag finds a tag by (slug, parentID) or creates it (UUIDv7, path computed
// from the parent, color_mode='inherit'). The find-or-create atom.
func (r *TagRepo) EnsureTag(ctx, name string, parentID *string) (id string, err error)

// AddAssetTags unions tagIDs onto an asset with the given source. INSERT … ON
// CONFLICT DO NOTHING: never duplicates, never deletes, never clears a tombstone.
func (r *TagRepo) AddAssetTags(ctx, assetID string, tagIDs []string, source string) error

// ImportKeywords is the orchestrator both consumers call, inside a Store.InTx:
// build each hierarchy chain (EnsureTag per node), dedupe flat names against the
// hierarchy node set, then AddAssetTags(leaf-of-each-path + surviving flat roots).
// `hierarchical` is pre-split by the caller ([][]string) so the repo stays free of
// the "|" XMP convention.
func (r *TagRepo) ImportKeywords(ctx, assetID string, flat []string, hierarchical [][]string, source string) error
```

`catalog` gains a minimal seam the `Syncer` depends on:

```go
type KeywordImporter interface {
    ImportKeywords(ctx context.Context, assetID string, flat []string, hierarchical [][]string, source string) error
}
```

**Transaction:** the `Syncer` runs `ImportKeywords` in its own `Store.InTx` (separate from the
judgment `ApplyXMPInbound`; both are idempotent and retry-safe, so two transactions is fine — no need
to couple them). `EnsureTag` calls within one `ImportKeywords` share that transaction, so a
half-built hierarchy never commits.

**Deferred to the tag-UI backend consumer** (design captured above, code not built): `Tree`, `Get`,
`Update` (rename + the reparent path-rewrite), `Delete`, and a replace-semantics `SetAssetTags` for
the user-editing path. Carve them when that UI is the caller.

## FTS integration — DEFERRED (pending an FTS deep-dive)

Tag search via FTS is intentionally **not** built here. The tables and junction make tags fully
storable and queryable on their own; wiring tag text into `assets_fts` is a separate design pass.
Open questions to resolve there:

- **Ancestor inclusion** — should an asset's FTS `tags` text include ancestor names (so free-text
  "Japan" finds a Tokyo-tagged asset)? Leaf-only attach + the flat/hierarchy dedupe drop ancestor
  keywords from the searchable text unless we add them back here.
- **Per-asset maintenance** — `AddAssetTags` would need to recompose that asset's FTS `tags` column;
  today only `RebuildFTS` composes it (from the join).
- **Rename/reparent rebuild** — both change ancestor *names*, so affected assets' FTS text must be
  rebuilt (cold path; scope the rebuild to the affected subtree's assets).

Until then, `AddAssetTags` leaves `assets_fts.tags` untouched (a `// TODO(fts):` marker), and the
impl/01 §15 "SetAssetTags maintains FTS tags text" note stays deferred.

## Pitfalls (the ones that bite)

- **`LIKE` for the path prefix** silently drops to a full scan — use `GLOB` (§Performance #3).
- **Reparent forgetting the subtree** — rewriting only `X.path` and not its descendants corrupts
  every descendant's ancestry. Always `GLOB oldPath||'*'`.
- **Reparent cycle** — moving X under its own descendant; guard with the `path GLOB` check.
- **Clearing a tombstone from the sync path** — `AddAssetTags` must be `DO NOTHING`, never
  `DO UPDATE`; resurrecting a user-suppressed tag violates D8.
- **Materializing implied ancestor rows into `asset_tags`** — tempting for a direct tag→assets
  lookup, but it amplifies writes, needs implied/explicit bookkeeping, recomputes on every reparent,
  and risks writing implied tags back into sidecars. Don't; the `path` expansion is the answer.
- **ASCII-stripping slugs** nukes CJK/accented keywords — normalize case/whitespace only.

## Acceptance

- **Find-or-create:** `EnsureTag("Tokyo", japanID)` twice returns the same ID; a second `Tokyo` under
  a different parent is a distinct row; `path` is correct for each.
- **Keyword import + dedupe:** `ImportKeywords` with `flat=[Travel,Japan,Tokyo,Sunrise]`,
  `hierarchical=[[Travel,Japan,Tokyo]]` yields exactly `Travel>Japan>Tokyo` + root `Sunrise`, asset
  attached to `Tokyo` + `Sunrise`, no duplicate flat nodes.
- **Union never deletes:** re-running the same import adds no rows; an existing unrelated tag stays.
- **Tombstone respected:** suppress a tag (`removed_at`), re-import the same keyword → row stays
  suppressed (`removed_at` unchanged); it does not reappear in an active-tags query.
- **Subtree filter:** attach assets across `Tokyo`/`Osaka` (both under `Japan`); `path GLOB japan||'*'`
  → asset set includes all of them; the query plan uses `idx_tags_path` (prefix range) and
  `idx_asset_tags_tag`, not a scan.
- **Reparent:** move `Japan` under a new root; `Tokyo.path` updates to the new prefix; a cycle move is
  rejected; `RebuildTagPaths` reproduces identical paths from `parent_id`.
- **Color resolution:** `Travel(custom #E58)>Japan(inherit)>Tokyo(inherit)` resolves both children to
  `#E58`; setting `Tokyo=none` yields null; changing `Travel`'s color propagates without touching
  child rows.

## Deferred / named upgrade paths

- **Descendant-inclusive live counts across the whole tree** ("Japan (12,431)" on every node at once)
  — the one workload `path` expansion doesn't make cheap; revisit with a materialized
  membership/closure table *if it becomes a requirement* (counts can be cached/periodic until then).
- **FTS + tags integration** — the deferred section above.
- **`source='ai'`** — P4 CLIP auto-tagging.
- **Custom/hex tag colors in the contract** — `TagInput.color` widens from `ColorLabel` to string at
  P2; storage (`color TEXT`) is already ready.
- **Tag-UI backend** — `Tree`/`Update`/`Delete`/replace-`SetAssetTags`.
```
