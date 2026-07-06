# Alexandria v2 Design Handoff — START HERE

**Date:** 2026-07-06
**Produced by:** a system-design working session (Ari + Claude Fable) that worked backwards from
`docs/functional-requirements.md`, interview-style. All decisions below were made deliberately,
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
| `impl/01-schema-rework.md` | **Blocker 1** — rewrite migration 0001 (spec-complete, start here) |
| `impl/02-repos-and-dbtx.md` | **Blocker 2** — transaction seam + writer-scoped repositories |
| `impl/03-type-registry-and-classifier.md` | **Blocker 3** — consolidate type dispatch + magic-byte classifier |
| `impl/04-ingest-pipeline.md` | **The milestone** — the six-stage pipeline, spec-complete |
| `impl/05-watcher-service.md` | Next milestone after ingest (design complete, do not start early) |
| `impl/06-xmp-sync.md` | Milestone after watcher (design complete) |
| `impl/07-dependency-fleet.md` | External-tool supervisor (needed before deep metadata / RAW / video) |
| `impl/08-dev-harness.md` | `cmd/dev` — runnable harness + debug server (pprof/expvar/statsviz/`/state`); build with impl/04 |

## Where the project is right now

**Code that exists** (all subject to modification per these docs):

- `internal/domain/` — domain types, file-type table, Opt[T] patch primitive. Sound, keep.
- `internal/migrations/` — migrator + `0001_initial_schema.sql`. Migrator is sound; **0001 gets
  rewritten in place** (pre-release, zero real catalogs exist — edit, don't stack migrations).
- `internal/sqlite/` — asset/source/duplicate repos. Functional but pre-date the DBTX seam and
  writer-split; `impl/02` reworks them.
- `internal/importer/` — a **sequential** single-threaded importer with the right stage
  factoring (scan/hash/classify/extract/thumbnail/persist as separate funcs) and the right
  identity matrix core. It is the seed of the real pipeline, not a throwaway.
- `internal/metadata/`, `internal/thumbnailer/` — per-MIME registries (pure-Go raster formats
  only). Get folded into the unified type registry (`impl/03`).
- `internal/main.go` — an in-memory smoke test, says so in comments. Not an app.
- `frontend/` — React shell, mock API, and `frontend/src/api/contract.ts` — the designed seam.
  **The contract is deliberately network-shaped** (async, DTOs, events, binary-by-URL); read it
  before touching the engine's public surface, but do not do frontend work yet.

**An earlier audit of this codebase** (same session) found and the design absorbed: FTS5 created
but never populated; ORDER BY SQL injection via raw sort field; no transactions in the write path;
reimport clobbering user edits; missing FK delete rules; the soft-delete unique-index trap. All are
fixed by `impl/01` + `impl/02` + `impl/04`.

## The immediate path (blocking order)

1. `impl/01` schema rework (~1 day)
2. `impl/02` DBTX + writer-split repos (~1 day)
3. `impl/03` type registry + classifier (~½ day)
4. `impl/04` ingest pipeline (the milestone; includes minimal job envelope and real `openCatalog`)

**Explicitly NOT needed for the ingest milestone:** dependency fleet (pure-Go covers JPEG/PNG/GIF),
grouping engine (derived state, backfillable — ingest only writes `sidecar_files`), watcher, XMP
sync, machine.json (hardcode default pool sizes), all frontend work, River/job persistence.

## After ingest ships

Watcher service (`impl/05`) → XMP sync (`impl/06`) → dependency fleet (`impl/07`) as formats demand.
In parallel or after: the two design rounds never held — **query layer** and **the seam** — see
`04-open-questions.md`. UI runtime selection (Wails v2/v3 vs Tauri vs Electron) is unresolved and
blocks only frontend work.

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
6. Go engine, React UI, SQLite catalog are fixed. Wails is *not yet* fixed.
7. Go conventions: per-OS build-tagged files inside the owning package (no shared `platform`
   package); explicit central registry tables (no `init()` self-registration); interfaces for
   varying behavior, generics for varying data.
