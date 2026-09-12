---
name: design-round
description: Run a design round self-driven — size it, reground, candidate shapes, scenario walks, adversarial pass, scope statement, build, close. Use when opening design work on a feature or subsystem, or when asked to design, propose, or shape something before building it.
---

# Design round

The ratified round procedure (2026-09-11). CLAUDE.md holds every law —
register, approvals, the catalog fence; this file only sequences the work
so Ari rules at decision points instead of driving between them. If a line
here would change what is *allowed*, it belongs in CLAUDE.md, not here.
Grounded in ADD (drivers-first, evaluation inside the iteration), Google's
design-doc practice (alternatives-considered, proportionality), and
Ousterhout's design-it-twice.

## Standing rules (every phase)

- **Register**: tag everything RATIFIED / PROPOSED / DERIVED, per CLAUDE.md.
- **Grounding**: research is claim-triggered at any phase, never only
  upfront. Any load-bearing architectural or platform claim gets fetched
  docs or a DERIVED label. Search the phenomenon, not the conclusion.
  Present the agenda before running it: each item is pain-point evidence
  or established-pattern grounding, and names the ruling it feeds — an
  item that can't name its ruling is cut.
- **Presentation for review**: lead with the decision and its costs, then
  an enumerated list of rulings needed, each with a recommendation. Detail
  sits below the ask. No term of art that isn't load-bearing; load-bearing
  ones get a one-line gloss unless already ratified vocabulary. Ari should
  never have to ask "what does that mean" before he can rule.

## The arc

**0. Size the round.** Full arc only when the design is genuinely
uncertain, contentious, or load-bearing (new subsystem, new noun, a shape
other rounds will inherit). Otherwise the slim path: reground → shape →
scope → build. State the sizing so Ari can overrule it. Process ceremony
must earn its place like any other machinery.

**1. Reground.** Before any shape-talk: what is this thing stripped of
vocabulary, which pressures and requirements are actually in the room,
what is ratified vs inherited vs open. Inherited vocabulary gets
re-derived or explicitly flagged for ratification — momentum is not
ratification. Output: the derivation chain the rest hangs on.

**2. Candidates.** For each load-bearing decision, sketch at least two
genuinely different shapes and compare them against the pressures — the
first idea is rarely the best, and alternatives named only to justify a
favorite are not alternatives. Every field, method, and noun in a
candidate traces to a pressure that generates it; a field with no pressure
is smuggled. Placement is part of the shape: say where each new type and
file lives and why, so it gets ruled with the design instead of
renegotiated at the gate. Code sketches go in chat.

**3. Pick and walk.** Pick with rationale, rejected candidates named with
why. Then walk the picked shape by hand before believing in it: the hot
paths, each consumer's contract (who reads what, who calls what), the
failure paths. A walk that surfaces a revision sends the round back to
phase 2 for that decision. **Loop 2–3 until a full pass yields no
revisions.**

**4. Adversarial pass.** Mandatory before calling the design viable, run
against the settled shape: inherited nouns that rode in unexamined; claims
made without evidence; assumptions quietly welded in (singletons,
single-context, "this can't change"); bias toward or away from the
ecosystem's own pattern; costs accepted without being named as costs.
Report findings even when the design survives them. For heavily
load-bearing rounds, offer Ari a fresh-context design reviewer (subagent,
model pinned per policy, chat-only findings) as the design-time bookend to
/round-review — his call, it costs tokens and reading time.

**5. Scope statement.** Unprompted, two layers: it OPENS with the round's
ratified-rulings ledger (every ruling Ari made this round, restated), then
the design scope — what, why, in, out, deliberately-unsettled markers on
everything deferred so nothing returns silently — then the concrete change
manifest: every file touched, new vs edit, one line on what changes in
each. Then stop. The build gate is Ari's explicit word, per CLAUDE.md;
arrive at it with everything he needs to rule and nothing he has to mine.

**6. Build.** Match the repo's conventions; tests pin ratified behavior,
not implementation; any deviation from the stated scope is flagged in the
report, never silent.

**7. Close.** Dispatch /round-review, with the scope statement's
rulings ledger flowing verbatim into the reviewer's authority section —
ruling → gate → review, one list, no drift. Relay findings ranked with an
agree/disagree judgment per finding and a proposed disposition. Then two
lists for Ari: proposed doc deltas (which of the round's rulings deserve
_design/ residence — written only on his word) and carried items (what was
deliberately deferred, recorded to session memory so future rounds inherit
them instead of re-litigating).
