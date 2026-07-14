---
name: pre-commit-review
description: The Alexandria pre-commit review ritual — the closing counterpart to task-pickup. Use when implementation work is believed complete and before presenting it for commit approval — "review the work", "pre-commit review", "is this ready to commit". Classifies the diff by area (backend / seam / frontend / docs), dispatches a fresh-context reviewer against the repo's rulebook, and drives findings to resolution.
---

# Pre-commit review

You believe the work is done. Before presenting it to Ari for commit approval, it gets a review
from fresh eyes. The reviewer is a subagent with **no access to this session** — it judges the
work product against the repo's rulebook, not against your intentions. That isolation is the
point: the session that wrote the code reviews it leniently.

## Preconditions — do these before dispatching, they are cheaper than a review

1. **`make check` passes** (or the relevant subset: `check-backend` / `check-frontend` /
   `check-app`). If it fails, fix that first — the review is for what the machines can't catch,
   not a substitute for them. The coverage gate is an aggregate floor, not test adequacy: if
   you wrote code, look at per-function coverage for it now
   (`go tool cover -func` filtered to your files) — a 0% function you can see yourself is
   cheaper to fix before the review than after it.
2. **The fold-and-delete is done (D27)**: if this round completes a work item, the diff deletes
   it from `_project-tracking/` and folds its durable residue (reference docs / package READMEs
   updated; a decision entry if the round decided anything). An epic design round mints ALL its
   child tasks in the same change. `make check-docs` is green. The reviewer checks this;
   showing up without it is a guaranteed finding.
3. Nothing is committed. (Standing rule — review precedes commit approval, always.)

## Procedure

1. **Establish the diff.** `git status` + `git diff` (+ `git diff --stat`, and list untracked
   files — new files are part of the work). If the round spans commits on a branch, diff from
   the branch base instead. Note the exact scope you are submitting for review.
2. **Classify the touched areas** — pick every one that applies:
   - **backend** — `internal/`, `cmd/`, `main.go`/`app.go`, migrations
   - **seam** — `internal/seam/`, `internal/ast` vocabulary, generated TS, event/job envelopes
   - **frontend** — `frontend/src/`
   - **docs** — `_project-tracking/`, `docs/`, READMEs, CLAUDE.md/AGENTS.md
3. **Right-size the review — token economy is a design constraint.** Every subagent pays a
   fixed cost re-reading the rulebook, so match the machinery to the diff:
   - **Low-blast-radius diff** (under ~150 changed lines of Go AND adding no catalog-write /
     identity-matrix / `internal/ast` / seam-contract logic — renames, doc rounds, config rows,
     additive instrumentation): skip the dispatch. Walk the relevant
     [checklists.md](checklists.md) sections yourself and include the same per-item coverage
     lines in your report (item — clean/finding/n-a, with what you inspected) — an inline
     review still shows its work. Blast radius, not line count, is the test: if the walk
     reveals sacred-path logic, dispatch regardless of size.
   - **Normal round** (the default): dispatch ONE reviewer subagent (general-purpose). Build
     its prompt from [reviewer.md](reviewer.md): fill in the work summary, the spec path, the
     binding C/D numbers from your task-pickup report, the touched areas, and the diff scope.
     Tell it which sections of [checklists.md](checklists.md) apply.
   - **Unusually large round** (several thousand diff lines across areas): one reviewer per
     area in parallel — same template, one area each. Never default to parallel.

   **Model:** default to `model: "sonnet"` — checklist-driven review doesn't need the frontier
   model, and findings get verified by the dispatcher anyway. Inherit this session's model
   (omit the param) ONLY when the diff itself touches sacred code: writer classes, the identity
   matrix, `internal/ast`, or the seam contract. Docs riding along never upgrades the model —
   D27 puts docs in every proper round, so "multi-area" is not a signal (that trigger once made
   the expensive model the de-facto default; it is deliberately gone).

   **Dispatch in the background** and continue close-out work that cannot contaminate the
   review while it runs — memory updates, drafting the presentation, sibling-repo docs; never
   edits to files under review. Collect the report before presenting.
4. **Receive findings with rigor — no performative agreement.** First check the review is
   valid: a READY verdict without a completed Test evidence section (per-function coverage,
   what the tests actually execute) is void — re-dispatch, don't accept it. For each finding,
   verify it against the actual code before acting. A finding can be wrong; the reviewer has fresh eyes,
   not authority. Then:
   - **Critical / Important** — fix now, or if you believe the finding is mistaken, write down
     why with evidence and carry the disagreement to Ari. Never silently drop one.
   - **Minor** — fix if trivial; otherwise record it (a `ponytail:` marker if it's a deliberate
     ceiling, or a note in the report).
5. **Re-run `make check`** after fixes. If the fixes were substantial (new logic, not
   mechanical), re-dispatch a review scoped to the fix delta only — **at most one re-dispatch
   per round**; if the second review still finds Critical issues, stop and bring the state to
   Ari rather than looping.
6. **Present to Ari**: the scope in one paragraph, the reviewer's verdict, a findings table
   (severity · finding · resolution: fixed / disputed-with-reasoning / recorded), and anything
   still open. Then wait for commit approval per the standing rule.

## What this skill is not

- Not a replacement for `make check` — mechanized rules (lint, depguard, coverage, TS
  freshness) stay mechanized; the checklists deliberately exclude them.
- Not a design review — if the reviewer surfaces a conflict with a spec or a C/D rule that
  needs a decision, that goes to Ari as a question, not a unilateral rework.
