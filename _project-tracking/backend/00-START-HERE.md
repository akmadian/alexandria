# Alexandria v2 Design Handoff — START HERE (backend area)

*Task-tree head: [`../00-START-HERE.md`](../00-START-HERE.md) — check it first for what's next.*

**Date:** 2026-07-06
**Produced by:** a system-design working session (Ari + Claude Fable) that worked backwards from
`_project-tracking/functional-requirements.md`, interview-style. All decisions below were made deliberately,
with tradeoffs discussed. Nothing in the pre-existing codebase was treated as sacred.
**Audience:** a Claude (Fable/Opus) instance doing further design refinement and/or implementation.

## What this document set is

The session covered: requirements distillation → process topology → storage → subprocess strategy →
type registries → data model & classification → asset identity → ingest pipeline → watcher/reconciler →
XMP sync → settings architecture → job/queue strategy. Frontend design was **deliberately deferred**
(backend → seam → frontend, in that order).

| Doc | Contents |
|---|---|
| `01-requirements-distilled.md` | The NFRs that drive everything; the system's essential nature |
| `02-decision-log.md` | Every architectural decision, numbered, with rationale and revisit triggers |
| `03-data-model.md` | Data classification system, schema spec, identity/matching policy |
| `04-open-questions.md` | Unresolved decisions, with recommendations where they exist |
| `05-code-disposition.md` | Per-path keep/modify/delete license over the existing code — specs win every conflict |
| `06-signals-and-enrichment.md` | Engine side of AI-assisted culling (2026-07-07 frontend round): ENRICH pipeline stage (cheap signals on the thumbnail) + heavy signals as attention-prioritized enrichment jobs. Design-only; build with the signals milestone |
| `impl/done/01-schema-rework.md` | **Blocker 1 — ✅ DONE (2026-07-06)** — migration 0001 rewritten |
| `impl/done/02-repos-and-dbtx.md` | **Blocker 2 — ✅ DONE (2026-07-06)** — transaction seam + writer-scoped repos |
| `impl/done/03-type-registry-and-classifier.md` | **Blocker 3 — ✅ DONE (2026-07-06)** — unified `assettype` registry + `Sniff` |
| `impl/done/04-ingest-pipeline.md` | **The milestone — ✅ DONE (2026-07-06)** — the six-stage concurrent pipeline, sidecar/session repos, job envelope, Sniff mismatch wiring |
| `impl/done/05-watcher-service.md` | **✅ DONE (2026-07-07)** — sensor + poll-timer connectivity; D20 detect-and-flag |
| `impl/06-xmp-sync.md` | **🔨 IN PROGRESS (2026-07-08)** — inbound + outbound + settings + triggers + debounce DONE; caption/title inbound pending (sparse observation writer) |
| `impl/07-dependency-fleet.md` | **🔨 exiftool slice DONE (2026-07-07)** — daemon + discovery; other tools / downloads / one-shot Run deferred |
| `impl/08-dev-harness.md` | `cmd/dev` — ✅ core subcommands (import/reconcile/errors/sessions/rebuild) DONE with impl/04; `--debug` HTTP server (pprof/expvar/statsviz/`/state`) still deferred |
| `impl/09-lrc-migration.md` | **Design only, not started.** Lightroom Classic catalog migration — D21; engine-first (`internal/lrcimport` + `cmd/lrcimport`), Wails wizard wraps it later; pure-read preflight, LrC-side DNG/XMP prep instead of hand-parsing Develop settings |
| `impl/10-tag-system.md` | **🔨 consumer slice DONE (2026-07-07)** — D22; adjacency + materialized `path`, direct-attach junction, `color_mode` tri-state, judgment tombstones. `TagRepo` (EnsureTag/AddAssetTags/ImportKeywords/RebuildTagPaths) + `KeywordImporter` seam built; wired into impl/06. Tag-UI backend (Tree/Update/Delete/reparent) + FTS⋈tags deferred |
| `impl/11-settings-service.md` | **✅ DONE (2026-07-07)** — `internal/settings`: three JSON files (`settings.json`/`machine.json`/`keybindings.json`), no DB table; generic `configFile[T]` with quarantine + hot-reload; ignore-list + worker counts wired. §5 live mid-run pool resize DEFERRED to impl/12 (DEFERRED §6) |
| `impl/12-app-host.md` | **Stub, not started (created 2026-07-08).** The Wails composition root + everything that needs a long-running process: startup sequence (integrity check, backup-before-migration floor), watcher supervision (DEFERRED §2), live pool resize (DEFERRED §6). Trigger: seam round completes |
| `impl/13-query-layer.md` | **✅ DONE (2026-07-08).** `internal/ast` (grammar + vocabulary + validation + JSON + `CompileToSQL`); `QueryAssets`/`AssetIDSlice`/`IndexOfAsset`/`DistinctValues`/`ReadTriageStates`/`ApplyTriagePatchByQuery` surface; collections CRUD; FTS⋈tags slice; COALESCE expression index. `AssetFilter`/`List`/`buildFilterSQL`/`sortColumns` deleted. Unblocks the seam round |

## Where the project is right now

**Blockers 01–03 AND the impl/04 ingest milestone are implemented and green**
(`go vet ./...` clean, full `go test -race ./...` passing). Current state of the tree:

- `internal/domain/` — domain types + `NewID()` (UUIDv7); `Source` split into `Enabled`/
  `Connectivity`; `Asset` gained `Title`/`Caption`/`JudgmentModifiedAt`; added `SidecarFile` and
  `ImportSession`/`ImportError`. `filetype.go`/`keybindings.go` removed (the `FileType` enum stays
  in `asset.go`).
- `internal/migrations/` — `0001_initial_schema.sql` rewritten (FK delete rules, partial unique
  indexes, sidecar_files/import_sessions/import_errors, trigger-maintained FTS5, dropped CHECKs +
  keybindings table).
- `internal/sqlite/` — writer-scoped repos over the `DBTX` seam (`db.go`: `Store`/`Repos`/`InTx`);
  `Open()` (WAL + flock instance lock); `RebuildFTS`; plus `SidecarRepo` and `ImportRepo` (sessions
  + the DLQ), both bundled into `Repos` for the pipeline's batched writes.
- `internal/catalog/` — writer-class interfaces (`AssetReader`/`ObservationWriter`/`JudgmentWriter`/
  `SyncWriter`/`DerivedWriter`); `FilePatch` + `TriagePatch` (the old `AssetPatch` is gone).
- `internal/assettype/` — unified registry (`Handler`, `Classify`, `IsSupported`, `IsSidecar`) +
  magic-byte `Sniff`/`ContentFamily`.
- `internal/metadata/`, `internal/thumbnailer/` — export `ExtractRaster`/`ExtractFunc` and
  `GenerateRaster`/`GenFunc`. "Raster" = the stdlib-decodable formats (JPEG/PNG/GIF), the shared
  backend every format is meant to funnel into (RAW extracts a preview and reuses it) — see
  `_project-tracking/perf/thumbnailing-and-hardware-acceleration.md`. Thumbnailer `Sizes` default `[512]`,
  `ApproxBiLinear` resize.
- `internal/importer/` — **the concurrent pipeline** (impl/04): `pipeline.go` (wiring + run-level
  state) with one file per stage (`stage_scan/hash/match/extract/thumb/write.go`), plus `item.go`,
  `jobs.go`, `ignore.go`, `mismatch.go`. `reconcile.go` is transitional (retires with impl/05).
  **Start at `internal/importer/README.md`** for the lay of the land.
- `cmd/dev/` — the engine harness (`import`/`reconcile`/`errors`/`sessions`/`rebuild fts`;
  `--catalog <dir|:memory:>`, `--debug` pprof/expvar, worker-pool flags, debug logging by default).
  Replaces the retired `internal/main.go`.
- `frontend/` — React shell + `frontend/src/api/contract.ts` (the designed, network-shaped seam).
  Do not do frontend work yet. Pending seam change: `SourceStatus` → `enabled` + `connectivity`.

**All the earlier audit findings are fixed** — including the last one, atomic batched writes, which
landed as the WRITE stage's 50-item `Store.InTx` in impl/04.

## The immediate path (blocking order)

1. ✅ `impl/01` schema rework — DONE
2. ✅ `impl/02` DBTX + writer-split repos — DONE
3. ✅ `impl/03` type registry + classifier (`assettype`) — DONE
4. ✅ `impl/04` ingest pipeline — DONE (concurrent 6-stage pipeline in `internal/importer/pipeline.go`;
   `sidecar_files` + `import_sessions`/`import_errors` repos; `Jobs`/`Progress` envelope; D7 raster
   mismatch policy; `cmd/dev` harness). One incidental fix: `ListKnownFiles` now returns only ONLINE
   assets so a missing file that reappears unchanged is restored, not skipped.
5. ✅ `impl/05` watcher service — DONE (2026-07-07; sensor + poll-timer connectivity, D20 detect-and-flag)
6. ✅ `impl/07` dependency fleet — exiftool slice DONE (2026-07-07; daemon + discovery; rest deferred)
7. ✅ `impl/13` query layer — DONE (2026-07-08; `internal/ast`, query/command surface, collections, FTS⋈tags, old surface deleted)
8. **IN PROGRESS:** `impl/06` XMP sync — inbound + outbound + settings + triggers + debounce
   DONE (2026-07-08). **Remaining:** caption/title inbound (blocked on sparse observation writer),
   `alexandria:Flag` custom namespace (best-effort, OQ #8).

**Explicitly NOT needed for the ingest milestone:** dependency fleet (pure-Go covers JPEG/PNG/GIF),
grouping engine (derived state, backfillable — ingest only writes `sidecar_files`), watcher, XMP
sync, machine.json (hardcode default pool sizes), all frontend work, River/job persistence.

## After ingest ships

Watcher service (`impl/05`) → XMP sync (`impl/06`) → dependency fleet (`impl/07`) as formats demand.
In parallel or after: the two design rounds never held — **query layer** and **the seam** — see
`04-open-questions.md`; both are now heavily pre-shaped by the 2026-07-07 frontend round
(`../seam/`, `../frontend/`, `../CONSTANTS.md`). UI runtime is RESOLVED: Wails v2 (Ari,
2026-07-07).

## House rules that govern all implementation

1. **Ponytail discipline**: laziest thing that works; stdlib first; interfaces carved at the
   *second* implementation, never speculatively; shortest diff wins. Deliberate shortcuts get a
   `ponytail:` comment naming the ceiling and upgrade path.
2. **One cook**: every catalog mutation flows through the pipeline's single writer. Watcher,
   reconciler, volume monitor are *sensors* emitting hints; they never write.
3. **Writer classes are law** (see `03-data-model.md`): observation writers never touch judgment
   columns and vice versa. This is enforced by interface shape, not convention.
4. **Events are hints, not facts**: any file event just means "go look"; truth is re-derived
   from the filesystem via the identity matrix.
5. **Derived state must carry a rebuild path**: anything computed (FTS, thumbnails, auto-groups)
   is deletable + recomputable via a registered rebuild function.
6. Go engine, React UI, SQLite catalog are fixed. Wails v2 is fixed (2026-07-07).
7. Go conventions: per-OS build-tagged files inside the owning package (no shared `platform`
   package); explicit central registry tables (no `init()` self-registration); interfaces for
   varying behavior, generics for varying data.
