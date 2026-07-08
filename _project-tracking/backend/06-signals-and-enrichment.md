# Signals and Enrichment (engine side)

**Date:** 2026-07-07 — engine architecture produced by the frontend/UX design round (the UX it
serves: `../frontend/05-culling-and-signals.md`; the governing principle: CONSTANTS C11 — AI
produces data, never verdicts). Design-only; build with the signals milestone (P2/P3 backlog
items: technical quality scoring, phash, grouping).

## The shape

Every signal is a **metadata column** written by the engine and exposed as a query token type
(`../seam/01-queries-and-commands.md`). No signal ever drives an automatic mutation — suggestions
surface as system smart collections / Review items (D20's grammar).

Two compute tiers, split by cost:

## Tier 1 — cheap signals ride ingest: the ENRICH stage

Extend the pipeline (follow-up to impl/04): SCAN → HASH → MATCH → EXTRACT → THUMB → **ENRICH** →
WRITE. ENRICH operates on the in-memory thumbnail already produced by THUMB (sharpness via
Laplacian variance, clipping % via histogram, phash) — small fan-out pool, near-zero marginal
cost, no new I/O. Consequence for UX: culling starts after import completes, so cheap signals are
simply *there* when the user sits down; burst collapse and threshold filters work on day-one cull
with no waiting.

Same pipeline rules as every stage: one wiring function, directional channels, per-item errors
never abort the run, cancel commits the current batch. **No workflow engines** — goroutines +
channels remain enough; those tools solve distributed durable execution and this is one process
on one machine.

**Data-flow prerequisite (2026-07-08 audit):** today `pipelineItem` drops the decoded thumbnail
after THUMB — only `thumbnailedAt` survives to WRITE (`importer/item.go`). ENRICH's "operate on
the in-memory thumbnail" therefore needs the item to carry the resized image (or encoded bytes)
across the THUMB→ENRICH hop, released after ENRICH the same way `head` is released after HASH.
One field on the item; do it when this stage lands, no pre-work.

## Tier 2 — heavy signals are background enrichment jobs

Blink/eyes-closed, face count/quality, embeddings (MobileCLIP2 per `_project-tracking/design/local-ai.md`):
**jobs, never pipeline stages.**

- Reports through the one Job envelope (C9, `../seam/02-events-jobs-and-binary.md`) — new `kind`,
  zero new UI.
- **Priority: work-follows-attention.** Opening a working set bumps its pending enrichment to the
  queue front; most-recent import first by default; *within* a set, order by cheap signals
  (sharpness descending — likely keepers get heavy scores first).
- What the jobs model buys: imports stay fast; **backfill** (ship a new signal → it computes over
  the existing catalog overnight); **model upgrades = re-run the job**; derived-state rule holds
  (every signal column is deletable + recomputable via a registered rebuild, same as FTS and
  thumbnails).
- The v1 Jobs map (D17) suffices until enrichment demands real queuing/persistence — that is the
  "durable background work" trigger the D17 note reserved for River; re-evaluate then, not before.

## Consumers to design against (not build yet)

- `sharpness`/`clipping` token types (filter/sort/threshold) — land with the query layer.
- phash cluster → burst/stack collapse via the asset-group machinery (grouping deep dive, open
  question #7).
- "Suggested rejects" system smart collection + Review category (`../frontend/06-review.md`).
- Thumbnail-quality note: ENRICH reads the thumbnail, so signal fidelity is bounded by thumbnail
  size/quality (currently 512px/q80) — acceptable for sharpness ranking *within* a burst;
  document per-signal if any future signal needs original-resolution reads (that one becomes a
  job, not an ENRICH computation).
