---
name: round-review
description: Dispatch the standard read-only reviewer agent over a design round's built surface — bugs, structure, logging, concurrency, determinism, schema drift, test gaps. Use when asked to review a round or build, dispatch a reviewer, or check recent work for bugs.
---

# Round review

Dispatch ONE general-purpose subagent to review the current round's build.
Non-negotiables:

- **Model: pin `opus` explicitly.** Never omit the model (omission inherits
  Fable, which is a policy violation); never use a smaller model for review.
- **Read-only.** The brief must forbid creating, editing, or writing any
  file anywhere. Findings return in the agent's final chat report only.
- **Run in background** so other work can continue; relay the report when
  the completion notification arrives — never predict its contents.

## Assemble the brief

1. **Scope**: the files the round touched (from `git status`/diff and the
   session's own record). List them explicitly in the brief. Supporting
   context (CLAUDE.md, adjacent models) may be read but not critiqued.
2. **Authority**: name the round's `_design/` doc and instruct the reviewer
   to read it FIRST. The doc's RATIFIED rulings win over reviewer taste:
   deviation from a ratified ruling is a high-severity finding; the
   reviewer's own design preferences are not findings.
3. **Verification discipline**: every claim verified against the actual
   code by reading it; walk the key scenarios by hand; no reporting from
   assumption.

## Review dimensions (include all; add round-specific ones)

1. **Bugs/correctness** — logic errors, edge cases; name concrete scenarios
   from the round for the reviewer to walk by hand.
2. **Concurrency/isolation** — this repo uses default-MainActor isolation:
   check `nonisolated`/`@concurrent` placement, CPU work silently on the
   main actor, Sendable soundness across `@concurrent` boundaries, and any
   read-then-write seam's safety argument.
3. **Determinism** — dictionary iteration, sort keys, anything where equal
   inputs could yield different outputs or orderings leak into results.
4. **Structure** — against CLAUDE.md (SOLID, complexity earns its place,
   registries) and the catalog fence: writes only via named catalog
   methods, catalog files named per TABLE never per process; the layer
   split (pipeline orchestrates / engines decide / catalog stores); dead
   code and stale doc comments left by refactors.
5. **Logging** — coverage and levels: anything critical silent? anything
   too chatty for its level? does metadata carry what a human debugging a
   real run needs? Concrete add/remove/relevel recommendations.
6. **Schema/code drift** — CatalogSchema vs record CodingKeys/Columns;
   round-trips (dates, JSON); code assuming constraints the schema no
   longer enforces, or vice versa.
7. **Test coverage vs ratified rulings** — list rulings without a pinning
   test; flag tests asserting implementation detail instead of ratified
   behavior.
8. **Performance hygiene** — only real issues with a concrete scenario at
   catalog scale (~40k files); don't re-report items already flagged in
   the design doc, but verify code matches what's flagged.

## Required output format (put verbatim in the brief)

A ranked findings report: per finding — severity (BUG / DESIGN / LOGGING /
TEST-GAP / NIT), file:line, one-sentence claim, concrete failure scenario
or rationale, suggested direction (not a patch). End with a short overall
assessment AND an explicit list of things verified CORRECT, so passing
checks are visible. No padding — a clean dimension is one line.

## After the report arrives

Relay findings to Ari ranked, with your own agree/disagree judgment per
finding. Fix nothing without his explicit instruction.
