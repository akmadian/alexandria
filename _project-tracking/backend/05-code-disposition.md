# Existing-Code Disposition

For the implementing instance. The v2 design docs are **authoritative over all existing code** —
where code and spec conflict, the code loses, without ceremony or preservation instinct. This
table grants explicit license per path. "Fresh file" means: write the new shape from the spec in a
new/emptied file, then port named functions in — do NOT incrementally mutate the old shape toward
the new one (that's how old patterns survive).

## Applied so far (impl/01–04, 2026-07-07)

The dispositions below through the sqlite/catalog/metadata/thumbnailer/importer layers have been
executed, and impl/04 restructured the importer into the concurrent pipeline. Notable outcomes vs
the table:

- **Deleted:** `domain/keybindings.go`, `domain/filetype.go` (+ its test), `catalog`'s fat
  `AssetRepository` and `AssetPatch`, the per-MIME maps in metadata/thumbnailer.
- **New:** `internal/assettype/` (the type registry). New with impl/04: `SidecarRepo` +
  `ImportRepo` in `sqlite`; the `importer` job envelope (`jobs.go`), ignore list (`ignore.go`),
  and Sniff-mismatch policy (`mismatch.go`); `cmd/dev` (the engine harness); and the
  `_project-tracking/perf/` reference.
- **`internal/main.go`:** RETIRED — `cmd/dev` replaced it (its `:memory:` smoke path lives on as
  `dev import --catalog :memory:`).
- **`internal/importer/`:** the fresh-file `pipeline.go` restructure (impl/04) is DONE — orchestration
  in `pipeline.go`, one file per stage (`stage_*.go`), the item in `item.go`. Renamed the raster
  processor: `metadata.ExtractImage`→`ExtractRaster`, `thumbnailer.GenerateImage`→`GenerateRaster`
  (files `raster.go`). See `internal/importer/README.md`.
- **`frontend/`:** untouched, as directed. Pending change noted for the seam round: `SourceStatus`
  → `enabled` + `connectivity` (the backend split already landed).

| Path | Disposition |
|---|---|
| `internal/domain/opt.go` | **Keep** as-is |
| `internal/domain/asset.go` | Modify: +title, +caption, +judgment_modified_at handling; Source of truth = impl/01 |
| `internal/domain/source.go` | Modify: `Status` → `Enabled` + `Connectivity` (impl/01 §9) |
| `internal/domain/filetype.go` | **Absorb**: the table's *content* survives into the unified TypeHandler registry (impl/03); the file itself may be replaced |
| `internal/domain/keybindings.go` | **Delete** (D16: keybindings table dropped; frontend owns the vocabulary) |
| `internal/domain/settings.go` | Keep for now; reconcile with contract Settings at the seam round (open question #5) |
| `internal/domain/{tag,collection,duplicate,asset_group,errors}.go` | Keep; light edits (+`Origin` on groups) |
| `internal/catalog/interfaces.go` | **Delete and replace** with the writer-scoped interfaces (impl/02 §2). Do not extend the old fat interface |
| `internal/catalog/asset_query.go` | Modify: `AssetPatch` **dies**, replaced by FilePatch/TriagePatch; AssetFilter grows later (seam round) |
| `internal/migrations/migrator.go` | **Keep** (sound); remove user_version interplay per impl/01 §18 |
| `internal/migrations/0001_initial_schema.sql` | **Rewrite in place** per impl/01 (pre-release; no migration stacking) |
| `internal/sqlite/asset_repo.go` | Heavy modify: DBTX, writer-split methods, sort whitelist. The scan/marshal helpers and patch-SQL builder *mechanics* are fine — port them |
| `internal/sqlite/{source_repo,duplicate_repo,marshal}.go` | Modify per impl/01–02 |
| `internal/importer/*` | **Fresh file for orchestration** (`pipeline.go` written from impl/04), transplant stage bodies (`scan`, `hashFile`, `classifyContent` matrix core, `applyMetadata`, thumbnail/extract wrappers) — they are settled logic. Old `importer.go`/`ingest.go` orchestration is superseded |
| `internal/metadata/*` | Keep extract funcs; **delete** the per-MIME Registry map (dispatch moves to TypeHandler) |
| `internal/thumbnailer/*` | Keep decode/encode + `Path` sharding; delete per-MIME map; Sizes → `[512]` v1 |
| `internal/main.go` | **Keep as the smoke harness until `cmd/dev` exists, then retire.** Its successor is the real dev harness (`impl/08-dev-harness.md`): `cmd/dev` with subcommands + debug server, full `internal/*` access, `:memory:` mode preserved. The real app entrypoint still arrives with the app milestone |
| `internal/testutil/`, all `*_test.go` | **Keep and update** — tests encode intent; extend them per each impl doc's acceptance list |
| `frontend/**` | **Do not touch** (frontend deferred; `api/contract.ts` is design-authoritative and network-shaped on purpose) |
| `docs/original prd/`, older design docs | Historical; **the decision log wins** on every conflict (known conflicts: keybindings table, FTS approach, sources.status, localStorage routing) |
