# CI orchestration

**Status: BUILT (2026-07-08, revised 2026-07-09).** `.github/workflows/ci.yml` runs
`make check-backend` — the same target a dev runs locally from the single root Makefile
(see [repo-hygiene-backend.md](repo-hygiene-backend.md) for the targets and the shift from
scripts to Make). This doc covers the trigger/orchestration layer.

## Path filtering: native `on.paths`, with a recorded caveat

A pure-backend change shouldn't pay for a backend run's cost, and vice versa. Two ways to
filter; the original spec chose [`dorny/paths-filter`](https://github.com/dorny/paths-filter),
the implementation went with **native `on.push.paths`/`on.pull_request.paths`** — simpler, no
third-party action, and correct for the current phase.

**The caveat, so it isn't rediscovered the hard way:** with native path filtering, a workflow
that doesn't trigger reports *no* status check. Once branch protection marks `backend` as a
required check, a PR that touches no backend paths has a required check that never runs — and
GitHub blocks the merge instead of treating it as "not applicable." dorny/paths-filter avoids
this by always running one workflow that internally skips work.

**Decision (Ari, 2026-07-09):** main is deliberately unprotected right now for solo velocity.
Branch protection, required checks, and everything contribution-facing arrive together in a
dedicated **contribution-readiness round** before opening the repo — revisit this filter choice
there (switch to dorny, or list the workflow as non-required). Until then, native paths stand.

## What triggers the backend job

`**.go`, `go.mod`, `go.sum`, `.golangci.yml`, `Makefile`, `testdata/**`, and the workflow file
itself. Rationale for the non-obvious ones:

- `go.mod`/`go.sum` — a dependency bump can break the build with zero `.go` edits.
- `.golangci.yml` / `Makefile` — a check-config change must re-run the checks it configures.
- `testdata/**` — a fixture change can flip test outcomes without touching source.

Push triggers are scoped to `main`/`dev`; PRs from any branch still run.

## The backend job

checkout → setup-go (`go-version-file: go.mod`) → install golangci-lint (pinned to the local
Homebrew version — see repo-hygiene-backend.md on why the pin must track local) → install
exiftool (`libimage-exiftool-perl`; without it the xmp + dependency suites self-skip and hide
regressions) → `make check-backend`.

## Frontend job: deferred

Deliberately absent (Ari, 2026-07-09) until frontend implementation progresses — the current
`frontend/src` is pre-rebuild and gating on it buys nothing. When it lands: setup-bun,
`bun install`, `make check-frontend`, triggered on `frontend/**`; the `format:check` script gap
([repo-hygiene-frontend.md](repo-hygiene-frontend.md)) should be closed in the same change.

## What's deliberately excluded right now

Solo, heavy-development phase — this workflow is scoped to **fast feedback for one person**,
not contribution-gating:

- No required-review / branch-protection enforcement — bundled into the future
  contribution-readiness round ([manual-setup.md](manual-setup.md)), not this workflow.
- No CODEOWNERS, no PR templates — nothing here assumes another contributor exists yet.

## Separate, already covered elsewhere

- **CodeQL** (`.github/workflows/codeql.yml`) — own workflow, own triggers (push/PR/weekly
  cron), not path-filtered — security scanning is cheap and valuable enough to always run.
- **Dependabot + auto-merge** (`.github/dependabot.yml`,
  `.github/workflows/dependabot-automerge.yml`) — unrelated trigger (Dependabot PRs only).
- **Release** (the ops-release epic in project tracking) — tag-push triggered, entirely separate.
