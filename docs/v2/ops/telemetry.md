# Telemetry — opt-in

Spec for implementation once the Wails shell exists; nothing to build yet.
Non-negotiable constraint from the start: **opt-in, not opt-out** — no
event leaves the machine until a user explicitly says yes.

## Consent

- First-run prompt, plain language, defaults to **off** if dismissed/ignored.
- Store the choice locally (same settings store as everything else in
  `domain.Settings` — no separate consent subsystem needed).
- A visible, easy-to-find toggle to revoke consent later, not buried three
  menus deep.

## What to collect

Two different things, worth keeping separate since they have different
sensitivity:

- **Crash/error reports** — stack trace, OS/arch, app version. Useful even
  from a single opted-in user; highest signal-to-noise.
  - **Usage events** — feature-level counts (e.g. "import run", "duplicate
  scan triggered"), no file paths, filenames, or asset content ever
  included. If a metric would require touching user file paths/content to
  compute, don't collect it.

## What never gets collected

- File paths, filenames, EXIF/XMP contents, thumbnails — anything that
  touches the user's actual photo library.
- Any identifier tied to a real person — a random per-install UUID
  (generated once, stored locally) is the only "identity," and it should be
  regenerable/deletable by the user (ties back to the revoke-consent path).

## Where it goes

Options, roughly in order of effort:

- **Self-hosted endpoint** — a small HTTP endpoint that accepts the two
  event shapes above and writes to... anything (SQLite, a log). Full
  control, no third-party data processor, but you own uptime.
- **PostHog** (self-hosted or cloud) — purpose-built for exactly this
  opt-in-analytics-plus-crash-reporting shape, has a generous free tier.
  Probably the pragmatic default — building your own ingestion pipeline for
  this is exactly the kind of thing not worth owning.
- **Sentry** — better fit if crash reporting is the priority over usage
  analytics specifically; less natural for feature-usage counts.

Lean toward PostHog unless a reason surfaces not to — it covers both
categories above without standing up custom infrastructure.

## Explicitly out of scope for now

- Any collection design detail beyond "opt-in, no file content/paths, revoke
  is easy" — the concrete event schema is a decision for when the Wails
  shell and its actual features exist, not before.
