# Events, Jobs, and the Binary Channel

**Status:** design locked 2026-07-07 (C8/C9); grounded against `internal/importer/jobs.go` and
contract.ts during the docs reconciliation pass.

## Events (C8)

One envelope shape over a small set of named topics; every event type declared in one constants
catalog per side. Wails events are named channels — "one envelope" is a *convention*, not a
single pipe.

```ts
{ topic: "jobs" | "watcher" | "sync" | "catalog", type: string, payload: T, timestamp: string }
```

- `catalog` carries the existing `CatalogChange` design (coarse by default, scope/ids-capable for
  selective invalidation — consumers may ignore the payload and invalidate the active view).
- `sources`/history/update events from contract.ts fold into topics (`watcher` carries source
  connectivity; `catalog` carries history state). **Built (impl/16, 2026-07-10):** the type
  catalog is `internal/seam/events.go` (`eventCatalog`) — `catalog`→`changed`/`historyChanged`,
  `jobs`→`progress`/`done`, `watcher`→`sourceStatus`, `sync` reserved. `historyChanged` and
  `sourceStatus` are declared with no producer yet (undo service / watcher supervisor land them —
  DEFERRED §7/§2). `UpdateAvailable` deferred with the update check (impl/12).
- **The one emit choke point:** `internal/seam/events_wails.go` is the sole `runtime.EventsEmit`
  caller (forbidigo-enforced); services hold a `seam.Emitter` and never touch Wails. `Emit` derives
  the topic from the catalog (a deliberate tightening of the design's `Emit(topic, type, payload)`
  — a type can't ride the wrong topic) and validates the payload's Go type against the catalog
  exemplar, so a malformed event can't cross.
- Events are hints for *display and invalidation*; request/response stays synchronous typed
  calls. No EDA (C8).

## Jobs (C9)

**The engine's `importer.Progress` is the base and it's already right** — including `TotalKnown`
(total is indeterminate until the scan walk finishes; flipping it true upgrades the UI from
spinner to bar without a counting pre-pass). The engine-side `Jobs` map (jobID → cancel) stays as
built (D17); River only if durable jobs materialize.

Seam envelope = engine Progress + presentation fields:

```ts
interface JobProgress {
  jobId: string;
  kind: string;              // "import" | "reconcile" | "integrity" | "xmp_sync" | "thumb_rebuild" | "enrich" | …
  label: string;             // human-readable, i18n-keyed on the frontend
  state: "running" | "done" | "failed" | "cancelled";
  done: number;
  total: number;
  totalKnown: boolean;
  stage?: string;            // pipeline stage for the activity drawer
  cancelable: boolean;
  message?: string;          // optional detail for logs + the dev corner
}
```

Completion carries the existing `JobSummary` (added/updated/skipped/errors). **No private
progress paths** (C9): import, enrichment, backup, export, integrity, RAW dispatch all report
through this envelope; status bar and activity drawer render it generically — a new kind of
background work is a new `kind` string, zero new UI.

## The binary channel

Standing convention (from contract.ts, adopted): **bytes never cross the seam.** Thumbnails,
previews, and originals travel over the asset URL scheme; the seam carries URL builders only.
Grid tiles carry their own `thumbURL` on the row. URLs are content-addressed/cache-busted
(reconciliation ledger #10) so in-place thumbnail regeneration invalidates correctly.

## The error shape

Standing convention (from contract.ts, adopted): every failure normalizes to
`ApiError { kind: transport|degraded|domain|unexpected, code?, detail? }`. **Codes cross the
seam, not strings** — display text is frontend-owned (C14). `degraded` is the nil-capability /
missing-dependency lane (C10's one-fallback rule surfaces here: "exiftool missing" is a degraded
code, never a crash).
