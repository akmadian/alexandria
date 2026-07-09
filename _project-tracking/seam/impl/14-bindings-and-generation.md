# impl/14 — Seam Bindings & Generation Harness

**Status: ✅ DONE (2026-07-09).** First of the three seam-round docs (14 → then 15 ∥ 16).
Numbering continues the project-wide impl sequence (backend owns 01–13); these live in
`seam/impl/` because the seam is its own area. **What shipped and the two deviations from the
locked decisions are recorded in §7 (bottom); read that before 15/16.**

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
     `wails dev`/`build` time — its native mechanism. ~~plus `EnumBind` for domain enums.~~
     **DEVIATION (2026-07-09): no EnumBind** — see §7.①. Wails `EnumBind` emits a TS `enum`,
     but frontend/09 mandates string-literal unions; and its `{Value, TSName}` manifest leaked a
     Wails shape into `internal/domain`. Domain enums moved to the hand-rolled generator.
   - **A hand-rolled generator** (no tygo — rejected: we'd post-process its conventions
     anyway) emits what Wails can't shape: `TokenField`/`TokenOperator`/`ValueKind` literal
     unions + the per-field grammar table from `internal/ast`, **and the domain-enum unions**
     (`FileType`, `ColorLabel`, …) — the members **discovered by type-checking `internal/domain`
     via `go/packages`**, so domain stays pure `type`+`const` with no list to drift (§7.①). It
     is a small Go program that compiles against the Go source of truth and cannot drift; impl/16
     extends it with the event topic/type unions.
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
3. ~~`EnumBind` wiring for the domain enums~~ → **the generator emits the domain-enum unions**
   (FileType, ColorLabel, Flag, FileStatus, SourceKind, SourceConnectivity), members discovered
   by type-checking `internal/domain` (§7.①).
4. The generator: emits to `frontend/src/_generated-types/` (the module slot frontend/09
   reserved) — `vocabulary.ts` (from `internal/ast`) + `enums.ts` (from `internal/domain`).
   Deterministic output (sorted, DO-NOT-EDIT header naming the source package).
5. Makefile: `generate-seam`, `check-generated` (freshness gate), `check-app` (build root w/
   tag); scope existing backend targets to `./internal/... ./cmd/...`. CI: path-filtered
   `frontend` + `app` jobs. **REFINEMENTS (2026-07-09), see §7.②:** the freshness gate lives in
   **`check-backend`**, not `check-app` — it is webkit-free and the drift is caused by Go edits,
   so it must run on the fast path. And CI is **three workflow files** (backend/frontend/app),
   with the webkit apt deps moved *off* the backend job so its green-without-them proves the
   isolation §5 demands.

## 4. Decisions to make DURING implementation (pre-scoped, none blocking)

| Decision | Recommendation |
|---|---|
| Generator home | `internal/seam/generate/` as a `go run`-able main, invoked by a `//go:generate` directive in `internal/seam` — keeps it beside its subject; `cmd/` stays user-facing. |
| Wails `ts_generation.outputType` | `interfaces` — frontend/09 wants plain data shapes, not classes. |
| Union naming map | `ast.Field`/`ast.Operator` values → TS literal members verbatim (they're already camelCase strings); only the *type names* map (`Field`→`TokenField`, `Operator`→`TokenOperator`). |
| wailsjs regeneration in `check-app` | **RESOLVED: freshness covers only the hand-rolled generator.** `wails generate module` runs the app's `OnStartup` (it opened the catalog + wrote a log during a trial), so it is neither cheap nor side-effect-free — unsafe as a CI gate. `wailsjs/` drift is caught at the next `wails dev`/`build`. |

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

## 6. Doc maintenance on landing (same change) — ✅ DONE 2026-07-09

- [x] Master head: frontier → impl/14 DONE, impl/15 ∥ impl/16 the picks; CI-round row (C) note
  updated (frontend + app jobs now exist).
- [x] `../00-START-HERE.md`: sequencing section updated (impl/14 DONE, 15∥16 frontier).
- [x] `../../backend/impl/12-app-host.md`: §1 marks the host built by impl/14; trigger → the
  startup-sequence design round.
- [x] This file: status block + §7 (shipped + deviations).

## 7. What shipped + deviations (2026-07-09)

Built to spec: Wails composition root (`main.go`/`app.go`/`wails.json`/`build/`), `internal/seam`
with `ListSources` bound end to end, generated `wailsjs/` (committed, linguist-generated), the
hand-rolled generator at `internal/seam/generate` → `frontend/src/_generated-types/`,
`internal/app` (one-dir app home `~/.alexandria` + per-run timestamped logging, macOS
Console.app symlink), `internal/logging` shared constructor, and the Makefile scoping. Two
deviations from the §2 locked decisions, both raised by Ari during review:

**① No EnumBind; domain enums flow through the hand-rolled generator, members *discovered*.**
`EnumBind` (§2-5, §3-3) is out. Reasons, in order of force: (a) it emits a TS `enum`, which
frontend/09 explicitly forbids (literal unions only); (b) its `{Value, TSName}` slice is a Wails
shape, and putting it in `internal/domain` is scope leak; (c) any hand-authored member list is a
second sync surface — the recurring smell. Resolution: the generator loads `internal/domain` with
`go/packages` and enumerates the string constants of each named enum type, so the **consts are the
single source of truth** — `internal/domain` is byte-for-byte unchanged (pure `type`+`const`),
adding a const auto-surfaces, and a renamed/removed type fails the generator loudly. The generator
holds only a manifest of *which type names* to publish, not their members. Cost: `golang.org/x/tools`
promoted from transitive to a direct dep (generator-only; not in the app binary). `Source.Kind`
stays `string` at the wire, which is what frontend/09 locked ("loose at the wire, strict at the
constructor"). C13 holds — TS still generated from Go, just via our generator not Wails for enums.

**② Freshness gate on the backend path + CI as three workflow files.** §2-4 put the freshness
gate in `check-app`; it moved to **`check-backend`** (target `check-generated`) because it is
webkit-free and the drift is caused by editing `internal/ast`/`internal/domain` — gating that
behind the app toolchain means the person who breaks it never runs it. `check-app` re-runs it too
(so an app-path trigger also verifies it). CI is `ci.yml` (backend) + `ci-frontend.yml` +
`ci-app.yml`, each natively path-filtered; the gtk/webkit apt deps moved from the backend job to
the app job, so the backend job's green-without-them *is* the isolation proof (§5). The app job
stubs `frontend/dist` for the `//go:embed` (compile check, not asset bundling — that is
`wails build`), keeping it Go+webkit only, no bun.

**Enforcement summary (how drift is caught):** generator unit tests (determinism, no-`enum`-leak,
sorted) + the `go/packages` fatal-on-empty guard + the `check-generated` freshness gate (Go→TS) +
the frontend `satisfies Record<TokenField, FieldGrammar>` compile gate (C10) + `check-app`
compiling the root. Hand-edits to the generated files are caught from the app side; Go vocabulary
edits from the backend side.
