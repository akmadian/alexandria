# impl/14 — Seam Bindings & Generation Harness

**Status: spec ready (2026-07-09), not started.** First of the three seam-round docs
(14 → then 15 ∥ 16). Numbering continues the project-wide impl sequence (backend owns 01–13);
these live in `seam/impl/` because the seam is its own area.

**Scope:** the machinery every other seam doc depends on — the Wails v2 composition root at the
repo root, the `internal/seam` package skeleton, the TS generation pipeline (Wails bindings +
the hand-rolled vocabulary generator), and the Makefile/CI modularity that keeps backend,
frontend, and whole-app checks independent.
**Blocked by:** nothing (impl/13 landed 2026-07-08). **Blocks:** impl/15 (method surface),
impl/16 (events & jobs), the frontend rebuild, impl/12 (app host grows this root).
**References (read FIRST, in order):** `../00-START-HERE.md` (what the seam is),
`../01-queries-and-commands.md` (the contract + reconciliation ledger),
`../../CONSTANTS.md` C6/C7/C8/C13, `../../backend/impl/12-app-host.md` §Pre-design notes
(structure research + Wails idioms), `../../frontend/09-ground-up-redesign-notes.md`
(§module structure, §token & AST drill-in — the generated-union consumer).

## 1. The problem

Two descriptions of the same boundary disagree: the Go engine surface (`internal/catalog` +
`internal/ast`, real and tested) and `frontend/src/api/contract.ts` (design-authoritative but
pre-AST, with hand-written `models/`). Nothing binds them: no host process, no generated types,
no event bridge. C13 says Go types are the single source of truth and TS is generated — this
doc builds the generator and the host; 15 and 16 fill in the surface.

## 2. Locked decisions (2026-07-09, Ari — do not relitigate)

1. **Root scaffolding, grafted from a `wails init` template.** Wails v2 requires the main
   package at the project root (upstream declined `cmd/` layouts, wails issue #2568): the root
   gains `main.go`, `app.go`, `wails.json`, `build/`. Follow Wails conventions exactly —
   `build/` keeps its name (it is hand-editable *input*: icons, `Info.plist`, platform
   manifests; only `build/bin/` is output and gitignored). `cmd/dev` stays as the throwaway
   harness; multiple mains are fine, only Wails's must be at root.
2. **Bound services live in `internal/seam`** — not a top-level directory, not `app.go`.
   Root `app.go` stays thin: construct engine services, construct seam structs, hand them to
   `Bind:`. Generated JS mirrors the package path (`wailsjs/go/seam/…`).
3. **This root IS the impl/12 app host, seeded minimal.** Startup here = resolve catalog dir →
   instance lock → open SQLite → `migrations.Migrate` → wire services → bind → ready. Nothing
   more — integrity check, backup-before-migration, watcher supervision, exit machinery are
   impl/12's, grown in place. There is no throwaway skeleton.
4. **Three independent test surfaces** (Ari: "don't want to run the entire wails build and
   bundle for simple backend tweaks"):
   - `make check-backend` — engine only, **must not require Wails/webkit toolchains**. Go
     targets scope to `./internal/... ./cmd/...`, excluding the root app package.
   - `make check-frontend` — unchanged (bun; mock-backed; typechecks against *committed*
     generated types, no Go toolchain needed).
   - `make check-app` (new) — compiles the root Wails package (`-tags webkit2_41` on Linux;
     apt deps already in ci.yml as of 2026-07-09) and runs the seam-freshness gate. Its own
     path-filtered CI job.
5. **TS generation is split in two, both outputs committed:**
   - **Wails generates** method bindings + struct models (`frontend/wailsjs/`) at
     `wails dev`/`build` time — its native mechanism, plus `EnumBind` for domain enums.
   - **A hand-rolled generator** (no tygo — rejected: we'd post-process its conventions
     anyway) emits what Wails can't shape: `TokenField`/`TokenOperator` literal unions, the
     per-field grammar table (field → operators → value kind), and — for impl/16 — the event
     topic/type unions. It is a small Go program that **imports `internal/ast`** and prints TS,
     so it compiles against the source of truth and cannot drift.
6. **The generator is NOT hooked into `wails build`.** It runs via `go:generate` /
   `make generate-seam`; output is committed; CI enforces freshness (regenerate +
   `git diff --exit-code`). Rationale: regeneration is only needed when the Go vocabulary
   changes (a backend edit — the editor reruns it, the gate catches forgetting); the frontend
   must remain checkable without a Go toolchain; and a build step that mutates the tree is a
   footgun.
7. **Wails v2 now; migrate to v3 when it stabilizes.** The thin-root + `internal/seam` split is
   what keeps that migration a composition-root rewrite.

## 3. Work items

1. Scratch-dir `wails init`, graft `main.go`/`app.go`/`wails.json`/`build/` into the repo root;
   adapt `wails.json` to bun + vite (`frontend:install/build/dev:watcher`, dev server URL).
   `.gitignore` += `build/bin`, `frontend/dist`; `.gitattributes` += `frontend/wailsjs/**
   linguist-generated=true`.
2. `internal/seam` skeleton + **one walking-skeleton method** bound end-to-end (recommend
   `ListSources` — real repo, trivial shape): Go method → wails binding → generated TS →
   called from a scratch frontend page under `wails dev`. Proves the whole pipe before 15/16
   invest in it.
3. `EnumBind` wiring for the domain enums the contract re-exports (FileType, ColorLabel, Flag,
   FileStatus, …).
4. The vocabulary generator: emits to `frontend/src/_generated-types/` (the module slot
   frontend/09 reserved). Deterministic output (sorted, header comment naming the source
   package and forbidding hand edits).
5. Makefile: `generate-seam`, `check-app` (build root w/ tag + freshness gate); scope existing
   backend targets to `./internal/... ./cmd/...`. CI: new path-filtered `app` job
   (`main.go`, `app.go`, `wails.json`, `internal/seam/**`, `internal/ast/**`,
   `frontend/wailsjs/**`, `frontend/src/_generated-types/**`).

## 4. Decisions to make DURING implementation (pre-scoped, none blocking)

| Decision | Recommendation |
|---|---|
| Generator home | `internal/seam/generate/` as a `go run`-able main, invoked by a `//go:generate` directive in `internal/seam` — keeps it beside its subject; `cmd/` stays user-facing. |
| Wails `ts_generation.outputType` | `interfaces` — frontend/09 wants plain data shapes, not classes. |
| Union naming map | `ast.Field`/`ast.Operator` values → TS literal members verbatim (they're already camelCase strings); only the *type names* map (`Field`→`TokenField`, `Operator`→`TokenOperator`). |
| wailsjs regeneration in `check-app` | Run `wails generate module` (or a dev-mode build) and diff, if invocation proves cheap; otherwise freshness covers only the hand-rolled generator and wailsjs drift is caught at the next `wails dev`. |

## 5. Acceptance

- `wails dev` serves the vite frontend with working hot reload; `wails build` produces a
  runnable app on macOS.
- Walking-skeleton method returns real catalog data in the webview; its generated TS signature
  typechecks.
- Generated unions compile; `satisfies Record<TokenField, …>` over a dummy registry literal
  compiles (the frontend/09 completeness mechanism works against generated types).
- Generator determinism: two runs, byte-identical output. Freshness gate fails when a
  vocabulary field is added without regenerating, passes after.
- **Toolchain isolation proven:** `make check-backend` passes in an environment without
  gtk/webkit headers; `make check-frontend` passes without a Go toolchain.
- CI green across backend / frontend-freshness / app jobs, each triggered only by its paths.

## 6. Doc maintenance on landing (same change)

- Master head: frontier row → impl/15 + impl/16 unblocked (parallel picks).
- `../00-START-HERE.md`: sequencing section updated (bindings live, generation live).
- `../../backend/impl/12-app-host.md`: note the host now exists; startup-sequence design round
  is next trigger.
- This file: status block → what shipped + deviations.
