# RAW export via external tool dispatch

Not a priority right now — no code, no timeline. Written so the design
exists before it's needed. Extends decisions already made rather than
inventing new architecture; see `docs/v2/post-ingest-design/02-decision-log.md`
(D4, D5, D6) and `docs/v2/functional-requirements.md` (the "Configurable
'Open In' Programs / External Program Registry" entry this builds on).

## The core realization this design rests on

Non-destructive RAW editing (Lightroom, Capture One, Photomator, etc.) stores
edits — exposure, masks, AI denoise — as *instructions*, not baked pixels.
Pixels only get produced when a specific engine *renders* those instructions
against the original RAW sensor data. **Those instructions are only
meaningful to the engine that wrote them** — you cannot take one tool's edit
stack and render it correctly in another. XMP sidecars give partial
interoperability for basic adjustments (exposure, white balance, crop), not
for AI masking/denoise.

Conclusion: Alexandria **cannot** build its own RAW export engine that
matches real edit-tool output (that's a categorically different, enormous
problem — demosaic algorithms, color science, masking engines, AI denoise
models, the entire product other companies sell). What Alexandria *can*
reasonably do: **dispatch a batch-export command to whichever external tool
actually holds the edit state**, and degrade gracefully when that tool
offers no automation surface.

## Export as a third `TypeHandler` capability (extends D6)

D6's registry already treats capabilities as small independent fields —
`Metadata`, `Thumb`, nil = graceful degradation, never an error. `Export`
slots in the same way, but resolves differently per format family:

- **Raster/rendered formats** (JPEG, PNG, already-flattened images): `Export`
  is a direct function — Alexandria dispatches to ffmpeg/ImageMagick itself,
  same shape as the existing export-pipeline plan. Alexandria owns the
  rendering because there's no proprietary edit stack in the way.
- **RAW formats**: `Export` isn't a function Alexandria implements — it's a
  *lookup* into the external-program registry (see
  `functional-requirements.md`'s "open in" / registry entry) for whatever
  tool is associated with that RAW extension, checked against that tool's
  `has-scripting-automation` capability flag.
  - Flag present → dispatch a batch-export command via that tool's
    automation surface (AppleScript/Automator for Pixelmator Pro, confirmed
    to support batch export actions).
  - Flag absent → **graceful degradation, per D6's existing principle**: no
    silent failure, no crash — surface a clear message ("Alexandria can't
    batch-export RAW files edited in \[Program\] — export from there
    directly") and stop. The *reason* for the nil capability differs (tool
    genuinely has no automation surface, vs. "we haven't written a handler
    yet") but the UI treatment is identical: option isn't offered, reason is
    explained.

## Open state this needs that doesn't exist yet

**Which tool rendered a given asset's edits** is a distinct piece of state
from "what program do I open this file type in" (the existing "open in"
default-app mapping). A user might open RAW files in Pixelmator Pro for
casual viewing but do their actual development elsewhere — "open in" and
"owns the edit state" can differ and shouldn't be conflated into one field.
Needs its own per-asset (or per-source) association, not yet designed.

## Mixed batches

A single export selection can span multiple owning tools (30 assets
developed in Pixelmator Pro, 20 in Photoshop). Dispatch has to **group by
owning tool** and fire separate automation calls per tool, not treat the
batch as homogeneous. Worth designing for from the start even if not built
day one — retrofitting grouping logic later is more painful than including
the grouping key up front.

## Job modeling: same outer shape as the dependency fleet, different inner supervision

This is the part that could easily go wrong by over-borrowing from D5, so
it's worth being explicit about exactly what transfers and what doesn't.

**Transfers cleanly:** the *outer* job-tracking shape. A dispatch is a row in
the `Jobs` registry (D17-style: registry map + `OnProgress`), surfaces in the
status bar exactly like import does ("Exporting via Pixelmator Pro..."). No
reason for the user to learn a second mental model for "something is running
in the background" — that consistency is a real, cheap win.

**Does NOT transfer — D5's actual subprocess supervision — because the risk
model inverts for a GUI app the user is sitting in front of:**

- **Crash isolation reasoning flips.** D4's rationale for subprocesses was
  "a crash is isolated, `kill -9` always works, no real cost." That's false
  here: if the target app crashes or hangs, it's the user's actual
  application, possibly holding unrelated unsaved work. Force-quitting it on
  a timeout — correct and safe for `vips`/`ffmpeg` — is actively dangerous
  applied to a program the user owns and may be using for other things
  simultaneously.
- **D5's per-tool semaphore (cap concurrent processes) doesn't apply.**
  There's normally one running instance of the target app, not N parallel
  copies being spawned. The real constraint is closer to "one dispatch in
  flight per target app," a simpler and different throttle than a
  process-count semaphore.
- **Progress granularity is coarser by necessity.** `exiftool`/`vips` expose
  structured, parseable progress. GUI automation (AppleScript) typically
  only confirms "the command returned" — likely no "12/50 files" signal.
  Design the UI around a 3-state model (dispatched → done, or dispatched →
  unknown/still-running) rather than assuming per-file granularity will be
  available.
- **User interaction during the job is expected, not an anomaly.** Headless
  subprocesses can't be interacted with by construction; a GUI dispatch
  might have the user watching or intervening. "Cancel" can't mean `kill -9`
  the way it does for `vips` — it more likely means surfacing "cancel this
  yourself in \[Program\]," since killing the user's active application out
  from under them is hostile UX, not safe cleanup.

**Net:** reuse the Jobs registry and progress-surface UX; do not reuse D5's
kill-on-timeout/semaphore/crash-isolation machinery. Model GUI dispatch as
its own thin job type — fire a command, don't own the process, don't kill on
timeout, surface uncertainty honestly rather than forcing a resolution one
way or the other.

## Explicitly out of scope for now

- Building any of this — no priority, no timeline, doc exists to not lose
  the design.
- Resolving exactly how "which tool owns this asset's edits" gets tracked —
  flagged as needed, not designed.
- Any specific automation script/AppleScript implementation — depends on
  confirming what each target program's automation surface actually
  supports (Photomator's specifically is still unconfirmed, per the earlier
  Pixelmator-vs-Photomator discussion).
