# impl/16 — Events & Jobs

**Status: ✅ SHIPPED (2026-07-10).** Third and final seam-round doc — the seam round is complete.

**What shipped:** `internal/seam/events.go` (pure: `Topic`/`EventType`/`JobState` consts, the payload
structs, `eventCatalog` + `ValidateEventCatalog`, `buildEnvelope`, the `Emitter` interface + `emitting`
embed + `WithEmitter` option) and `events_wails.go` (`WailsEmitter` — the sole `runtime.EventsEmit`
caller, forbidigo-enforced, context bound at `OnStartup`). `import_service.go` is the first C9 producer
(`StartImport`/`CancelJob` over `importer.Jobs`, with an injected `runImport` the host wires to a real
importer). `catalog/changed` emits added to `AssetService` (UpdateAssets/RemoveFromCatalog) and
`CollectionService` (all five mutators — Create/Update/Delete/Add/Remove), each with a no-op guard so
an empty write neither writes nor emits. The generator emits `events.ts` (the unions).

**Tests:** the `ImportService` *dispatch/mapping* logic is unit-tested with a fake `runImport` +
fake emitter (progress ordering, spinner→bar, cancel→cancelled, failure→failed, offline rejection,
nil-emitter safety); a separate **real-importer integration test** (`TestStartImport_RealImporterEndToEnd`,
§6 acceptance) drives the actual pipeline over a seeded `fstest.MapFS` and asserts progress events plus
a `jobs/done` summary whose Added count matches the DB rows. The event catalog's completeness and all
three failure branches are covered, and `buildEnvelope`/`WailsEmitter` guard logic too. `make check`
green; seam coverage ~86%. **Not covered by automation:** `app.go host.runImport`'s exact `os.DirFS` +
thumbnailer wiring (package-main composition glue, like the rest of `app.go`) — validated at `wails dev`.

**Deviations from this spec (accepted):**
- **`Emit(type, payload)`, not `Emit(topic, type, payload)`** — topic is derived from the catalog, so
  a type structurally cannot ride the wrong topic. A tightening, not a loss.
- **Emitter lives in `internal/seam`, split across two files** — `events.go` (pure, testable) +
  `events_wails.go` (the one Wails caller). The Wails runtime package is webkit-free (verified), so the
  seam's checks stay webkit-free.
- **Payload TS interfaces NOT generated** (per the round's decision) — only the unions are. Deferred to
  the wails-dev pass with a hard trigger (DEFERRED §7); Go structs mirror contract.ts so it's mechanical.
- **Frontend event-pump consumer not built** — `frontend/src/` is disposable (frontend/09); the rebuild
  owns the real pump. The Go integration test is the behavior proof; the through-webview loop is a
  wails-dev-time manual check.

Original spec below (unchanged) for the design rationale.

---

**Status (original): spec ready (2026-07-09), not started.** Third seam-round doc; runs after impl/14, in
parallel with impl/15 (coordinate on `internal/seam` file layout; the emit hook points in
impl/15's write methods land here).

**Scope:** the asynchronous channel — the C8 event envelope + topic catalogs on both sides, the
C9 job envelope over the engine's `importer.Progress`, the engine→seam event bridge, and the
lint/test mechanization that makes ad-hoc emits impossible. **Claims reconciliation-ledger row
#7.** **Blocked by:** impl/14. **Blocks:** the frontend rebuild (TanStack invalidation is
event-driven per frontend/09).
**References (read FIRST, in order):** `../02-events-jobs-and-binary.md` (THE design this
implements), `../../CONSTANTS.md` C8/C9, `internal/importer/jobs.go` + the `Progress` envelope
(the base that is "already right"), `../../backend/impl/DEFERRED.md` §2 (the watcher status
snapshot the `watcher` topic must anticipate), `../../frontend/09-ground-up-redesign-notes.md`
(§server state — the consumer).

## 1. The problem

The engine produces async facts (import progress, watcher connectivity, catalog mutations) but
nothing carries them to the UI. contract.ts sketches six ad-hoc `on*` subscriptions; C8 says one
envelope over four topics with a declared catalog per side, and C9 says one job envelope for
every kind of background work. This doc builds that channel once, so a new background-work kind
or event type is a constant + a payload type, never new plumbing.

## 2. The envelope and the catalogs

Per `../02`: `{ topic: "jobs" | "watcher" | "sync" | "catalog", type: string, payload, timestamp }`.
Wails events are named channels — emit on the *topic* name; the envelope is the payload
convention.

- **Go side:** `internal/seam` holds the topic + event-type constants and payload structs; one
  `Emit(topic, type, payload)` helper is the ONLY caller of `runtime.EventsEmit`.
- **TS side:** topic/type literal unions and payload types come from the impl/14 generator —
  the catalog physically cannot drift between sides.
- **Mechanized C8** (the repo's invariants-as-lint pattern, per `.golangci.yml`): a `forbidigo`
  rule bans `runtime.EventsEmit` outside the one emitter file; a completeness test asserts
  every declared type has a payload struct and vice versa.

**Initial type list** (the seam-round work item from `../02`; write into the catalog as built):
`catalog` — `changed` (the existing `CatalogChange` design: coarse by default, scope/ids-capable),
`historyChanged` (`HistoryState`). `jobs` — `progress`, `done`. `watcher` — `sourceStatus`
(shaped so DEFERRED §2's snapshot — mode `events|polling|offline`, last reconcile, dirty count —
extends it without a new type; start with the connectivity fields that exist). `sync` — reserve
for XMP conflict/apply notifications (impl/06 remainder); empty catalogs are fine, topics are
cheap, types are declared.
`UpdateAvailable` from contract.ts is DEFERRED with the update check itself (impl/12).

## 3. Jobs (C9, ledger #7)

`importer.Progress` is the base and stays as built (D17 map+mutex; River only if durable jobs
materialize). The seam envelope adds presentation:

```
JobProgress { jobId, kind, label, state: running|done|failed|cancelled,
              done, total, totalKnown, stage?, cancelable, message? }
```

- `label` is an i18n *key* (C14), derived from `kind` — display text stays frontend-owned.
- Completion (`jobs/done`) carries the existing `JobSummary` (added/updated/skipped/errors).
- **No private progress paths:** `startImport` reports through this; every future kind
  (reconcile, integrity, xmp_sync, thumb_rebuild, enrich) is a `kind` string, zero new UI and
  zero new seam surface.

## 4. The bridge (engine stays runtime-agnostic — D1)

The engine must not import Wails. Pattern: engine components expose the callback/channel hooks
they already have (`importer` Progress func, watcher connectivity writes); `internal/seam`
subscribes/adapts and emits. Where a hook doesn't exist yet (catalog-changed after user-action
writes), the *seam service method itself* emits after a successful write — impl/15's methods
are the mutation choke point, so this is one line per write method, not engine surgery. Note
the hook points impl/13 already flagged in `sqlite/collection_repo.go`.

Frontend side: one **event pump** in `api/` (frontend/09's module map) — subscribes on the four
topics via the Wails runtime, dispatches to TanStack invalidation and the store. Built here as
a thin proof (console/dev-corner consumer); the rebuild owns the real consumers.

## 5. Decisions to make DURING implementation (pre-scoped)

| Decision | Recommendation |
|---|---|
| Timestamp source | Emit-time, Go side, RFC3339 — consistent with `formatTime`; consumers treat it as display/debug metadata, never ordering truth. |
| `CatalogChange` granularity at launch | Coarse only (`{scope}`, no ids) — frontend/09's invalidation model tolerates it; ids arrive when a consumer measurably needs selective invalidation. |
| Event-pump delivery guarantee | None beyond Wails's (fire-and-forget). Events are hints (C8); anything correctness-critical stays request/response. Document this in the pump's header. |

## 6. Acceptance

- End-to-end: `startImport` from TS on a seeded directory → `jobs/progress` events arrive,
  spinner→bar flip observed when `totalKnown` goes true → `jobs/done` carries the summary
  matching the DB.
- Cancel: `cancelJob` mid-import → `state: "cancelled"` terminal event; batch-commit invariant
  untouched (existing importer tests still green).
- Catalog completeness test green both sides; generated TS unions match the Go catalog after
  `make generate-seam` (freshness gate covers it).
- `forbidigo` proof: a raw `runtime.EventsEmit` outside the emitter fails lint.
- A triage write through an impl/15 method emits `catalog/changed`; a no-op write does not
  emit.

## 7. Doc maintenance on landing (same change)

- `../02-events-jobs-and-binary.md`: the "exact type list is a seam-round work item" note
  replaced by a pointer to the built catalog; ledger #7 checked off in `../01`.
- Master head: seam round complete → frontier becomes frontend rebuild + impl/12 design round.
- `../../backend/impl/DEFERRED.md` §2: note the `watcher` topic shape awaiting the supervisor.
- This file: status block → shipped + deviations.
