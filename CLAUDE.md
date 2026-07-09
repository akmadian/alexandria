# Alexandria — Agent Instructions

Local-first DAM for creative professionals. Go engine + React UI + SQLite catalog + Wails v2.

## Design authority (read before designing or implementing anything)

1. **`_project-tracking/CONSTANTS.md`** — cross-cutting load-bearing invariants (C1–C14:
   vocabulary, state equation, seam rules, registry rules). Applies to every area.
2. **`_project-tracking/00-START-HERE.md`** — the master head of the implementation task
   tree: what's next right now, and links to the per-area trackers (`backend/`, `seam/`,
   `frontend/` — each has its own `00-START-HERE.md`). The backend decision log
   (`backend/02-decision-log.md`) **wins every conflict** with older docs and existing code.
3. `_project-tracking/functional-requirements.md` — the single source of truth for the
   feature roadmap (P0–P4): what will be built, and when.
4. Existing code follows the disposition tables (backend `.../05-code-disposition.md`, frontend
   `.../frontend/07-code-disposition.md`): specs win; you have explicit license to delete what
   they mark deleted.
5. `internal/importer/README.md` (ingest engine, impl/04) and `_project-tracking/perf/`
   (thumbnail/hardware acceleration) are the up-to-date implementation references for the pipeline.

## Commands

- Backend: `go test -race ./...` · `go vet ./...` (run from repo root)
- Frontend: `bun run check` (in `frontend/` — typecheck + lint + tests; must pass before commit)
- Dev harness: `go run ./cmd/dev import <path>` (`--catalog <dir>` to browse the DB, `--debug`
  for pprof; also `watch`, `errors`, `sessions`, `rebuild fts`)

## Architecture invariants — violating any of these is a bug, not a style choice

These are the system-level rules that no single file's code review would catch. The full
rationale lives in `_project-tracking/backend/02-decision-log.md` and `03-data-model.md`.

- **Writer classes:** ingest/watcher code writes observation columns only; judgment columns
  (rating/label/flag/note/deletes) are written only by the user-action path, which is the ONLY
  place `judgment_modified_at` is bumped. XMP sync writes judgment values but never that
  timestamp. Enforced by the `catalog` interfaces — inject the narrowest writer, never widen one.
- **One cook:** every catalog mutation flows through the pipeline's single writer goroutine /
  repo transaction. Watcher, reconciler, volume monitor are sensors emitting hints — never writers.
- **Detect-and-flag (D20):** reconciliation never auto-mutates identity. Same-path fidelity is
  automatic; a file at a new path is a new asset + a pending review row. Never auto-relink or
  auto-merge.
- **Events are hints, not facts:** a file event means "go re-examine this path"; truth is
  re-derived from the filesystem via the identity matrix.
- **Derived state carries a rebuild path:** anything computed (FTS, thumbnails, auto-groups,
  signal columns) must be deletable + recomputable via a registered rebuild function.
- **Pre-release schema policy:** edit `internal/migrations/0001_initial_schema.sql` in place;
  do NOT stack migrations until a real release exists.
- **External binaries via the `dependency` package** — subprocesses, never cgo; no silent
  downloads (user-consented fetch with integrity verification only).
- **Registries, not hierarchies:** one explicit table per dispatch concern (`assettype`,
  actions, settings files); add a capability = add a row. No `init()` self-registration.

## Repo conventions

- IDs via `domain.NewID()` (UUIDv7), never inline `uuid.NewString()`.
- Per-OS code = build-tagged files inside the owning package; no shared `platform` package.
- No new third-party dependency where stdlib or an existing dep works.
- Settings live in the three JSON files owned by `internal/settings` — never a DB table.

## Coding standards

**`docs/coding-guidelines.md` is the authority for how Go is written here — read it before
writing any.** Package layout and the sanctioned `domain` exception (§0, §5), pure-core vs
orchestration (§1), interfaces at boundaries only (§3), error + logging discipline (§4),
pipeline stage layout (§6), tests (§8), naming (§9), seam method shape (§10). Two rules worth
flagging because violations are treated as review defects, not style nits:

- **Names are spelled in full — no abbreviations** (§9; only `i/j/k`, `err/ctx/ok/id/db/tx`,
  and short method receivers are sanctioned).
- **Logging ships with the flow, not after** (§4): milestones/results at `Info`, per-item
  play-by-play at `Debug`. A flow that completes work while logging nothing is a defect.

Frontend rules: `frontend/CLAUDE.md`.
