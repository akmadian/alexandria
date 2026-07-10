# Review checklists, by area

For the reviewer subagent. These deliberately EXCLUDE everything `make check` mechanizes
(gofmt/goimports, depguard purity, junk-drawer ban, forbidigo, varnamelen short names,
exhaustive switches, coverage gate, generated-TS freshness) — do not re-litigate those.

## Common — every review

- **Scope fidelity.** The diff matches the scope confirmed at task-pickup: nothing the spec
  requires is missing; nothing landed that wasn't in scope (fold-ins must have been agreed).
- **Doc maintenance in the same change.** Procedure: open the spec's doc-maintenance section
  and check each doc it names appears in the diff, item by item; independently verify the
  spec's own status block, the area tracker, the master head
  (`_project-tracking/00-START-HERE.md` — frontier + "Last updated"), and any ledger rows
  (DEFERRED triggers, seam reconciliation ledger, open questions the work answers). Absence
  is the failure mode here — you cannot find a missing doc update by reading the hunks that
  exist.
- **Logging ships with the flow** (guidelines §4). Evidence, not impression: list each
  new/changed flow (service start/stop, run, unit of work, state transition) beside the Info
  line it emits — a flow you cannot pair with a milestone line IS the finding. Then check
  Debug covers the per-item play-by-play, one line per event, key/values not struct dumps,
  greppable subsystem prefix. A flow that completes work in silence is a defect, same as a
  missing test.
- **Test adequacy — evidence required, never vibes.** This dimension has a mandatory
  procedure; "tests exist and `make check` is green" is not a review, because the coverage
  gate is an *aggregate floor* — 70.8% overall can hide core methods at 0%.
  1. **Run per-function coverage on the touched packages** and read the numbers for every
     file in the diff: `go test -coverprofile=cover.review.out ./internal/<pkg>/...` then
     `go tool cover -func=cover.review.out | grep <changed files>` (frontend:
     `bun run coverage`, read the per-file report). Delete the profile after.
     A new/changed non-trivial function at 0% is **Important** minimum; at 0% on a core path
     (data integrity, judgment writes, seam methods, matrix/identity logic) it is **Critical**.
  2. **Trace what each test actually executes.** Open the new tests and follow the call: does
     the assertion exercise production code, or does it assert values the test itself stubbed
     in? A test whose subject is a fake proves the fake works. Repo law (testing-strategy):
     SQLite adapters test against a real ephemeral DB (`testutil.NewTestDB`), never DB mocks —
     "mocking the DB layer just tests the mock." Fakes are legitimate only at the `catalog`
     interface seam, for orchestration-*decision* logic, and even then the assertions must
     target the decisions, not the fake's echoes. A service layer tested "almost entirely
     against a fake backend" is **Critical**.
  3. **Enumerate the branches of new logic** — error paths, empty/nil cases, boundaries — and
     check a test hits each. §8 form still applies: external `foo_test` package, plain
     `t.Fatalf`, `testutil` builders, fixtures in `testdata/`.
- **Naming in full** (§9). Lint catches `<3` chars; it does not catch `relPath`, `impMeta`,
  `cfgVal`. Only `i/j/k`, `err/ctx/ok/id/db/tx`, short receivers are sanctioned.
- **Lazy vs careless.** Deliberate shortcuts carry a `ponytail:` comment naming the ceiling and
  upgrade path. A shortcut without one is either careless or undocumented — both are findings.
  Conversely: speculative abstraction (interface with one implementation, config for a constant,
  scaffolding "for later") is a finding too — interfaces are carved at the *second*
  implementation.
- **Leftovers.** No debug prints routed around forbidigo, no dead code kept "just in case", no
  stray scratch files, no TODOs without an owner (a DEFERRED entry or a ponytail marker).

## Backend (Go engine)

Architecture invariants — a violation here is Critical, not style:

- **Writer classes (D8).** Ingest/watcher paths write observation columns only; judgment
  columns only via the user-action path, the sole place `judgment_modified_at` is bumped; XMP
  sync writes judgment values but never that timestamp. Evidence: for each new injection
  site, name the writer interface it receives and the columns written through it — a widened
  interface (or a struct field swapped from a narrow writer to `Repos`) is the finding.
- **One cook.** Every catalog mutation flows through the single writer goroutine / repo
  transaction. Watcher, reconciler, volume monitor emit hints; if the diff has one of them
  writing, that's the finding.
- **Detect-and-flag (D20).** No code path auto-mutates identity — no relink, no merge, no move
  heuristics. New-path content = new asset + pending review row. Test at every matrix branch:
  is identity being reshuffled? Then it must be a flag.
- **One query authority (impl/13, C7).** No hand-written asset WHERE/ORDER BY fragments in
  repos — predicates compile through `internal/ast`. Grep the diff's added lines for
  `WHERE`/`ORDER BY`/`Sprintf`-assembled SQL over assets outside `internal/ast`; a new
  predicate string in a repo file is the finding. A new filterable capability is a vocabulary
  field + compiler entry, never a new query method. `ast` stays pure: no I/O, `now` a
  parameter.
- **Events are hints**; truth is re-derived from the filesystem via the identity matrix.
- **Derived state carries a rebuild path** — new computed state (FTS, thumbs, signal columns)
  registers a delete+recompute function.
- **Schema policy.** Pre-release: `0001_initial_schema.sql` edited in place, no stacked
  migrations.
- **External binaries** through the `dependency` package — subprocess, never cgo, never a
  silent download.
- **Registries, not hierarchies/`init()`.** New capability = new row in the explicit table.

Guidelines discipline (Important unless noted):

- §0 packages by concern; a type lives with its one consumer, `domain` only if genuinely
  global. §1 pure computation split from orchestration — logic buried in a walk loop, or a
  helper taking a *path* where a reader would do, is the smell. §2 no package-level mutable
  state. §3 interfaces at I/O boundaries only. §4 no swallowed errors (`_ =` needs a comment).
  §5 `domain` methods pure — no Active Record. §6 stage transform separate from channel
  plumbing; `stage_<name>.go` files, not sub-packages. §7 stdlib over hand-rolling. §10 method
  name containing a predicate (`GetSharpAssets`) = an AST node escaping — Critical, it's C7.
- IDs via `domain.NewID()`; per-OS code build-tagged in the owning package; no new dependency
  where stdlib or an existing one works; settings only in the three `internal/settings` JSON
  files.

## Seam

- **C7 result-shape rule.** Every new bound method must justify a genuinely new result shape;
  writes go through `UpdateAssets(ids, patch)`-style closed patch structs, not per-field
  setters. Targets may be `{ids}` or `{scope, where, exceptIds}` — applied as one SQL
  statement, never an id-materialized loop.
- **C8 events.** One envelope `{topic, type, payload, timestamp}`; every emitted type exists in
  the constants catalog on both sides. Grep the diff for emit calls (`EventsEmit`, the seam
  bridge's emit): each event type must resolve to a catalog constant — a string literal at the
  call site is the bug. Events only for genuinely async facts; request/response stays
  synchronous typed calls.
- **C9 jobs.** All background work reports through the one Job envelope; a private progress
  path must justify itself in writing.
- **C13 generation.** Freshness and hand-edits under `_generated-types/` are mechanized
  (`check-generated`) — skip those. The un-mechanized half is yours: grep the frontend diff
  for hand-written TS types/interfaces that duplicate a Go model or enum outside
  `_generated-types/` — a parallel model type is the finding, wherever it lives.
- **Binary channel.** Bytes never cross IPC — thumbnails and files travel as URLs.
- **ApiError.** New methods map errors into the seam's error shape; no raw Go error strings
  leaking to the frontend.

## Frontend

- **Three state planes, never blurred.** Catalog view state in the one Zustand store (all
  mutations via the single `dispatch`; reducer invariants preserved — selection cleared on
  scope/filter change, kept on arrangement change, cursor id-anchored). Server state in
  TanStack only — **the store never holds borrowed server state**. Pre-paint chrome prefs only
  in localStorage.
- **Seam discipline.** All I/O in `api/`; components → feature hooks → TanStack → `api/`.
  ESLint only bans the two adapter modules — grep the diff for any other leak: Wails runtime
  imports (`wailsjs/`, `@wailsapp`) or fetch/IPC calls outside `api/`. Mutation payloads carry
  absolute values, never deltas.
- **Optimistic mutation (the five rules, frontend/09).** Cancel-on-mutate + invalidation gate;
  ONE ordered lane for mutations + undo/redo; undo/redo render pessimistically; optimism only
  for ids-shaped targets; failure reverts loudly. Reads: `retry: false` default, per-code
  opt-in; every query consumer renders an explicit error state.
- **Types.** No bare field/operator strings — generated literal unions; no TS `enum`; leaves
  through strict registry constructors; `validate()` gates persisted trees (unknown token →
  inert pill, never a crash or dropped predicate). `query-model/` stays pure — vocabulary
  passed in, never fetched.
- **Boundaries lint can't see.** `features/` never imports another feature (shared code moves
  down) — this is convention + review, the ESLint config says so; you are the enforcement.
  Grep changed files under `features/<x>/` for imports reaching `features/<y>` (aliased or
  relative). Registries dispatch, conditionals don't (C10): `satisfies Record<Key, Entry>` +
  one fallback path at the dispatch point.
- **UI.** RAC for chrome (aliased `Aria*`), bespoke content surfaces; CSS Modules + semantic
  tokens only (no raw hex, no primitive `--grey-*`); every state readable with the accent
  unset; motion CSS-first, transform/opacity only, gated by `useMotion()`.
- **C14.** No hardcoded display text — i18n keys, display registries for enums, `Intl` for
  dates/numbers/sizes.

## Docs

- **Status claims verified against code**, never inferred from another doc — anything marked
  pending that shipped (or vice versa) is a finding.
- **Master head contract.** Completing or reprioritizing a frontier item updates
  `00-START-HERE.md` in the same change, including "Last updated". Relative dates converted to
  absolute.
- **CONSTANTS.md is LOCKED.** A doc change that edits a C-rule to match drifted code is
  Critical — the code is what's wrong, or the rule gets reopened explicitly with date +
  rationale. C-numbers and D-numbers are never renumbered or reused.
- **Decision log wins.** New text contradicting a D-entry without superseding it explicitly
  (dated note in the D-entry, like D9/D10 → D20) is a finding.
- **Cross-references resolve.** Moved/retired docs leave no dangling links; ledgers
  (DEFERRED, seam reconciliation, open questions) stay consistent with the change.
