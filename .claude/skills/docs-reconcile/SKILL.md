---
name: docs-reconcile
description: Documentation reconciliation sweep for Alexandria. Use when asked to "reconcile the docs", "do a docs sweep", "check the docs are fresh", or after a work round lands without its doc-maintenance pass. Verifies every tracking doc against code reality and updates what drifted.
---

# Docs reconcile

The tracking docs under `_project-tracking/` carry a maintenance contract: whoever completes or
reprioritizes work updates the affected docs in the same change. This skill is the audit for when
that contract slipped. Docs hold state; you re-derive state from the code and git, then make the
docs match.

## Procedure

1. **Establish reality first, docs second.** `git log --oneline` since the master head's
   "Last updated" date; note what landed. Skim the touched packages — code is truth, docs are
   claims.
2. **Sweep, in this order:**
   - `_project-tracking/00-START-HERE.md` — frontier table, dependency tree, status-at-a-glance,
     "Last updated" date. This is the doc whose staleness costs most.
   - Area trackers (`backend/`, `seam/`, `frontend/` `00-START-HERE.md`) — status vs. reality.
   - Every `impl/NN-*.md` status block — anything marked pending that shipped, or vice versa.
   - `backend/impl/DEFERRED.md` — entries whose trigger has fired or whose premise dissolved;
     the `ponytail:` marker census in its audit note (recount with grep; note stale markers —
     completed-but-still-commented).
   - `backend/04-open-questions.md` — questions answered by landed work.
   - The reconciliation ledger in `seam/01-queries-and-commands.md` — per-row done/pending.
   - Cross-references — links between docs that moved or retired.
3. **Check `CONSTANTS.md` last, differently.** Constants are LOCKED; never edit one to match
   drifted code. If code contradicts a C-rule, that is a *finding to report*, not a doc fix.
4. **Apply and report.** Make factual updates directly (status blocks, dates, checkmarks, links,
   counts). For anything requiring judgment — reprioritizing the frontier, retiring a ledger
   entry, promoting a marker — propose it and let the user decide. Do not commit; end with a
   summary of what changed and the judgment calls awaiting an answer.

## Rules

- Update a status by *verifying the code*, never by inferring from another doc.
- Convert relative dates to absolute when touching them.
- Keep the master head's maintenance contract intact — if this sweep found drift, the interesting
  question is which round skipped its doc-maintenance step; note it.
