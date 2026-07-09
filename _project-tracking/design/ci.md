# CI orchestration

**Status: IMPLEMENTED (2026-07-08).** `.github/workflows/ci.yml` runs
`make check-backend`. Recursive Makefiles at repo root / `internal/` /
`frontend/` ensure local `make check` runs the identical checks.
See [repo-hygiene-backend.md](repo-hygiene-backend.md) for the shift from
scripts to Makefiles.

> **Note:** The workflow YAML below is the original spec. The as-built version
> uses `make check-backend` instead of individual script steps — see
> `.github/workflows/ci.yml` for the current implementation.

## Why path-filter, and which kind

A pure-backend change shouldn't pay for a frontend build, and vice versa —
precedented, common pattern, not unusual. Two ways to do it:

- **Native `on.push.paths`/`on.pull_request.paths`** — the workflow doesn't
  start at all if nothing in the listed paths changed. Simple, but has a
  real gotcha once branch protection is enabled (see
  [manual-setup.md](manual-setup.md), currently deferred): a required status
  check that never runs because a PR didn't touch its paths can block merge
  instead of being treated as "not applicable."
- **[`dorny/paths-filter`](https://github.com/dorny/paths-filter) inside one
  workflow** — the workflow always runs and always reports exactly one PR
  status check; a first job computes `backend-changed`/`frontend-changed`
  booleans, later jobs are gated with `if:` on those. No "check never ran"
  gotcha, since there's always exactly one check, it just skips work.

Going with the second specifically **because** branch protection is a
planned future step, not because it's needed today — better to build the
habit now than hit the surprise later when protection actually turns on.

## `.github/workflows/ci.yml`

```yaml
name: ci
on: [push, pull_request]

jobs:
  changes:
    runs-on: ubuntu-latest
    outputs:
      backend: ${{ steps.filter.outputs.backend }}
      frontend: ${{ steps.filter.outputs.frontend }}
    steps:
      - uses: actions/checkout@v4
      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            backend:
              - '**/*.go'
              - 'go.mod'
              - 'go.sum'
            frontend:
              - 'frontend/**'

  backend-check:
    needs: changes
    if: needs.changes.outputs.backend == 'true'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - run: ./scripts/mod-tidy-check.sh
      - run: ./scripts/fmt-check.sh
      - run: ./scripts/vet.sh
      - run: ./scripts/build.sh
      - run: ./scripts/lint.sh
      - run: ./scripts/vulncheck.sh
      - run: ./scripts/test.sh
      - run: ./scripts/cover-check.sh

  frontend-check:
    needs: changes
    if: needs.changes.outputs.frontend == 'true'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: cd frontend && bun install
      - run: cd frontend && bun run format:check
      - run: cd frontend && bun run check
```

Note this **inlines** `scripts/check.sh`'s steps rather than calling the
script wholesale, specifically so `backend-check` doesn't also try to `cd
frontend` (which `check.sh` does at its tail for the *local dev* one-command
case). Two entry points, same underlying scripts, slightly different
composition — not a duplication problem since the actual check logic still
lives in one place per step.

`go.mod`/`go.sum` changes always trigger `backend-check` even with zero
`.go` file edits — a dependency version bump can break the build without
touching a single line of Go source.

## What's deliberately excluded right now

Solo, heavy-development phase, not expecting or wanting outside
contributions yet — so this workflow is scoped to **fast feedback for one
person**, not contribution-gating:

- No required-review / branch-protection enforcement — tracked as a future
  step in [manual-setup.md](manual-setup.md), not wired to this workflow.
- No CODEOWNERS, no PR templates — nothing here assumes another contributor
  exists yet.
- Triggers on every push to every branch (not just `dev`/`main`) — the
  point right now is fast per-push signal, not gating merges.

## Separate, already covered elsewhere

- **CodeQL** (`.github/workflows/codeql.yml`) — its own workflow, own
  triggers (push/PR/weekly cron), not path-filtered — security scanning is
  cheap enough and valuable enough to just always run.
- **Dependabot + auto-merge** (`.github/dependabot.yml`,
  `.github/workflows/dependabot-automerge.yml`) — unrelated trigger (PR
  opened by Dependabot specifically), not part of this path-filter logic.
- **Release** ([release.md](release.md)) — tag-push triggered, entirely
  separate from this push/PR-triggered `ci.yml`.
