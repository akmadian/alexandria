# Culling and Signals

**Status:** design locked 2026-07-07 (C11). Cull UX is frontend; the signal architecture spans
the seam (ENRICH stage + enrichment jobs are backend follow-ups, flagged below).

## Cull view mode

The ingest-day weapon; benchmark is Photo Mechanic speed wearing a nicer suit. Where Alexandria
beats LrC on *feel* rather than features: zero lag, zero ceremony.

- Fullscreen, lights-out chrome by default; filmstrip; the same working set/arrangement/cursor as
  every view mode (C2) — cull is task-shaped as an *activity* but lens-shaped in its *state*: it
  inherits the current query rather than being a separate place. (Locked 2026-07-07; revisit to
  task view only if practice demands.)
- **Auto-advance** on P/X/rating (toggleable, per requirements).
- **Key-feedback overlay**: big transient confirmation ("★3", "REJECT") — one of the few
  sanctioned fun/color/motion moments (`02`).
- Mixed-type sessions: cull respects the current arrangement; users wanting type-batched order
  are one group-by away. No pre-cull configurator until practice demands one (any future
  "preflight" is just mutations to the already-defined query/arrangement).
- Per-type engagement via media verbs (`05`): Space zooms a photo, plays a clip.

## Signals: models propose as data, the user disposes via the query

Why current AI culling sucks: it makes *judgments* ("we picked your best 200"). Judgment is the
photographer's job and the trust-killer to automate. Alexandria's version (C11):

**Every model/algorithm output is a metadata column** — computed on-device, stored per asset,
exposed as a token type (`04`). No verdicts, no opaque scores driving hidden behavior.

| Signal | Cost | How |
|---|---|---|
| Sharpness | cheap | Laplacian variance on the thumbnail — pure signal processing, microseconds |
| Highlight/shadow clipping % | cheap | histogram on thumbnail |
| phash (near-dup cluster) | cheap | already planned (P3) |
| Blink / eyes-closed probability | heavy | small on-device model |
| Face count / face quality | heavy | on-device |
| Embeddings (semantic) | heavy | MobileCLIP2 per `docs/ops/local-ai.md` (P4) |

Marketing framing (positioning-aligned): *the AI does the measuring; you do the judging.*
Defensible because competitors can't copy it without rebuilding around inspectable signals.

## Two-tier compute architecture

**The ingest pipeline stays boring; goroutines + channels remain enough.** No workflow engines —
those solve distributed durable execution; this is one process on one machine.

1. **Cheap signals ride ingest: a new ENRICH stage** (backend follow-up to impl/04): SCAN → HASH
   → MATCH → EXTRACT → THUMB → **ENRICH** → WRITE. Operates on the in-memory thumbnail
   (sharpness, clipping, phash), small fan-out pool, near-zero marginal cost. Consequence:
   since culling starts after import completes, **the signals that make culling fast are simply
   there when the user sits down.** Day-one cull works with burst collapse and thresholds.
2. **Heavy signals are background enrichment jobs**, never pipeline stages: priority-queued,
   reporting through the one Jobs envelope (C9). Buys: fast imports; backfill over the existing
   catalog (ship a new signal → it computes for 200k old assets overnight); model upgrades =
   re-run the job. Priority: **work-follows-attention** — opening a working set bumps its jobs;
   most-recent import first by default; within a set, order by cheap signals (e.g. sharpness
   descending, so the likely keepers get heavy scores first).

**The UI never pretends** (locked): filtering on a still-computing signal annotates the pill —
"sharpness > 0.5 · **214 not yet scored**" — with results streaming in as jobs land. Users
understand waiting; they don't forgive silent wrongness.

## Force multipliers built on signals

- **Burst/stack collapse** — the single biggest time recovery ("thousands of assets, tiny-
  semantics comparison" is mostly burst pain): phash clusters render as stacks, pre-sorted within
  by sharpness + eyes-open, best frame as cover. Cull representatives; expand only when the top
  pick is contested. Rides the existing asset-group machinery (P2).
- **Suggested rejects — never auto-rejects**: below-threshold frames get a *suggested* state —
  dimmed in the filmstrip, collected in a system smart collection ("Suggested rejects · 214") —
  confirmed in bulk (keep / reject-flag / delete-from-disk with the usual warnings), reviewable
  before, after, or during the main pass. The model drafts; the user signs.
- **Threshold filters**: "when culling, only show sharpness > 0.5" is just a pill on the cull
  session's query. Saveable.
- **Auto-grouping opt-out**: ask once, respect forever (a setting). Suggestions arriving as
  system smart collections are inherently ignorable, not dismissable-nagware.
