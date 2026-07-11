# Enrichment (epic) — signals, thumbnails, and the post-ingest job system

**Areas:** backend, seam, frontend. **Blocked by:** nothing.
**Date:** 2026-07-07 — engine architecture produced by the frontend/UX design round (the UX it
serves: `frontend-culling-signals.md`; the governing principle: CONSTANTS C11 — AI produces
data, never verdicts). Needs its design round, which closes by minting this epic's tasks.

> **D25 (2026-07-11) supersedes this doc's pipeline placement.** The "ENRICH stage inside the
> ingest pipeline" shape below predates D25, which removed THUMB from the pipeline entirely:
> ingest is now specced as `SCAN → HASH → MATCH → EXTRACT → WRITE`, and **thumbnails are the
> first citizen of the post-ingest enrichment system** — per-artifact idempotent jobs,
> completeness derived from the missing artifact (never recorded), a DLQ-style failure record
> for poison assets, viewport-priority queue ordering, and grid cells with honest
> enriching/ready/failed states. This epic's design round merges D25's model with the tier
> split below (the tier-1/tier-2 *cost* distinction survives; the "rides ingest" placement does
> not), decides the worker shape (D17's River trigger), the job envelope evolution, and the
> queue-depth UI. It also re-derives impl/04's "full-processing cancel invariant" test — that
> acceptance was defined pre-D25 and changes shape when thumbnails leave the pipeline.

## The shape

Every signal is a **metadata column** written by the engine and exposed as a query token type
(`docs/seam-contract.md`). No signal ever drives an automatic mutation — suggestions surface as
system smart collections / Review items (D20's grammar).

Two compute tiers, split by cost:

## Tier 1 — cheap signals (pre-D25: the ENRICH stage; post-D25: the fast enrichment jobs)

Cheap per-asset computations operate on the thumbnail (sharpness via Laplacian variance,
clipping % via histogram, phash) — near-zero marginal cost once the thumbnail exists, no new
I/O against the original. Post-D25 these run as enrichment jobs ordered right behind thumbnail
generation, so cheap signals are effectively *there* by the time the user sits down to cull;
burst collapse and threshold filters work on day-one cull with no waiting.

**No workflow engines** — goroutines + channels remain enough; those tools solve distributed
durable execution and this is one process on one machine.

**Data-flow note (2026-07-08 audit, still relevant post-D25):** signal computation wants the
decoded/resized thumbnail in memory rather than a re-read from disk — the thumbnail job and the
cheap-signal jobs should be able to hand the decoded image along (or run fused as one job) so
the original is decoded once. Design detail for this epic's round.

## Tier 2 — heavy signals are background enrichment jobs

Blink/eyes-closed, face count/quality, embeddings (MobileCLIP2 per `backend-local-ai.md`):
**jobs, never pipeline stages.**

- Reports through the one Job envelope (C9, `docs/seam-events-jobs.md`) — new `kind`, zero new UI.
- **Priority: work-follows-attention.** Opening a working set bumps its pending enrichment to the
  queue front; most-recent import first by default; *within* a set, order by cheap signals
  (sharpness descending — likely keepers get heavy scores first). Post-D25 this generalizes:
  thumbnail generation for the visible viewport outranks everything.
- What the jobs model buys: imports stay fast; **backfill** (ship a new signal → it computes over
  the existing catalog overnight); **model upgrades = re-run the job**; derived-state rule holds
  (every signal column is deletable + recomputable via a registered rebuild, same as FTS and
  thumbnails).
- The v1 Jobs map (D17) suffices until enrichment demands real queuing/persistence — that is the
  "durable background work" trigger the D17 note reserved for River; re-evaluate then, not before.

## Consumers to design against (not build yet)

- `sharpness`/`clipping` token types (filter/sort/threshold) — the query layer exists; adding a
  field is a vocabulary row + compiler entry (C7/C15).
- phash cluster → burst/stack collapse via the group machinery (grouping deep dive, open
  question #7 in `../ideation/backend-open-questions.md`).
- "Suggested rejects" system smart collection + Review category (`frontend-review.md`).
- Thumbnail-quality note: cheap signals read the thumbnail, so signal fidelity is bounded by
  thumbnail size/quality (currently 512px/q80) — acceptable for sharpness ranking *within* a
  burst; document per-signal if any future signal needs original-resolution reads (that one
  becomes a heavier job class, never an inline computation).
