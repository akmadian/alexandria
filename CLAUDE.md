# Alexandria — Agent Instructions

Local-first DAM for creative professionals. Go engine + React UI + SQLite catalog.

## Design authority (read before designing or implementing anything)

1. **`docs/project-tracking/CONSTANTS.md`** — cross-cutting load-bearing invariants (C1–C14:
   vocabulary, state equation, seam rules, registry rules). Applies to every area.
2. **`docs/project-tracking/00-START-HERE.md`** — the master head of the implementation task
   tree: what's next right now, and links to the per-area trackers (`backend/`, `seam/`,
   `frontend/` — each has its own `00-START-HERE.md`). The backend decision log
   (`backend/02-decision-log.md`) **wins every conflict** with older docs and existing code.
3. `docs/functional-requirements.md` — feature source of truth (P0–P4).
4. Existing code follows the disposition tables (backend `.../05-code-disposition.md`, frontend
   `.../frontend/07-code-disposition.md`): specs win; you have explicit license to delete what
   they mark deleted.
5. `internal/importer/README.md` (ingest engine, impl/04) and `docs/perf/` (thumbnail/hardware
   acceleration) are the up-to-date implementation references for the pipeline.

## Commands

- Backend: `go test -race ./...` · `go vet ./...` (run from repo root)
- Frontend: `bun run check` (in `frontend/` — typecheck + lint + tests; must pass before commit)
- Dev harness: `go run ./cmd/dev import <path>` (`--catalog <dir>` to browse the DB, `--debug` for pprof)

## Hard rules — violating any of these is a bug, not a style choice

- **Writer classes:** ingest/watcher code writes observation columns only; judgment columns
  (rating/label/flag/note/deletes) are written only by the user-action path, which is the ONLY
  place `judgment_modified_at` is bumped. XMP sync writes judgment values but never that timestamp.
- **One cook:** every catalog mutation flows through the pipeline's single writer goroutine /
  repo transaction. Watcher, reconciler, volume monitor are sensors emitting hints — never writers.
- **Events are hints, not facts:** file events mean "go re-examine this path"; truth is re-derived
  via the identity matrix.
- **Derived state carries a rebuild path:** anything computed (FTS, thumbnails, auto-groups) must
  be deletable + recomputable via a registered rebuild function.
- **Pre-release schema policy:** edit `internal/migrations/0001_initial_schema.sql` in place; do
  NOT stack migrations until a real release exists.
- IDs via the shared helper (UUIDv7), never inline `uuid.NewString()`.
- `internal/domain` imports stdlib only. No `utils`/`helpers`/`models`/`common` packages, ever.
- Interfaces are carved at the *second* implementation — no speculative abstraction.
- **Names are spelled in full — no abbreviations.** `extractedMetadata` not `md`, `scanned` not `sf`,
  `relativePath` not `relPath`. Only sanctioned short names: loop indices `i/j/k`; `err`/`ctx`/`ok`/
  `id`/`db`/`tx`; short method receivers (a *local* of the same type is still spelled out). This
  applies to struct fields, params, and locals alike. See coding-guidelines §9.
- Per-OS code = build-tagged files inside the owning package; no shared `platform` package.
- Pipeline channels are created/wired/closed in ONE function; stages take directional channel params.
- External binaries via the `dependency` package (subprocesses, never cgo); no silent downloads.
- No new third-party dependency where stdlib or an existing dep works.

- **Log comprehensively — add logging with the flow, not after.** A running system must narrate
  itself: lifecycle boundaries, workflow *results* (verdict/counts/ids), and state transitions at
  `Info`; per-event/per-item play-by-play at `Debug`; recoverable per-file failures at `Warn`;
  serious ones at `Error`. A flow that completes work while logging nothing is a defect, same as a
  missing test. Don't optimize for a quiet clean run — optimize for a readable narrative. See
  coding-guidelines §4 for the level rubric.

## Detailed standards

`docs/coding-guidelines.md` — package layout, pure-core/orchestration split, error/logging/test
conventions. Read it before writing Go. Frontend rules: `frontend/CLAUDE.md`.
