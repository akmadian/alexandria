# Alexandria — Agent Instructions

Local-first DAM for creative professionals. Go engine + React UI + SQLite catalog + Wails v2.

## Design authority (read before designing or implementing anything)

1. **`docs/CONSTANTS.md`** — cross-cutting load-bearing invariants (C1–C15: vocabulary, state
   equation, seam rules, registry rules). Applies to every area.
2. **`docs/decisions.md`** — the append-only decision log (D-numbers). It **wins every
   conflict** with older docs and existing code. D27 defines the docs system itself.
3. **`_project-tracking/00-START-HERE.md`** — how work is tracked (state = directory:
   `ideation/ → epics/ → tasks/ → deleted`; the queue is `ls tasks/` in NN order). Work items
   are transient; never cite one as an authority.
4. `_project-tracking/functional-requirements.md` — the single source of truth for the
   feature roadmap (P0–P4): what will be built, and when.
5. Living reference: `docs/` (data model, seam contract, vocabulary, requirements distilled)
   plus package READMEs beside the code (`internal/importer/README.md` for the ingest engine,
   `internal/enrichment/README.md` for the enrichment engine, `internal/xmp/README.md` for
   sidecar sync). Where code and current specs conflict, specs
   win. `frontend/src/` predating the ground-up redesign (2026-07-08, the frontend-redesign
   epic) was disposable by decision; the rebuild now underway replaces it.
6. **Anything visual:** `docs/design-constitution.md` is the design law (§1–§30), and the
   token source `frontend/design/` is the product it governs (D29). Design sessions start at
   `frontend/design/CLAUDE.md`; code consumes the system per `frontend/CLAUDE.md` §3. The
   rendered app currently serves a frozen legacy token snapshot — never learn design values
   from running code.

## Commands

- **`make check`** (repo root) — runs all backend + frontend checks. Must pass before commit.
- **`make check-backend`** / **`make check-frontend`** — run one side only.
- Individual backend steps (all from the root Makefile — there are no subdirectory Makefiles):
  `make tidy-check-backend` / `build-backend` / `lint-backend` / `vulncheck-backend` /
  `test-backend` / `cover-backend` (coverage gate).
- Dev harness: `go run ./cmd/dev import <path>` (`--catalog <dir>` to browse the DB, `--debug`
  for pprof; also `watch`, `errors`, `sessions`, `rebuild fts`)

## Architecture invariants — violating any of these is a bug, not a style choice

These are the system-level rules that no single file's code review would catch. The full
rationale lives in `docs/decisions.md` and `docs/data-model.md`.

- **Writer classes:** ingest/watcher code writes observation columns only; judgment columns
  (rating/label/flag/note/deletes) are written only by the user-action path, which is the ONLY
  place `judgment_modified_at` is bumped. XMP sync writes judgment values but never that
  timestamp. Enforced by the `catalog` interfaces — inject the narrowest writer, never widen one.
- **One cook:** every catalog mutation flows through the pipeline's single writer goroutine /
  repo transaction. Watcher, reconciler, volume monitor are sensors emitting hints — never writers.
- **Detect-and-flag (D20):** reconciliation never auto-mutates identity. Same-path fidelity is
  automatic; a file at a new path is a new asset + a pending review row. Never auto-relink or
  auto-merge.
- **One query authority (impl/13):** every predicate over assets compiles through
  `internal/ast` (`ast.Query` → the `Compile*` family). Never hand-write asset WHERE/ORDER BY
  fragments in repos; a new filterable capability is a new vocabulary field + compiler entry,
  never a new query method (C7). `ast` stays pure — no I/O, `now` is a parameter, and it
  imports `domain` only for enum membership (validate/compile only). This governs USER
  predicates; engine-internal plumbing (the enrichment missing-artifact scan in
  `sqlite.EnrichmentRepo`) is a sanctioned separate lane — see the D28 dated note.
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
- Prefer stdlib/an existing dep, but a well-maintained dependency that carries real load is
  welcome — the rule bans *redundant* dependencies, not dependencies.
- Shared vocabularies/shapes are declared once in Go and generated everywhere (C15):
  `cmd/generate` is the schema compiler (`make generate`); hand-written parallel definitions
  are a defect. Concepts: `docs/vocabulary.md`; inventory: `docs/data-dictionary.md`.
- Settings live in the three JSON files owned by `internal/settings` — never a DB table.
- Docs discipline (D27): status is never written down — no ✅/DONE prose, no ledgers; work
  items are deleted on completion after folding residue into `docs/` + the decision log.
  Durable docs never point at `_project-tracking/` work items. `make check-docs` (pre-commit
  hook + CI) enforces the mechanical half of this.

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

## Acceptance criteria

**`make check` must pass before any commit.** This is not optional — a failing check run is a
blocking defect, same as a broken test. CI runs the same Makefiles, so local and remote are
identical. If a lint finding or test failure exists in the diff, fix it before presenting work
for review.
