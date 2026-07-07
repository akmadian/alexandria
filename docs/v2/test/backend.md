# Go backend — testing strategy notes

Handoff notes, not a finished spec — the goal is a comprehensive testing
strategy built from this starting point, not a prescription to follow
literally. Context: this repo already uses table-driven tests extensively
and has real binary fixtures in `testdata/` (`.jpg`, `.ORF`, etc.) used for
parsing tests. See also [docs/coding-guidelines.md](../../../coding-guidelines.md)
(the "accept `io.Reader`, return a struct" rule directly shapes what's unit-
testable here) and [ops/backend.md](../ops/backend.md) (CI/coverage/vulncheck
scripts this testing work plugs into).

## The core distinction that actually matters

Not "uses a real file" vs. "uses a fake" — **how many components are wired
together, and how much real mutable state gets touched** (filesystem writes,
DB writes, network, clock). A test that reads a real fixture file but only
exercises one pure function is still a unit test with good fixture data, not
an integration test. What makes something an integration test is multiple
collaborating packages being exercised together, typically with real I/O
side effects rather than just real input bytes.

## Three tiers in this codebase

**Tier 1 — pure parsing/transform, real fixtures, still unit tests**
`internal/metadata`, `internal/filetype`, XMP parsing. These fit the
`io.Reader` in → struct out shape already mandated by the coding guidelines.
Opening a real `testdata/*.ORF` file and passing its reader in is
deterministic and side-effect-free — no cleanup, nothing that can leak
between runs. Table-driven: `{fixture filename → expected struct}`.

- **Golden files** worth adopting here: for large/complex expected output
  (full EXIF tag sets), check in an expected-output file
  (`testdata/golden/<name>.json`) instead of hand-writing the struct, with a
  `-update` flag that regenerates it when behavior deliberately changes.
  Standard Go idiom (flag check + `os.WriteFile` when set), no library
  needed.

**Tier 2 — SQLite adapters, real but throwaway DB**
`internal/sqlite`. Mocking the DB layer just tests the mock, so use a real
SQLite instance, ephemeral per test (`:memory:` or `t.TempDir()`-backed
file). Still fast — no network, no server — so it stays in the normal
`go test ./...` run, not gated behind anything. `internal/testutil` is the
natural home for the fresh-DB-per-test helper if it isn't already.

**Tier 3 — actual integration tests**
`internal/importer`, and `internal/watcher` once built. Multiple packages
wired together for real: importer touching a real temp directory, writing
through to a real SQLite-backed catalog, asserting end state. Slow/stateful
enough to justify splitting out via `testing.Short()` (tests call
`t.Skip()` under `-short`) or a `//go:build integration` tag — `go test
-short` for the fast dev loop, plain `go test` (CI) runs everything.
`testing.Short()` is the simpler of the two (one flag, no file-splitting);
pick whichever this codebase ends up preferring, don't need both.

## Why the ports/adapters split matters for test cost

`catalog` (interface/ports) + `sqlite` (adapter) — per coding-guidelines
§"Name adapters by their dependency" — isn't just a navigation win, it
directly controls how much of the suite has to be slow. Importer
*decision-making* (what counts as a duplicate, what gets skipped) can be
unit-tested against a small in-memory fake implementing
`catalog.AssetRepository`, with real files still coming from `testdata/`.
Only the tests proving "the whole pipeline actually works end-to-end
against real SQLite and a real directory" need to pay Tier 3's cost. Keep
Tier 3 small and targeted; push as much orchestration-logic testing as
possible down into fake-backed unit tests.

## Open questions for the comprehensive strategy

- Coverage targets per tier — does the ~75-80% overall gate from
  `ops/backend.md` need per-package carve-outs (e.g. Tier 3 packages
  naturally lower, Tier 1 parsing packages naturally near 100%)?
- Fixture management as `testdata/` grows — naming convention, whether
  large/many binary fixtures need Git LFS eventually (not yet — flag as a
  watch item, not a current need).
- Flakiness policy for Tier 3 (concurrent import/watch paths) — retries,
  `-race` interaction, timeout conventions.
- Whether `internal/testutil` needs restructuring as fixture/helper needs
  grow across tiers.
