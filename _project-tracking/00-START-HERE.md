# Alexandria — project tracking

This directory holds **work items only** — what we intend to do, never what is true or what was
done. Current truth lives in `docs/` and package READMEs; *why* lives in `docs/decisions.md`
(append-only); the feature roadmap is [`functional-requirements.md`](functional-requirements.md);
deliberate deferrals with triggers are [`DEFERRED.md`](DEFERRED.md). Full rationale for this
system: D27.

## How it works

**State = directory.** A work item moves `ideation/ → epics/ → tasks/` by `git mv`, and is
**deleted** when done — after folding its durable residue into the reference docs and the
decision log, in the same closing commit. Git history is the archive
(`git log --diff-filter=D -- _project-tracking/` lists every completed item). There are no
status fields, checkmarks, or done-ledgers anywhere, ever — state is derived from location and
existence, and `make check-docs` (pre-commit hook + CI) enforces it.

- **`ideation/`** — the inbox. Half-thoughts welcome; `inbox.md` is the loose-note pile.
- **`epics/`** — work too big for one round. One file per epic, `<area>-<slug>.md`. An epic's
  design round closes by minting ALL its child tasks at once, wiring their `Blocked by:` lines,
  folding decisions/contracts out, and deleting the epic file. Siblings share a filename stem —
  `ls tasks/ | grep <stem>` is the epic's live tracking view.
- **`tasks/`** — agent-sized, implementation-ready items, `NN-<area>-<slug>.md`. One task = one
  round = one context window, PR-shaped. Specs state the boundary (acceptance criteria + C/D
  citations), never the interior.

**The queue is `ls tasks/` in NN order; next up is the first item whose `Blocked by:` files no
longer exist** (a deleted blocker is a finished blocker). Area is an attribute in the filename
(`backend-` / `frontend-` / `seam-` / `ops-` / `perf-`), not a directory.

## Picking up work

Use the **task-pickup** skill. Reading order: `docs/CONSTANTS.md` (the C-invariants,
non-negotiable) → this file → the work item + everything it cites → sweep `DEFERRED.md`,
`ideation/backend-open-questions.md`, and `ponytail:` markers for intersections. Close a round
with the **pre-commit-review** skill; its docs check verifies the fold-and-delete happened.
