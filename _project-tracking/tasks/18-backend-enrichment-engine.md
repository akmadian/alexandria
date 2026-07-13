# 18 — Enrichment engine: registry, dispatcher, budget, tracker, derived writer

**Areas:** backend. **Blocked by:** nothing.
**References:** D25, D28 (`docs/decisions.md`) — D28 is the governing record for every shape
below; C9/C10/C11 (`docs/CONSTANTS.md`); `docs/data-model.md` (observation/judgment/derived
column taxonomy); writer-class + one-cook invariants (root `CLAUDE.md`).

Build `internal/enrichment`: the convergent-lane engine. No job kinds beyond a test fake ship
in this task — the engine must be complete and dispatching before thumbnail migrates onto it
(task 19).

## Scope

- **Job-kind registry** — one file, the whole graph (D28 legibility commitment #1). Row: kind,
  lane, applicability (via `assettype` capabilities), prerequisite artifacts, pool-size
  default, timeout policy func `f(size, type) → duration` (+ optional stall-watchdog mode for
  subprocess kinds), priority class, producer ref. `MustValidate` topo-sorts at boot and as a
  table test: cycles, dangling prerequisites, kinds applicable to no type all fail (C10).
- **Dispatcher** — one goroutine. Missing-artifact scan (on catalog open + on demand) fills a
  cold backlog ordered by import recency; a hot lane holds viewport hints (replace-wholesale,
  latest wins); dispatch order hot-then-cold; prerequisite check at dispatch; in-flight jobs
  never preempted. Pause/resume, global and per-kind. No priority column in the DB, ever.
- **Global weighted CPU budget** — semaphore above the per-kind pools; heavy decodes acquire
  weight by estimated size (bounds peak memory by construction). Effort dial
  (paused/low/normal/full) as a settings value mapping to token counts. Per-device I/O tokens:
  HDD depth ~2, SSD dozens; backlog reads path-ordered.
- **Per-kind worker pools** — counts settings-owned in `machine.json`
  (`Workers.Enrichment.<kind>`), mirroring `Workers.Ingest`; next-run-applied (live resize
  stays DEFERRED §6).
- **In-flight tracker** — `map[assetID]Stage` bitmask under RWMutex; `SetRunning` /
  `ClearRunning` / `Running` / `RunningBatch` (sparse result). All bitwise operations internal;
  callers pass typed constants. Write ordering contract: DB write → clear bit → emit.
- **`derived` writer class** — third catalog writer interface, derived columns only; single
  batched enrichment-writer goroutine (one cook), lulls flush like ingest WRITE.
- **`enrichment_errors` DLQ table** — (asset_id, kind, reason_code, message, attempts,
  last_attempt_at); edit `0001_initial_schema.sql` in place (pre-release policy). Scan skips
  attempt-exhausted rows.

## Out of scope

Any real job kind (19/20), seam exposure (21), observability surface (22), River / intent
lane (P3 trigger per D28), live pool resize (DEFERRED §6).

## Acceptance

- Registry with a cyclic or dangling-prereq row fails `MustValidate` (test proves both).
- Fake-kind integration test: scan finds missing artifacts, dispatcher runs producers under
  the budget, results commit through the derived writer, artifacts stop being "missing" on
  the next scan (convergence), DLQ row + attempt increment on a failing producer, exhausted
  rows skipped.
- Hot-lane hint reorders dispatch ahead of cold backlog; replacing hints discards the old hot
  set; pause stops dispatch with in-flight jobs finishing; resume drains.
- Budget test: concurrent heavy fakes never exceed token capacity; weighted acquisition blocks
  a jumbo fake until room frees.
- Tracker: bits set during producer run, cleared only after commit; `RunningBatch` is one lock
  acquisition; process-restart semantics need no test — the map is gone, and that is correct.
- Logging per guidelines §4: scan/dispatch/pause milestones at Info, per-item at Debug.
