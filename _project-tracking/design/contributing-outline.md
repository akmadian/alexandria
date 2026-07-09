# CONTRIBUTING.md outline

Not the guide itself — the section list for whoever writes it. Target: repo
root `CONTRIBUTING.md`, one file, links out to detail docs rather than
duplicating them.

1. **Prerequisites** — Go version (from `go.mod`), Node/Bun version (from
   `frontend/package.json`), how to get both installed.

2. **First-time setup** — clone, `go mod download`, `cd frontend && bun
   install` (or whatever the frontend uses). One command block, copy-pasteable.

3. **Day-to-day commands**
   - `make check` (repo root) — runs backend + frontend checks, same as CI.
   - `make check-backend` / `make check-frontend` — run one side only.
   - Subdirectory Makefiles: `make -C internal lint`, `make -C frontend test`.
   - Link to [repo-hygiene-backend.md](repo-hygiene-backend.md) for details.

4. **Before opening a PR** — "run `make check` and make sure it passes
   (it covers both backend and frontend — see [ci.md](ci.md))." This is the
   one paragraph that actually prevents CI churn — say it plainly, once.

5. **Code style / architecture pointers** — don't restate the rules, link to
   `docs/coding-guidelines.md` and `CLAUDE.md`.

6. **Commit / PR conventions** — commit message format if one exists (none
   currently — note "no enforced convention yet" rather than inventing one),
   branch naming if any, how PRs get reviewed.

7. **Where things live** — one-paragraph map: `internal/` (Go backend,
   package-per-concern per coding-guidelines), `frontend/` (React/Vite),
   `docs/` (design docs + ops notes), `testdata/` (fixtures).

   **Where tracking lives specifically** (worth being explicit about, since
   it's split by *kind* of content, not accidentally scattered — see
   `functional-requirements.md`'s intro for the same note):
   - **Feature backlog / roadmap** → `functional-requirements.md` (P0–P4
     prioritized, single source of truth as of 2026-07-07 — `todo.md` is a
     deprecated historical breadcrumb, do not add to it)
   - **Architectural decisions** → `_project-tracking/backend/02-decision-log.md`
     (ADR-lite — decision + rationale + revisit trigger)
   - **Implementation-phase deferrals** → `_project-tracking/backend/impl/DEFERRED.md`
     (things deliberately deferred *during* building, each with a stated
     trigger for revisiting — different from a "someday" feature idea)

   These are kept separate deliberately (standard convention: ADRs and
   backlogs are different artifacts with different lifecycles) — don't
   merge new entries across them just because they're all "future work."

8. **Reporting issues** — where bugs/ideas go (GitHub Issues, presumably) —
   only include if there's an actual process; skip if it's just "open an
   issue."

Explicitly skip: a full style guide (already exists elsewhere, just link),
a license/CLA section (check `LICENSE` first, only add if it requires
contributor action), a code-of-conduct section (add only if the project
wants one — don't invent boilerplate).
