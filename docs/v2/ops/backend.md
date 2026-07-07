# Go backend — repo hygiene & CI strategy

Spec for implementation. Target: a `scripts/check.sh` where running it is
exactly what CI runs — nothing should fail in the pipeline that
`scripts/check.sh` wouldn't have already caught locally. No git hooks
(deliberately skipped — annoying, script discipline covers it).

Plain bash over Make/`just`: every check here is a one-off command, not a
real file-dependency graph (nothing in this repo needs Make's actual
differentiator, timestamp-based incremental rebuilds), so a directory of
`.sh` files avoids introducing a new dependency for no real gain. Every dev
machine and CI runner already has bash.

## Principle

Every check has two properties: **why it exists** (what class of bug/drift it
catches that the others don't) and **speed tier** (so `scripts/check.sh` fails
fast on cheap mistakes before burning time on slow ones). Order: cheap →
expensive.

## Tool pinning

Use Go 1.24+'s native `tool` directive in `go.mod` instead of a hand-rolled
`go install @version` step:

```
go get -tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.x.x
go get -tool github.com/vladopajic/go-test-coverage/v2@v2.x.x
go get -tool golang.org/x/vuln/cmd/govulncheck@latest
```

This records the tool + version in `go.mod`/`go.sum`, so `go mod tidy` keeps
it honest and CI/local always run the identical binary — no separate
version-pinning scheme to forget to update. Invoke via `go tool golangci-lint`
/ `go tool go-test-coverage` (no `$GOBIN`/PATH management needed).

## Steps

Each step below is `scripts/<name>.sh` (e.g. `mod-tidy-check` →
`scripts/mod-tidy-check.sh`).

### Fast tier

**`mod-tidy-check`** — `go mod tidy -diff`
Catches: `go.mod`/`go.sum` drift (import added, tidy not run). Fails on a
clean CI checkout with zero tolerance for drift. Cheapest check, always first.

**`fmt`** — `gofmt -w . && goimports -w .`
Dev-only convenience target: fixes formatting in place. Not run in CI.

**`fmt-check`** — `gofmt -l . goimports -l .`, nonzero exit if any output.
Catches: unformatted code. Split from `fmt` because CI must fail loudly, never
silently rewrite files.

**`vet`** — `go vet ./...`
Catches: real semantic bugs (bad Printf verbs, Mutex copies, unreachable
code). stdlib-only, no external binary, ~1s. Kept separate from `lint` so
this fast signal doesn't wait on the slower golangci-lint run.

### Medium tier

**`build`** — `go build ./...`
Catches: compile errors in packages `go test` wouldn't touch (main packages,
build-tag-gated files, untested packages).

**`lint`** — `go tool golangci-lint run`
Catches: everything vet doesn't — staticcheck, errcheck, gosec, unused,
bodyclose, goconst, gocritic. Superset of vet, slower, run after the cheaper
checks.

### Slow tier

**`test`** — `go test -race ./...`
Race detector always on, not optional — this repo has SQLite + concurrent
import/watch paths where races are invisible in a quick dev run but appear
under CI's different scheduling.

**`cover`** — `go test -race -coverprofile=coverage.out ./...` then
`go tool cover -func=coverage.out`
Produces the `coverage.out` artifact consumed by `cover-html` (human) and
`cover-check` (gate). Kept separate from `cover-check` so a coverage-gate
failure can be debugged against visible numbers instead of a bare pass/fail.

**`cover-html`** — `go tool cover -html=coverage.out -o coverage.html`
Local-only convenience, opens in a browser with uncovered lines highlighted.
Never part of CI.

**`cover-check`** — `go tool go-test-coverage --config .testcoverage.yml`
Threshold gate (~75-80% overall, per-package overrides as needed). Depends on
`cover` having produced `coverage.out`.

**`vulncheck`** — `go tool govulncheck ./...`
Official Go tool (`golang.org/x/vuln/cmd/govulncheck`), pinned via the same
`tool` directive mechanism as the linter. Catches known CVEs in dependencies
— but specifically only the ones your code's call graph actually reaches,
not every vuln that exists anywhere in the module tree, so it doesn't cry
wolf over unused code paths. Cheap enough to run alongside `lint`.

### Aggregate

**`check`** — runs, in order:
`mod-tidy-check fmt-check vet build lint vulncheck test cover-check`
The one command devs run before pushing and CI runs as its only step.

**`clean`** — removes `coverage.out`, `coverage.html`.

## Scripts (draft)

All scripts live in `scripts/`, start with `set -euo pipefail`, and are run
from the repo root. `check.sh` just calls the others in order and stops at
the first failure (bash's `set -e` + a plain sequence of commands gives this
for free — no need for a task-runner's dependency graph).

```bash
#!/usr/bin/env bash
# scripts/mod-tidy-check.sh
set -euo pipefail
go mod tidy -diff
```

```bash
#!/usr/bin/env bash
# scripts/fmt.sh — dev-only, fixes in place
set -euo pipefail
gofmt -w .
go tool goimports -w .
```

```bash
#!/usr/bin/env bash
# scripts/fmt-check.sh — CI: fail loudly, never rewrite
set -euo pipefail
unformatted=$(gofmt -l .; go tool goimports -l .)
if [ -n "$unformatted" ]; then
  echo "$unformatted"
  exit 1
fi
```

```bash
#!/usr/bin/env bash
# scripts/vet.sh
set -euo pipefail
go vet ./...
```

```bash
#!/usr/bin/env bash
# scripts/build.sh
set -euo pipefail
go build ./...
```

```bash
#!/usr/bin/env bash
# scripts/lint.sh
set -euo pipefail
go tool golangci-lint run
```

```bash
#!/usr/bin/env bash
# scripts/vulncheck.sh
set -euo pipefail
go tool govulncheck ./...
```

```bash
#!/usr/bin/env bash
# scripts/test.sh
set -euo pipefail
go test -race ./...
```

```bash
#!/usr/bin/env bash
# scripts/cover.sh — produces coverage.out, consumed by cover-html/cover-check
set -euo pipefail
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

```bash
#!/usr/bin/env bash
# scripts/cover-html.sh — local-only, never run in CI
set -euo pipefail
./scripts/cover.sh
go tool cover -html=coverage.out -o coverage.html
```

```bash
#!/usr/bin/env bash
# scripts/cover-check.sh
set -euo pipefail
./scripts/cover.sh
go tool go-test-coverage --config .testcoverage.yml
```

```bash
#!/usr/bin/env bash
# scripts/clean.sh
set -euo pipefail
rm -f coverage.out coverage.html
```

```bash
#!/usr/bin/env bash
# scripts/check.sh — the one command devs run before pushing, and what CI calls
set -euo pipefail
./scripts/mod-tidy-check.sh
./scripts/fmt-check.sh
./scripts/vet.sh
./scripts/build.sh
./scripts/lint.sh
./scripts/vulncheck.sh
./scripts/test.sh
./scripts/cover-check.sh
(cd frontend && bun run format:check && bun run check)
```

`chmod +x scripts/*.sh` once at creation time; invoke as `./scripts/check.sh`.

Note: `goimports` needs adding as a `tool` directive too
(`golang.org/x/tools/cmd/goimports`) alongside golangci-lint and
go-test-coverage.

## `.golangci.yml` (v2 schema)

```yaml
version: "2"
linters:
  default: standard
  enable:
    - errcheck
    - gosec
    - bodyclose
    - goconst
    - gocritic
```

`default: standard` is the baseline (govet, unused, staticcheck, ineffassign,
etc.); the explicit list adds error-checking, security, and a few
diagnostic-only extras. Avoid `default: all` — it needs constant
exclusion-tuning to stay usable.

## `.testcoverage.yml` (go-test-coverage config)

```yaml
threshold:
  total: 75
exclude:
  paths:
    - internal/main.go
```

Adjust the exclude list as generated/wiring-only files appear.

## CI (GitHub Actions)

Single job, single step, calling the same script as local dev:

```yaml
name: ci
on: [push, pull_request]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - run: ./scripts/check.sh
```

`setup-go`'s built-in cache (`cache: true`) covers module + build cache — no
extra caching config needed.

## Explicitly out of scope

- **Git hooks** — skipped by request. Revisit only if `check.sh` drift
  (people pushing without running it) becomes an actual recurring problem.
- **Codecov/external coverage services** — go-test-coverage's GitHub Action
  reads the local `coverage.out` and needs no external account/token; add an
  external service only if trend dashboards across time become a real need.
- **Complexity linters** (`cyclop` etc.) — add only when a specific PR shows
  a concrete need; tuning thresholds blind is wasted effort.
