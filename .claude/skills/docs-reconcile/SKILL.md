---
name: docs-reconcile
description: Documentation reconciliation sweep for Alexandria. Use when asked to "reconcile the docs", "do a docs sweep", "check the docs are fresh", or after a work round lands without its doc-maintenance pass. Verifies every tracking doc against code reality and updates what drifted.
---

# Docs reconcile

The docs system (D27) derives state from the tree — mechanical drift is caught by
`make check-docs` (status prose, work-item authority pointers, dead links, filename contracts).
This skill is the JUDGMENT half: the checks a grep can't run. Code is truth, docs are claims;
re-derive, then make the docs match.

## Procedure

1. **Run `make check-docs` first.** Fix anything it reports before judging — never hand-audit
   what the machine already checks.
2. **Establish reality.** `git log --oneline` since the last reconcile/round; skim the touched
   packages.
3. **Judgment sweep, in this order:**
   - **Fold completeness.** For each work item deleted since the last sweep
     (`git log --diff-filter=D -- _project-tracking/`): did its round actually fold the residue —
     reference docs updated, decision entry appended where a decision was made? A deletion
     without its fold is the worst drift this system allows.
   - **Reference docs vs code reality.** `docs/*.md` + package READMEs — anything they state that
     the code no longer does. These are the docs whose staleness costs most.
   - `_project-tracking/DEFERRED.md` — entries whose trigger has fired or whose premise
     dissolved; recount the `ponytail:` markers with grep (note completed-but-still-commented).
   - `_project-tracking/ideation/backend-open-questions.md` — questions answered by landed work
     (delete them; the answer lives in the decision log).
   - `tasks/` — items whose scope has partially landed (rescope them), or whose `Blocked by:`
     names a file that never existed (typo — fix it).
   - `epics/` — epics whose design round quietly completed without minting tasks.
4. **Check `docs/CONSTANTS.md` last, differently.** Constants are LOCKED; never edit one to
   match drifted code. If code contradicts a C-rule, that is a *finding to report*, not a doc fix.
5. **Apply and report.** Make factual updates directly (reference-doc corrections, dead-entry
   deletions, link fixes). For anything requiring judgment — retiring a ledger entry, rescoping
   a task, promoting a marker — propose it and let the user decide. Do not commit; end with a
   summary of what changed and the judgment calls awaiting an answer.

## Rules

- Update a claim by *verifying the code*, never by inferring from another doc.
- Convert relative dates to absolute when touching them.
- Never ADD status prose while reconciling — done work is deleted and folded, not annotated.
- If this sweep found drift, the interesting question is which round skipped its fold-and-delete
  step; note it.
