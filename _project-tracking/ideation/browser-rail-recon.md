# Design doc — `features/browser`: wiring the Tree rail to real data

**Round type:** reconnaissance + design (no build). Nothing is written into `features/` this
round. The output is this doc — the frontend-side seed spec for the **source-management round**:
`features/browser` is folded into that round (Ari's call, 2026-07-22), because that round already
owns the D24 `Volume`/`Folder` split, the folder-tree deriver, the counts, and the schema migration,
and the rail is its primary consumer. This doc supplies the data-format findings, the DTO→node
mapping, the state wiring, and the seam projections the rail needs — as *input* to that combined
backend+frontend milestone, not a standalone build.

**Revision 2 (2026-07-22):** incorporates the design-review findings — the smart-collection count
gap (D-2 now decides it), the `collection_assets` table-name correction, a count-freshness pin,
the offline-volume `isDisabled` mapping reopened as a question, the §18 residue narrowed to
"partially resolved," and the `CollectionKind` C15 fix. The D24 story, previously told in four
places, is consolidated into one authoritative section.

## Context

The Tree primitive (D37) is built and domain-blind: it takes `nodes: TreeNodeData[]` and renders
Sources / Collections / Tags with elbow connectors, a scent-count badge, and single/multiple scope.
It has **no real consumer** — the only callers are stories and hardcoded design-library specimens.
`features/browser` will be its first real consumer: fetch the nav axes from the seam, map each DTO
onto `TreeNodeData`, and drive the catalog query **scope** from tree selection.

Before wiring, we needed the truth about what the backend serves. The finding: the three axes sit at
three maturity levels, per-node counts exist in no DTO, and — decisively — the top axis is mid-rename
under D24 (see "The D24 transition" below), which is why the rail is folded into the round that owns
that rename rather than built `source`-shaped now.

## Where this lands (tracking)

The source-management round is named across the docs (D24; DEFERRED §1/§8/§18; `vocabulary.md`;
`epics/frontend-import.md`) but is **not yet a filed epic/task**. Filing it — a combined
backend (Volume/Folder split + migration + `getFolderTree` + count projections) + frontend (the
browser rail) milestone — is a queue action for Ari, not this round. This doc is its frontend seed.

## What the Tree consumes (the target shape)

`frontend/src/components/tree/tree.tsx` — domain-blind, already built:

```ts
interface TreeNodeData {
    id: string;
    label: ReactNode;
    icon?: IconConcept;      // "source" | "folder" | "collection" | "tag" (§14, one glyph per kind)
    count?: number;          // §13 scent count, rendered muted in a Badge
    children?: TreeNodeData[];
    isDisabled?: boolean;
    textValue?: string;      // typeahead; defaults to label when it's a string
}
```

`features/browser` maps source/collection/tag DTOs onto this. The Tree owns behavior (RAC keyboard,
expand, selection); the feature owns fetch + mapping + scope dispatch.

## Data inventory — what EXISTS vs what's PLANNED

| Axis | Seam call | Status | Wire shape | Hierarchy | Count |
|---|---|---|---|---|---|
| **Sources** | `api.listSources()` | **real e2e + mocked** (`src-0..2`) | `domain.Source` — PascalCase, untagged | flat | none |
| **Collections** | `CollectionService.ListCollections()` | binding **generated, not in the `AlexandriaAPI` contract** | `domain.Collection` — PascalCase | `ParentID` (client builds tree) | none |
| **Tags** | `tagTree` et al. | **no seam service; `domain.Tag` absent from `models.ts`** | — (blocked on task `10-backend-tag-system.md`) | `ParentID` + materialized `Path` | none |
| **Folder tree** | `getFolderTree` | **deferred** (DEFERRED §7 — no path→tree deriver) | — | — | — |

Exact structs (source of truth — Go `internal/domain/*`, reflected into `wailsjs/go/models.ts` for
the first two; PascalCase because the Go structs carry no json tags):

- **`domain.Source`**: `ID, Name, Kind ("local"|"external_drive"|"smb"|"nfs"), BasePath,
  FilesystemUUID?, DiskSerial?, VolumeLabel?, Host?, ShareName?, PollIntervalSecs?, ScanRecursively,
  Enabled, Connectivity ("online"|"offline"), LastScannedAt?, CreatedAt, UpdatedAt`
- **`domain.Collection`**: `ID, Name, ParentID?, Kind ("manual"|"smart"), Query? (AST JSON),
  CoverAssetID?, SortField?, SortDir, CreatedAt, UpdatedAt`
- **`domain.Tag`** (not yet on the wire): `ID, Name, Slug, ParentID?, Color?, ColorMode
  ("inherit"|"custom"|"none"), Path (materialized ancestry), CreatedAt`

Membership storage: manual-collection membership lives in **`collection_assets`**
(`collection_id, asset_id, position` — schema line 229). Smart collections store **no membership
rows**; their membership *is* the stored AST `Query`. This asymmetry drives D-2 below.

## Three load-bearing facts the recon surfaced

1. **The top axis is mid-rename under D24** — see "The D24 transition" below for the full story
   and its consequences. The one-line version: a `SourceNode` projection minted now would be born
   with a dead noun, so the rail is folded into the round that owns the rename.
2. **There is no `source` scope kind.** Generated `ScopeKind = library | collection | folder | tag` —
   the vocabulary already anticipates D24. Selecting a top-axis node maps to a **`folder` scope**:
   `{ kind: "folder", sourceId, path: "", recursive: true }` (the mock already narrows `folder` by
   `sourceId`). Consequence: the top rail is the set of **folder-scope roots**, the deferred
   `getFolderTree` supplies their expandable children, and the Tree + scope are already noun-neutral —
   only the data adapter and the section label carry "source." Collection/tag scope narrowing in the
   mock is still a stub (`return true` — "membership tables land with the browser sidebar").
3. **Per-node counts exist in no DTO and nowhere in the plan.** The only count today is
   `queryAssets().total` (whole-query COUNT). Feeding the badge for real is net-new seam shape work
   (specified below) — and the manual/smart membership asymmetry means "the count" is two different
   computations (D-2).

`DEFERRED.md §18` anticipates this round — *"the browser-rail / collections round … mint the wire
projections there"* — and flags that `ListSources` still returns the raw untagged `*domain.Source`.
But §18 predates weighing it against D24's "don't propagate source" rule; the two are reconciled
below (mint for Collections, defer the top axis to the round that owns its noun).

## The D24 transition (authoritative section — resolved: fold the rail into that round)

**D24 (ratified 2026-07-10)** splits `Source` into a **`Volume`** (identity/portability anchor —
filesystem UUID + connectivity) and a **`Folder`** (tracked root — sync scope + sync_mode); one
volume, many folders; assets reference `(volume, path)` and the folder tree is *derived* from
paths, never stored. `vocabulary.md` forbids propagating "source" into new surfaces until the
**source-management round** renames the noun; that round also owns the schema/migration, the
NFD-normalization key, the folder-tree deriver, and the counts.

**Decision (Ari, 2026-07-22): fold the browser rail into that round** rather than ship a standalone
`source`-shaped rail now. Consequences:

- **Volumes + Folders (was "Sources")** are built as one milestone with their backend: the round
  delivers the `Volume`/`Folder` schema split + migration, the `getFolderTree` deriver, the count
  projections, the NFD normalization key, **and** the rail's top axis UI together. The Tree primitive
  and the `folder` scope are already noun-neutral, so no rework there — only the (not-yet-written)
  top-axis adapter + label are authored fresh, in the correct vocabulary from day one.
- **Collections** (stable noun) ride along in the same round as a second rail section — the
  `CollectionNode` projection (D-1) is cheap and the round is already touching the rail. It *could*
  split into its own later increment, but co-building avoids a second pass over the same feature file.
- **Tags** stay blocked on task 10 regardless; the rail renders a coming-soon Tags section until the
  tag-management backend lands, whichever round pulls it in.

Net: there is no separate "browser-rail round." This doc is the frontend spec inside the
source-management round. Later sections reference this one rather than restate it.

## Design decisions

### D-1 · Mint a `CollectionNode` wire projection now; defer the top-axis projection to D24's round

For **Collections** (stable noun), follow DEFERRED §18: stop shipping raw `*domain.Collection`,
introduce a json-tagged wire DTO carrying display + nav fields **plus the count** (C7-clean — a new
result shape, not a new predicate; folds the count into one round trip):

```go
// internal/seam — proposed Collections wire projection (json-tagged; camelCase on the wire)
type CollectionNode struct {
    ID         string                `json:"id"`
    Name       string                `json:"name"`
    ParentID   *string               `json:"parentId"`   // client assembles the tree
    Kind       domain.CollectionKind `json:"kind"`       // C15: the domain enum, not a bare string;
                                                         // TS side consumes the generated union
    AssetCount *int                  `json:"assetCount"` // nil = no count (smart collections, D-2)
}
```

```ts
listCollections(): Promise<CollectionNode[]> // NEW on the AlexandriaAPI contract
```

For the **top axis**, the equivalent projection (`VolumeNode` + a derived `FolderNode` tree, with the
count) is **specified by, and owned by, the source-management round** — not minted here (see "The D24
transition"). `ListSources`/`domain.Source` stay as-is until then. This is the reconciliation of
DEFERRED §18 against D24: mint where the noun is settled, defer where it isn't.

### D-2 · Counts come from the list read (one GROUP BY, one round trip) — manual collections only

"Whatever is necessary to get them from the API" = **the list method computes the count**, rather
than N scoped `queryAssets` calls or a separate keyed counts map. Backend:

- Collections: `SELECT collection_id, COUNT(*) FROM collection_assets GROUP BY collection_id`.
- Top axis (Volume/Folder): the count is derived per volume/folder path — but this lands in the
  source-management round alongside the folder-tree deriver (same GROUP BY discipline).

**Smart collections get no count badge this round.** A smart collection has no rows in
`collection_assets` — its membership is its stored AST query, so the GROUP BY would silently badge
every smart collection `0`, which is worse than no badge. The honest alternatives:

- **(chosen) `assetCount: null` for smart collections**; the Tree already renders no badge when
  `count` is absent. Zero extra query cost, no lie on screen.
- *(rejected for now)* compile each smart query to a COUNT inside the list read — still one round
  trip, but N compiled queries per rail render, each hitting the asset table. If smart-collection
  badges prove wanted, this is the upgrade path, and it should be measured first.

The `AssetCount *int` in D-1 encodes this: `nil` = "no count," distinct from `0` = "empty manual
collection."

**Direct counts only, this round.** Subtree rollups (a parent collection showing descendants'
totals) are a separate decision: collections are adjacency (needs a recursive CTE), tags carry a
materialized `Path` (subtree is a cheap `LIKE`). Flag as an open pin, don't build. §13's rule is a
count "in scope" — direct membership is the defensible default until Ari rules on rollup.

**Freshness pin:** counts ride the list read, so they go stale the moment an import or a membership
mutation lands while the rail is mounted. The generated event bus already carries what's needed —
topic `catalog`, type `changed` (plus `sourceStatus` for connectivity). The build must name which
events invalidate the `useCollections()` / top-axis TanStack queries; badges that silently drift
during an import (the product's flagship flow) are worse than no badges. This doc pins the
requirement, not the debounce/coalescing policy — that's an implementation detail for the round.

Rejected alternative — per-node `queryAssets().total`: N round trips per rail render, and it abuses
the asset workhorse for a nav count. The GROUP BY projection is one call and matches C7.

### D-3 · DTO → `TreeNodeData` mapping (the feature's only real logic)

| Axis | id | label | icon | count | children |
|---|---|---|---|---|---|
| Volume (top) | `.id` | `.name` | `"source"`→`"folder"` at D24 | `.assetCount` | derived Folder subtree (`getFolderTree`, D24) |
| Collection | `.id` | `.name` | `"collection"` | `.assetCount` (null → no badge) | built from `parentId` (pure client fn) |
| Tag | `.id` | `.name` | `"tag"` | count | built from `parentId`/`Path` (blocked) |

**Offline volumes: `isDisabled` is NOT the mapping — open question.** The earlier draft mapped
`connectivity: "offline"` → `isDisabled`, but that blocks selecting the volume as scope, and the
catalog remains fully queryable while a drive is unplugged — browsing offline volumes (metadata,
judgments, thumbnails persist) is the local-first product's core promise, and `Connectivity` is an
observation column, not a gate. The working default is therefore **offline = visual state**
(dimmed icon / badge treatment per the design constitution), selection still allowed; `isDisabled`
stays reserved for genuinely non-selectable nodes (e.g. the coming-soon Tags section). Carried to
Ari as open question 4.

`buildForest(items, parentId)` is a small pure helper (adjacency → `TreeNodeData[]`),
unit-testable, feature-local. The `"source"` icon concept survives the rename (it's already the
`hard-drive` glyph — a Volume); Folders take `"folder"`.

### D-4 · State wiring (per `frontend/CLAUDE.md` §2/§4)

- **Node data → server state:** a new `useSources()` / `useCollections()` hook in `api/queries.ts`
  (TanStack; `retry:false`; explicit error state per §4). Not the Zustand store — the store never
  holds borrowed server state. Invalidation on catalog events per the D-2 freshness pin.
- **Tree selection → view state:** selecting a node dispatches a **scope** change into the one
  catalog store (`stores/catalog-store.ts`): Source → `{kind:"folder", sourceId, path:"",
  recursive:true}`; Collection → `{kind:"collection", id}`; Tag → `{kind:"tag", id}`. Scope is the
  extensional root of the C2 state equation, distinct from the filter predicate.
- Tree `selectionMode="single"` (§15 single scope). The rail is chrome — hue-free.

### D-5 · What stays blocked (record the triggers, build nothing)

- **The whole top axis (Volumes + Folders):** owned by the source-management round (see "The D24
  transition") — the schema split, the `VolumeNode`/`FolderNode` projections + count, the
  `getFolderTree` deriver (DEFERRED §7), and the NFD-normalization key (one migration event).
- **Tags:** needs task `10-backend-tag-system.md` (`Tree/Get/Update/Delete/SetAssetTags` + the
  `tagTree`/… seam methods + a `TagNode` wire projection). Trigger on file: "when the tag-management
  UI is the caller" — i.e. this rail. Rail renders Tags as an empty/coming-soon section until then.
- **Subtree count rollups** and **smart-collection count badges:** open pins (D-2).
- **Per-asset collection membership read** (DEFERRED §18's inspector "Contained in" row): NOT
  satisfied by `listCollections` — "which collections hold asset X" is a different result shape
  (C7-sanctioned new method), still unbuilt. §18 stays partially open (see residue below).

## Proposed durable residue (written when the source-management round runs, not now)

This is a *design* round; the code and its doc-residue land inside the source-management round. When
that round executes, the decisions above fold into the durable docs (NOT written this round):

- `docs/seam-contract.md` — add `listCollections` (projected, count-carrying) to the bound-surface
  ledger; note the `CollectionNode` projection. The top-axis (`VolumeNode`/`FolderNode`) projection is
  cross-referenced to the source-management round, not added here.
- `docs/decisions.md` — a new D-number: the `CollectionNode` projection + count-via-GROUP-BY
  (manual-only; smart = null) + the (existing) folder-scope mapping for the top axis + direct-count
  default; explicitly records that the top-axis projection is deferred to D24's round.
- `_project-tracking/DEFERRED.md` — §18: mark the collection *list* projection resolved by this
  decision, but keep the row **partially open**: the per-asset membership read ("which collections
  hold asset X") and the source-name projection both remain unbuilt (the latter folds into the
  source-management round). Keep §7's tag rows; note that the source/volume projection folds into
  the D24 source-management round (§18's "mint here" is superseded by "don't propagate source" for
  that axis).
- Backend work items (collection count projection; task 10 for tags; the volume/folder axis under the
  source-management round) enter the queue.

## What is explicitly NOT happening this round

No `features/browser/*` files, no `api/` contract/mock/wails-api edits, no store changes, no backend
Go. Recon + design only. The seam signatures above are a **proposal to ratify**, not code to write.

## Verification (of the doc, not code)

- The three struct shapes are quoted from `internal/domain/*` / `wailsjs/go/models.ts` and match the
  bound reality (`app.go` `boundServices()`: sources, assets, collections, settings, imports — no
  tags).
- The `ScopeKind` claim (no `source` kind; top axis → folder scope) is checked against
  `src/_generated-types/vocabulary.ts` and the mock's `inScope`; the folder payload shape
  `{sourceId, path, recursive?}` matches `query-model/ast.ts`.
- The D24 transition claim is checked against `docs/decisions.md` D24, `docs/vocabulary.md` ("don't
  propagate source into new surfaces"), and `DEFERRED.md` §1/§18.
- The counts gap is checked against `internal/seam` + `internal/domain` (no count field on any nav
  DTO). The membership table is `collection_assets` (schema line 229) — an earlier draft misnamed it
  `collection_members`; smart collections confirmed to store no membership rows.
- The event names in the freshness pin (`catalog`/`changed`, `sourceStatus`) are checked against
  `src/_generated-types/events.ts`.

## Open questions for Ari (carried into the source-management round)

Sequencing is settled (fold the rail in). What remains to rule on when that round is designed:

1. **Collections: co-build or split out?** Ship the Collections rail section within the
   source-management round (co-build; one pass over `features/browser`), or defer it to its own later
   increment. (Recommend: co-build — the round already owns the rail; `CollectionNode` is cheap.)
2. **Count semantics:** direct membership only, subtree rollup deferred? (Recommend: yes — direct via
   GROUP BY; tags' materialized `Path` makes subtree cheap later, collections need a recursive CTE.)
3. **Smart-collection badges:** accept no-badge-for-smart (D-2's `assetCount: null`), or fund the
   compiled-COUNT upgrade path? (Recommend: no badge — zero cost, no lie; measure before funding N
   compiled counts per rail render.)
4. **Offline volumes:** confirm offline = visual state, selection allowed (D-3) — i.e. an unplugged
   drive's catalog stays browsable from the rail. (Recommend: yes — `isDisabled` would block the
   local-first offline story; `Connectivity` is an observation, not a gate.)
5. **File the round.** The source-management round isn't a queued epic/task yet — a combined
   backend+frontend milestone folding in this rail spec. Filing it is Ari's queue action.
