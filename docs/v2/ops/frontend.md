# Frontend hygiene & CI

Mirrors [backend.md](backend.md)'s treatment — same "local == CI" principle,
same fail-fast ordering — but built on the tooling already in `frontend/`
(Vite, Vitest, ESLint flat config, Prettier + import-sort plugin) rather than
introducing anything new.

## Existing scripts (frontend/package.json)

Already in place, no changes needed:

- `dev` — Vite dev server.
- `build` — `tsc -b && vite build`. Typecheck gates the build; a type error
  fails before Vite even runs.
- `test` / `test:watch` — Vitest.
- `coverage` — `vitest run --coverage` (v8 provider).
- `lint` — `eslint src`.
- `typecheck` — `tsc -b --noEmit`.
- `check` — `tsc -b --noEmit && eslint src && vitest run`. Already the
  aggregate target, already fail-fast ordered (type errors are cheapest to
  surface, tests are slowest) — no rework needed here, just wire it into the
  root `scripts/check.sh`.

## Gap: no format script

`prettier` and `@trivago/prettier-plugin-sort-imports` are already
devDependencies but there's no `format`/`format:check` script — the backend's
`fmt`/`fmt-check` split doesn't exist yet on this side. Add:

```json
"format": "prettier --write .",
"format:check": "prettier --check ."
```

Same rationale as the Go split: `format` is the dev-fix command, `format:check`
is what CI runs — CI must fail loudly on unformatted code, never silently
rewrite it.

## Coverage: ratchet, not gate

Already a deliberate decision in `vitest.config.ts` (see the comment there):
record coverage now, fail only on *regression* once a baseline is stable —
not a hard percentage threshold like the backend's `cover-check`. Leave this
as-is; don't retrofit a `go-test-coverage`-style threshold gate onto a
codebase that explicitly chose otherwise. Revisit only if the frontend team
decides the ratchet isn't holding the line in practice.

## Dependency vulnerabilities

No separate `npm audit`/`bun audit` target needed — Dependabot (`.github/
dependabot.yml`, `npm` ecosystem pointed at `/frontend`) already surfaces
known CVEs as PRs. Adding a second, redundant audit step in CI buys nothing
Dependabot doesn't already cover.

## Root scripts integration

Package manager is Bun (decided — `bun.lock` is the sole lockfile,
`package-lock.json` removed). Frontend already owns its own task
definitions in `frontend/package.json`'s `scripts` block — no parallel
`scripts/*.sh` directory needed on this side. `scripts/check.sh` (backend's,
at repo root since that's where `go.mod` lives) just adds two lines at the
end to call into it — not worth a separate wrapper file for two commands:

```bash
# tail of scripts/check.sh
(cd frontend && bun run format:check && bun run check)
```

### Enforcing a minimum Bun version

Add to `frontend/package.json` to hard-fail if a dev/CI runner's Bun is too
old (pin an *exact* version, not a range — Dependabot's corepack integration
currently breaks on ranges here):

```json
"devEngines": {
  "packageManager": { "name": "bun", "version": "1.3.5", "onFail": "error" }
}
```

Dependabot does not currently bump this version automatically (open
upstream request, no committed timeline) — treat it as a manual, occasional
bump, not something to automate.

## CI

See [ci.md](ci.md) for the actual workflow — path-filtered so frontend-only
changes don't trigger a backend build and vice versa.

## Explicitly out of scope

- A separate frontend coverage *threshold* gate — contradicts the existing
  ratchet decision, see above.
- A second audit/vuln-scan tool — Dependabot already covers it.
- Splitting frontend CI into its own workflow file — one combined job keeps
  "is this PR green" a single yes/no instead of two.
