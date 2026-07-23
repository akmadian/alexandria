# 41 — Folder-tree deriver + the nav counts

**Areas:** backend. **Blocked by:** 40-backend-volume-folder-schema.md.
**References:** D24 (tree derived from paths, never stored), D41 (per-axis count semantics;
smart-count architecture), C7 (counts are a new result shape), DEFERRED §7 (`getFolderTree`
row).

## Scope

The read-side engine powering the browser rail; no seam surface yet (that's 45).

- **The deriver:** asset paths per volume → an in-memory folder forest (volume → tracked
  roots → derived subfolder nodes). Derived nodes are not rows — they exist only in the
  response; nothing to rebuild because nothing is stored.
- **Folder counts = subtree**, computed bottom-up in the deriver's tree in one pass (D41:
  matches what clicking shows — folder scope defaults recursive). Volume count = its whole
  tree.
- **Collection counts:** one `GROUP BY collection_id` over `collection_assets` (direct
  membership; rollup stays deferred, D41 revisit trigger).
- **Smart-collection counts:** one compiled COUNT per smart collection through the `ast`
  compile family — the one query authority; **no Go-side predicate evaluation** (impl/13,
  C15). Runs inside the same list read; time-anchored drift accepted (D41; timer refresh is
  the named upgrade).
- Excluded from every count: `is_deleted` rows.
- Logging: per-volume derive summary at Info (folders, assets, elapsed), per-root at Debug.

## Out of scope

Wire projections and services (42/45), subtree rollup for collections, count caching (measure
first — D41).

## Acceptance

- Fixture catalog (two volumes, nested tracked roots' subtrees, loose deep paths): derived
  forest matches the on-disk shape; counts at every node equal the assets a recursive folder
  scope at that node yields (property: node count == `QueryAssets` total for that scope).
- Collection counts: manual collections match membership; a smart collection's count equals
  its query's `total`; an *empty manual* collection is `0`, a smart collection with an
  unsatisfiable query is `0` — and the two are distinguishable from "no count" at the wire
  layer (task 42's `*int`).
- Soft-deleted assets appear in no count.
- Benchmark note recorded in the task-closing commit message: derive + counts elapsed on a
  ~50k-asset fixture (the D41 "measure before caching" baseline).
