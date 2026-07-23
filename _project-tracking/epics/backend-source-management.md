# Source management — executing the D24 Volume/Folder split, and the browser rail

**Status:** drafted 2026-07-22, awaiting design review. Constants referenced: C2, C7, C15.
Parent design: D24 (the split, "compare keys, open bytes", the real-copies direction).
Frontend seed: `../ideation/browser-rail-recon.md` (a work-item citation; folded in and
deleted with this epic on close). Backend-led with a seam widening and two frontend
features; this is the round nearly every deferral ledger points at.

## What this epic is

The round that retires the `Source` noun. `Source` becomes **`Volume`** (identity/portability
anchor: filesystem UUID + connectivity) + **`Folder`** (tracked root: sync scope + sync_mode);
one volume, many folders; assets keyed `(volume, path)`; the folder tree *derived* from paths,
never stored (D24). Everything gated on that split executes here: the NFD `PathKey` wiring
(same tables, same schema event — DEFERRED §8), the folder-tree deriver + `getFolderTree`
(§7), the `removeSource` verb (§7), the per-folder
`sync_mode` gating layer (§1), the nav wire projections + counts (§18 + the seed doc), and
the **browser rail** as the tree's first real consumer. The ast token `source` renames with
the noun (`vocabulary.md`); UI copy already says "folder" (C14 made that free).

## What exists already (inventory, verified 2026-07-22)

- **Engine:** watcher/reconciler/pipeline key off `source.ID` + `base_path` mechanically —
  §1's assessment stands: "a migration + keying off a new field, not a gut-rewrite." All
  three sync behaviors (manual / watched / scheduled) already run; `sync_mode` is a
  policy/gating layer, not new machinery. `domain.PathKey` + tests landed with D24 but no
  production path calls it (§8 — the NFD phantom-identity risk is still live).
- **Schema:** pre-release policy applies — the split edits `0001_initial_schema.sql` in
  place; no stacked migration, no real user catalogs to migrate. §8's design note binds the
  NFD decision to this same schema event (stored normalized key column, key-to-key compare).
- **Seam:** `SourceService` List/Create/Update real e2e; `CollectionService` full CRUD bound
  but **not on the `AlexandriaAPI` contract**; no count field on any nav DTO;
  `getFolderTree` / `pickDirectory` / `removeSource` deferred (§7's table).
- **Frontend:** the Tree primitive (D37) built, domain-blind, consumer-less. `ScopeKind` is
  already noun-neutral (`folder`, no `source` kind); the folder scope payload
  `{sourceId, path, recursive?}` compiles today. Full recon + DTO mapping: the seed doc.

## The work

- **Backend:** the schema split + `(volume, path)` rekey + stored NFD key column, one schema
  event; the noun-rename ripple through domain/catalog/ast token + `make generate`; the
  folder-tree deriver; count projections (per-folder, and the `collection_assets` GROUP BY);
  `SourceRepo.Delete` under the cascade-vs-orphan ruling; the `sync_mode` field + entry-point
  gating (§1).
- **Seam:** `VolumeNode` / `FolderNode` / `CollectionNode` wire projections (counts riding
  the list reads); `getFolderTree`; `listCollections`; `removeSource` — each onto contract +
  mock + wails adapter. `pickDirectory` is a **shared verb, not owned here**: §7's trigger is
  the Add-Source flow (frontend-import's add form, likely first lander), and export's output
  picker is a third consumer — it's a noun-free Wails-dialog wrapper, so whichever epic lands
  first mints it and the others call it.
- **Frontend:** `features/browser` — the rail per the seed doc (D-1..D-5); and the
  manage-sources surface (add folder via `pickDirectory`, remove, sync_mode) — this epic's
  eponymous UI, its shape a design-round call.

## Design-round agenda (settle before minting tasks)

1. The seed doc's open questions 1–4: collections co-build vs split; direct-count vs rollup;
   smart-collection badges; offline-volume treatment.
2. **Cascade vs orphan** on `removeSource` (§7 names this ruling as the precondition).
3. **sync_mode UI slice:** does watch/schedule *enablement* ship here, or does the field land
   engine-side with manual-only UI? (§1's guardrail paragraph gets its dated note either way.)
4. **Asset/file logical-physical split + the real-copies lineage edge** — D24 says this round
   *evaluates* them; rule them in or out explicitly.
5. **Review-UX boundary:** DEFERRED §5 names a joint "review-UX / source-management
   milestone" for the pending-review projection + resolution actions, but the Review tab has
   its own planned epic (`frontend-review.md`). Only the *source-aware kind rule* (same-source
   → move, cross-source → duplicate) depends on this split — decide which epic owns the
   projection.
6. **Loose-file per-file scope** (§1's original driver) — in, or explicitly re-deferred?

## Out of scope (deferred with triggers, unchanged by this epic)

- **Tags axis** — task `10-backend-tag-system.md`; the rail ships a coming-soon section.
- **Subtree count rollups; smart-collection compiled counts** — seed-doc pins stand.
- **Per-asset collection membership read** (§18's remainder) — unless the design round pulls
  it in as the cheap C7 method it is, alongside the other projections.
- **`openAsset` / `openWith` / `revealInFileManager`, `deleteFromDisk`, undo/redo** — §7's
  other rows keep their own triggers.

## Child tasks (sketch — minted, numbered, and Blocked-by-wired on epic close)

1. **backend-volume-folder-schema** — the split in 0001, the rekey, the NFD key column, the
   noun rename + generate ripple. The schema event everything else stacks on.
2. **backend-folder-tree-deriver** — path→tree derivation + the count projections.
   Blocked by 1.
3. **seam-source-management-wire** — the projections, `getFolderTree`, `listCollections`,
   `removeSource` (+ `pickDirectory` only if no earlier epic has minted it); contract +
   mock + adapter. Blocked by 1 (shapes).
4. **frontend-browser-rail** — `features/browser` per the seed doc. Blocked by 3.
5. **frontend-manage-sources** — add/remove/sync_mode surface. Blocked by 3; scope from
   agenda item 3.

Numbers assigned at close against the live queue (frontend-import's sketch holds 36–39).
