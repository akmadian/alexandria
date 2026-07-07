# CONTRIBUTING.md outline

Not the guide itself — the section list for whoever writes it. Target: repo
root `CONTRIBUTING.md`, one file, links out to detail docs rather than
duplicating them.

1. **Prerequisites** — Go version (from `go.mod`), Node/Bun version (from
   `frontend/package.json`), how to get both installed.

2. **First-time setup** — clone, `go mod download`, `cd frontend && bun
   install` (or whatever the frontend uses). One command block, copy-pasteable.

3. **Day-to-day commands**
   - Backend: `make check`, `make test`, `make cover-html`, `make fmt` — link
     to [backend.md](backend.md) instead of re-explaining each target.
   - Frontend: equivalent lint/test/build commands (link to a future
     `frontend.md` if one gets written, or `frontend/CLAUDE.md` if that's
     where they already live).

4. **Before opening a PR** — "run `make check` (backend) / `<frontend
   equivalent>` and make sure both pass." This is the one paragraph that
   actually prevents CI churn — say it plainly, once.

5. **Code style / architecture pointers** — don't restate the rules, link to
   `docs/coding-guidelines.md` and `CLAUDE.md`.

6. **Commit / PR conventions** — commit message format if one exists (none
   currently — note "no enforced convention yet" rather than inventing one),
   branch naming if any, how PRs get reviewed.

7. **Where things live** — one-paragraph map: `internal/` (Go backend,
   package-per-concern per coding-guidelines), `frontend/` (React/Vite),
   `docs/v2/` (design docs + ops notes), `testdata/` (fixtures).

8. **Reporting issues** — where bugs/ideas go (GitHub Issues, presumably) —
   only include if there's an actual process; skip if it's just "open an
   issue."

Explicitly skip: a full style guide (already exists elsewhere, just link),
a license/CLA section (check `LICENSE` first, only add if it requires
contributor action), a code-of-conduct section (add only if the project
wants one — don't invent boilerplate).
