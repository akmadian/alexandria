# Go backend — repo hygiene & CI strategy

**Status: BUILT (2026-07-08, revised 2026-07-09).** This doc describes what is; the root
`Makefile` is the implementation. Running `make check-backend` locally is exactly what CI
runs — nothing should fail in the pipeline that it wouldn't have already caught. No git hooks
(deliberately skipped — annoying, make discipline covers it).

## Mechanism: one root Makefile

Went with Make over the originally-spec'd bash scripts: `make` is the dominant Go-community
convention (Kubernetes, Prometheus, Hugo) and is zero-install on every dev machine and CI
runner. One constraint added 2026-07-09 (Ari): **everything lives in the single root
`Makefile`** — Go only operates at the module root, so the first cut's per-directory Makefiles
(`internal/Makefile` doing `cd .. && go …`) were a fiction and were removed. `make check` runs
backend + frontend; `make check-backend` is the CI entry point; individual targets
(`tidy-check`/`build`/`lint`/`vulncheck`/`test`/`cover`) run standalone.

## Principle

Every check has two properties: **why it exists** (what class of bug/drift it catches that the
others don't) and **speed tier** (fail fast on cheap mistakes before burning time on slow
ones). `check-backend` runs cheap → expensive.

## Targets

**`tidy-check`** — `go mod tidy -diff`
Catches `go.mod`/`go.sum` drift (import added, tidy not run). Cheapest check, always first.

**`build`** — `go build ./...`
Catches compile errors in packages `go test` wouldn't touch (main packages, build-tag-gated
files, untested packages).

**`lint`** — `golangci-lint run ./...`
Everything vet doesn't catch, plus formatting (the v2 `formatters` section runs `gofmt` +
`goimports` as lint findings — no separate fmt-check step). The config (`.golangci.yml`) also
**mechanizes repo invariants**: `depguard` enforces domain-imports-stdlib-only, the
junk-drawer-package ban, and `internal/ast` purity; `forbidigo` bans `fmt.Print*` per
coding-guidelines §4. Version pinning: CI installs the exact version of the local Homebrew
install via `golangci-lint-action`'s `version:` field — findings differ across linter majors,
so the two must move together. Do NOT lower the module's `go` directive to satisfy an old
linter binary; bump the linter instead (learned 2026-07-09).

**`vulncheck`** — `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
Known CVEs that the code's call graph actually reaches, not every vuln in the module tree.
`@latest` on purpose: a vuln scanner wants the freshest database, not a pin. Proved itself the
day it was added — flagged GO-2026-5856 (crypto/tls, reachable via the pprof debug server and
the EXIF fetch path) and forced the go1.26.5 patch bump.

**`test`** — `go test -race -coverprofile=coverage.out ./...`
Race detector always on — SQLite + concurrent import/watch paths have races invisible in a
quick dev run that appear under CI's different scheduling.

**`cover`** — depends on `test`, so the suite runs ONCE (the coverprofile run *is* the test
run; no separate test step in CI). Prints the filtered total and gates it: ≥ `COVERAGE_MIN`
(70 at creation, measured 74.0% — **ratchet up as areas gain tests, never down**). Excludes
`cmd/dev` and `internal/testutil` (wiring and test support by design). `internal/migrations`
is deliberately NOT excluded — its 0% is a real gap that should stay visible.

**`check-backend`** — `tidy-check build lint vulncheck cover`, with a pass/fail banner.

## Test dependencies

The `xmp` and `dependency` suites `t.Skip` without exiftool. CI installs
`libimage-exiftool-perl` so those suites actually run — a green CI that silently skipped the
active milestone's tests would be worse than a red one. Any future tool-gated suite gets the
same treatment (install in CI, skip gracefully locally).

Fixture hygiene: camera-original fixtures (`testdata/exif-original.JPG`) get their serial
numbers stripped (`exiftool -SerialNumber= -LensSerialNumber= -InternalSerialNumber=
-overwrite_original`) before commit — the tests need EXIF structure, not device identifiers,
and the repo is public.

## CI

See [ci.md](ci.md) — trigger/orchestration is shared with the frontend and lives there.

## Explicitly out of scope

- **Git hooks** — skipped by request. Revisit only if `make check` drift (pushing without
  running it) becomes an actual recurring problem.
- **Codecov/external coverage services** — the Makefile gate needs no account/token; add an
  external service only if trend dashboards become a real need.
- **Complexity linters** (`cyclop` etc.) — add only when a specific PR shows a concrete need.
