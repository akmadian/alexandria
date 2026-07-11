# Manual GitHub setup

Everything here is a **repo setting**, not a committed file — GitHub has no
native "drop a YAML in `.github/` and it auto-applies as repo config"
mechanism (the closest thing is the third-party
[probot/settings](https://probot.github.io/apps/settings/) App, not used
here). These require clicking through Settings, or `gh`/Terraform run with
explicit sign-off each time — kept in one place so it's clear what's a file
change (handled elsewhere, already done) vs. what's a manual action against
the live repo (listed below, not yet done unless marked).

## Do now

**Secret scanning + push protection**
Settings → Code security → enable "Secret scanning" and "Push protection".
Blocks a commit containing a recognizable credential *before* it lands.
Free. You don't expect secrets in this codebase, but push protection is a
zero-cost backstop, not a bet on discipline.

**Dependabot alerts** (distinct from the version-update PRs already
configured in `dependabot.yml`) — Settings → Code security → confirm
"Dependabot alerts" is on. On by default for public repos, worth confirming
explicitly. This is what actually emails/notifies on a known CVE hitting a
dependency, independent of whether an auto-PR opens.

**Allow auto-merge** — Settings → General → check "Allow auto-merge".
Required for `.github/workflows/dependabot-automerge.yml` to function at
all; without it, `gh pr merge --auto` has nothing to enable.

## Deferred — revisit when opening to outside contributions

**Branch protection on `main`/`dev`**
Require the CI status check (`scripts/check.sh`'s job) to pass, require
signed commits, disallow force-push. Not set up yet by request — this repo
is still active solo development and the friction isn't worth it yet. This
is also what makes Dependabot auto-merge actually *wait* for CI rather than
merging the instant the PR opens — without a required status check,
`gh pr merge --auto` has nothing to block on, so revisit both together.

**License-compliance scanning as a required PR check**
No harm running a scanner (`go-licenses` for Go, `license-checker` for
npm/Bun) informationally at any time, but the actual *gate* — failing a PR
that introduces a new dependency with an incompatible license — is
explicitly meant to pair with human scrutiny once external contributors can
open PRs, not before. Wire this in alongside branch protection, not
separately.

## Already automatic, no manual step

- **CodeQL** (`.github/workflows/codeql.yml`) — runs on push/PR/weekly cron,
  no toggle needed beyond GitHub Actions being enabled (default on).
- **Dependabot version updates + grouping** (`.github/dependabot.yml`) —
  already grouped by minor/patch per ecosystem so it won't spam individual
  PRs; majors still arrive individually, deliberately, for scrutiny.
