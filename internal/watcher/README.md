# internal/watcher

Keeps a source's catalog rows fresh while the app is running. When a file under a
watched tree is created, changed, moved, or deleted, this package notices and
feeds a hint to the ingest engine — so the catalog tracks the disk without the
user re-importing. It is a **sensor, not an actor**: it never writes catalog rows
itself (with one sanctioned exception, below); it drives [`internal/importer`](../importer),
which does the writing.

If you read one thing first, read
[`impl/05-watcher-service.md`](../../docs/v2/post-ingest-design/impl/05-watcher-service.md)
— this package implements its "reconciled plan" section. `D<n>` references point
at [`02-decision-log.md`](../../docs/v2/post-ingest-design/02-decision-log.md);
`D14` is the watcher decision.

## The four ideas that explain everything else

1. **Events are hints; the filesystem is truth (D14).** An OS event only means
   "this path is worth a look." We never trust *what* it claims happened. At
   graduation we `stat` the path and let the result decide: it exists → ingest it;
   it's gone → mark it missing. This is why the code has **no branching on event
   type** — create, write, delete, and rename all arrive the same way ("this path
   is dirty"), and the single stat is the fact. Lost or reordered events cost
   freshness, never correctness.

2. **Reconcile is the answer to every failure.** There is no clever per-failure
   recovery. Event overflow, a watch-limit blowup, a kill-9 while running, a
   dropped event — all resolve the same way: run a full walk (`Ingester.Run`),
   which re-derives the whole source from disk. "Reconcile is a schedule, not a
   component" (D14): it's just the importer in full-walk mode. The watcher runs one
   at **startup** (the crash-recovery path) and again on **overflow**.

3. **Debounce collapses save storms.** Creative apps save a file as a flurry —
   temp write, rename, re-write — in a few hundred milliseconds. A dirty **set**
   (one entry per path) with a 500ms timer reset on every new event turns that
   flurry into exactly one ingest once the path goes quiet. A settle check (two
   stats 50ms apart; size+mtime must match) then confirms the writer is actually
   done before we hash.

4. **The importer holds the judgment guarantee, so the watcher inherits it.** The
   watcher only calls observation-class methods (`Run`, `IngestFile`,
   `MarkMissing`). It structurally cannot touch a rating/flag/label — the same
   writer-scoping that protects the batch pipeline (D8) protects this path.

## How a hint flows

```
notify (FSEvents / inotify / RDCW)
        │  absolute OS path
        ▼
  source_notify.go ──normalize──►  Event{Path: "sub/dir/photo.jpg"}   (root-relative, slash)
        │
        ▼
  watcher.go loop  ── one goroutine owns the dirty set ──────────────────────┐
        │  intake: ignore-list check, then arm/reset a 500ms per-path timer   │
        ▼                                                                     │
  timer fires ──► graduated channel ──► graduate():                          │
        stat(path):  exists+settled → Ingester.IngestFile   (→ online)       │
                     gone            → Ingester.MarkMissing  (→ missing)      │
                     still changing  → re-arm the timer  ◄────────────────────┘
```

Everything runs in **one loop goroutine** that owns the dirty-set map, so there
are no locks on it. Timers fire on their own goroutines but only post a path to a
channel; the loop does all the map mutation.

## Files

| File | Job |
|------|-----|
| `watcher.go` | The service. `Watcher.Run` (startup reconcile → subscribe → loop), the debounce loop, `graduate` (the stat-decides-truth logic), and `settled`. |
| `source_notify.go` | The **only** file that touches the OS backend (`rjeczalik/notify`). Subscribes recursively, normalizes each event to the local `Event`, and closes its channel when the context ends. |
| `event.go` | The `Event` type — a root-relative path plus an `Overflow` flag. Deliberately thin, because truth is re-derived downstream. |
| `watcher_test.go` | Save-storm→one-ingest, ignore-at-intake, delete→missing, overflow→reconcile, driven by a fake event source and an `Ingester` spy (no real catalog, race-clean). |

## Why `rjeczalik/notify` and not hand-rolled adapters

The design nominally wanted three per-OS event adapters (FSEvents / inotify /
ReadDirectoryChangesW). `rjeczalik/notify` *is* exactly those three behind one
type, with recursive watching — including recursive FSEvents on macOS, **not**
kqueue (kqueue needs an fd per file and can't watch a tree). Rather than write and
maintain three cgo adapters, we take the dependency and keep the seam to one file
(`source_notify.go`): everything else speaks the local `Event`, so swapping the
backend later is a one-file change. (`fsnotify` was rejected — kqueue on macOS, no
recursion, wrong for large photo trees.)

## Rules you must not break

- **Don't branch on event type in `graduate`.** The stat is the source of truth;
  trusting the event's claim is how you reintroduce flapping (rename = delete then
  create) and stale rows. Add an event kind only if you also add a real reason the
  stat can't answer the question.
- **The loop goroutine owns the dirty-set map.** Timers may only *send a path to a
  channel*. Don't touch the map from a timer callback — that's a data race.
- **Every failure falls back to reconcile.** New failure mode? The answer is
  almost always "drop the affected state and `Ingester.Run`," not a bespoke repair.
- **`MarkMissing` never removes a row.** A delete is an observation; the file may
  come back (moved back, remount). Marking missing is the most the watcher may do.
- **Keep the OS backend in `source_notify.go`.** The rest of the package must not
  import `notify`.

## Sharp edges

- **macOS symlink canonicalization.** `/var` is a symlink to `/private/var`, and
  FSEvents reports the *resolved* path. If the watched root isn't resolved too,
  every event looks like it escaped the tree (`filepath.Rel` → `"../…"`) and gets
  dropped. `Run` calls `EvalSymlinks` on the root so both live in one namespace.
  This bug is invisible to the unit tests (they use a fake source) — the live
  `cmd/dev watch` smoke is what catches it.
- **Settle sleeps in the loop.** The 50ms settle stat runs inline, so a burst of
  many *distinct* files serializes ~50ms each. Fine for interactive editing; if
  bulk-drop throughput ever matters, move settle+ingest to a worker queue (marked
  `ponytail:` in `watcher.go`).
- **Benign double-graduation.** A late event can reset a timer that already fired,
  so a path may graduate twice. Harmless — re-ingesting an unchanged file is a
  no-op reimport. Not worth a generation counter.

## Deliberately deferred (with `ponytail:` markers)

- **Rename pairing.** `notify` gives no portable from→to link, so a rename is seen
  as delete-old + create-new: the old path goes missing, the new path ingests (as
  a duplicate if the name changed). The 05.1 rename-enrichment seam
  (`importer.IngestRenamed`) is ready for a future inotify-cookie-based pairing;
  until then a reconcile heals the split.
- **Sidecar echo check.** Deferred because nothing writes `xmp_hash` until XMP sync
  (impl/06) — there's nothing to echo against yet.
- **Connectivity / the `events ⇄ polling ⇄ offline` state machine.** The
  poll-timer + EIO-probe volume monitor is impl/05.3; today the watcher assumes a
  reachable local root.
- **Disjoint-sources assumption.** Paths are relative *per source*, and the
  matrix's content match + delete-side merge are global (not source-scoped), so
  overlapping/nested source roots corrupt state. The fix is a registration-time
  resolver, not anything here — see
  [`impl/DEFERRED.md`](../../docs/v2/post-ingest-design/impl/DEFERRED.md) §1.
- **External supervision.** A `Watcher` is one unit; it does not restart itself,
  and nothing here runs one-per-source. The supervisor (restart+backoff + status
  snapshot, and deliberately no health-kill watchdog) arrives with the app host —
  see [`impl/DEFERRED.md`](../../docs/v2/post-ingest-design/impl/DEFERRED.md) §2.

## Run it by hand

```
go run ./cmd/dev watch <path> --catalog <dir>
```

Startup-reconciles the tree, then watches until Ctrl-C. Create/edit/delete files
under `<path>` and inspect `<dir>/catalog.db` (e.g. `sqlite3 … "SELECT
relative_path, file_status FROM assets"`) to see rows track the disk. Note that
`IngestFile` is a batch-of-one and opens no `import_sessions` row, so `dev
sessions` shows only the startup reconcile — look at the `assets` table for the
live hints.
