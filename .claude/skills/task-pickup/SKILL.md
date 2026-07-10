---
name: task-pickup
description: The Alexandria task-pickup ritual. Use when starting an implementation task or round in this repo — "pick up impl/NN", "start the next frontier item", "let's build X". Reads the doc tree in order, sweeps the ledgers for intersections, and confirms scope before any code is written.
---

# Task pickup

You are picking up a unit of work in Alexandria. The tracking docs are the source of truth for
*what* and *why*; this skill is only the ritual. Never restate doc content from memory — read it.

## Procedure

1. **Read, in this order** (the cold-start order from the master head):
   1. `_project-tracking/CONSTANTS.md` — the C-invariants. Non-negotiable everywhere.
   2. `_project-tracking/00-START-HERE.md` — the master head. Confirm the task you were given is
      actually on the frontier; if it isn't, say so before proceeding.
   3. The owning area tracker (`backend/`, `seam/`, or `frontend/` `00-START-HERE.md`).
   4. The task's spec (usually `backend/impl/NN-*.md`) **and every doc its references list marks
      read-FIRST**, in the order given.
   5. If there are multiple tasks you could pick up, read the other tasks' specs and their references too. You may find
      that the task you were given is blocked on another task, or that a different task is more
      urgent. Recommend which task to pick up first, and wait for confirmation before proceeding.
2. **Sweep for intersections** with the task's scope:
   - `_project-tracking/backend/impl/DEFERRED.md` — does any entry's trigger fire on this task?
   - `grep -rn "ponytail:" --include="*.go" --include="*.ts"` — markers whose named trigger or
     ceiling this task touches.
   - `_project-tracking/backend/04-open-questions.md` — open questions the task brushes against.
3. **Verify docs against reality.** Status blocks lag. Check the code (and `git log`) for
   anything the spec claims is pending/missing — it may have landed since the doc was written.
4. **Ground Approach in Idioms, Best Practices, Patterns, Etc**
   - Always consider best practices, framework and language idioms, testability, and the repo's coding guidelines.
   - Ground your approach in common programming patterns - our problems are not new, smarter people have solved them before. If you find a better pattern than the one the spec uses, propose it before writing code.
   - If you find a conflict between the spec and the guidelines, or a gap in the spec, report it before writing code.
   - If necessary, search online for best practices and idioms, and propose a change to the spec before writing code.
   - Instead of writing code that is "good enough" to get the job done, propose a change to the spec that will result in better code.
5. **Report and stop.** Before writing any code, state back:
   - The scope in your own words, including what is explicitly out of scope.
   - The constants/decisions that bind the work (by C-number / D-number).
   - Intersections found in step 2 and whether you propose folding them in or leaving them.
   - What code will be changed/ added/ removed and why.
   - Any in-round decisions the spec leaves open, with your recommendation.

   Wait for User's confirmation. Do not start implementing on your own initiative.

## Standing rules (they apply to the implementation that follows)

Full identifiers, no abbreviations beyond the repo's allowed set. Comprehensive logging
(milestones at Info, per-item at Debug). Never commit without presenting the work and getting
explicit approval. On landing, execute the spec's doc-maintenance section in the same change.
When you believe the work is complete — checks green, docs maintained — run the
`pre-commit-review` skill and drive its findings to resolution BEFORE presenting the work for
commit approval.
