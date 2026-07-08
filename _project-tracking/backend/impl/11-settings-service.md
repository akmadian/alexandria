# impl/11 — Settings Service (User Settings + Keybindings + Machine Config)

**Status: settings service DONE (§1–4, §6, §7). Live pipeline resize (§5) DEFERRED —
see note below.** Shipped `internal/settings` (generic `configFile[T]`, tolerant
cold-load with quarantine, live reload keeping last-known-good, per-field clamps,
atomic save, debounced parent-dir hot-reload watch) with tests covering every
Acceptance bullet except the two resize tests. Dropped the `settings` table from
`0001_initial_schema.sql`. `internal/settings` is a strict leaf (imports no other internal
package); importer and watcher now import *it* for the D18 ignore list (§6, §8) — a deliberate,
acyclic dependency, not the original "consumers never import settings" rule.
>
> **Two deliberate deviations from §1/§3 as written, per Ari (2026-07-07):** (1)
> files are **created with defaults on first run** — same strategy as the catalog
> DB (`sqlite.Open`) — not left absent until the first `Save`. The malformed-file
> path still does NOT auto-rewrite (quarantine + inspect intent preserved); only the
> *missing* case materializes defaults. (2) The catalog-scoped `settings.json` lives
> in a **`settings/` subdir** of the catalog dir (`<catalog-dir>/settings/settings.json`),
> not at `<catalog-dir>/settings.json`. The dev harness wires `settings.Open` there
> alongside the catalog and colocates `machine.json`/`keybindings.json` in the same
> dir provisionally (they move to `<app-config-dir>` when the app host lands).
>
> **YAGNI checkpoint + wiring (Ari, 2026-07-07).** Only fields with a live consumer
> survive; the §2 shapes below are aspirational, the shipped structs are trimmed.
> **Wired:** `Machine.Workers.Ingest.{Hash,Extract,Thumb}` → `resolvePools`,
> `Settings.ImportBatchSize` → the WRITE batch size, `Settings.ThumbnailQuality` →
> the thumbnailer (importer holds `settings.Settings` + `settings.Machine`; harness
> injects from `Get()`). **Dropped** (no consumer — re-add with the feature that
> needs them): `xmpConflictResolution`/`xmpWriteBack` (impl/06), `catalogBackupCount`,
> `undoStackSize`, `updateCheckEnabled`, `defaultSortField`/`Dir` (no query layer yet),
> `Machine.memoryLimitMB`, `telemetryConsent`. Also deleted a dead `domain.Settings`
> stub. Live mid-run worker resize (§5) stays deferred → DEFERRED §6.

> **§5 live resize deferred, on purpose.** It needs invasive surgery on the working
> pipeline (`fanStage` spins fresh goroutines per run; a resizable `stagePool` replaces
> that) AND the spec itself flags an unresolved run-teardown race as a correctness
> requirement. It also can't be wired yet: the `svc.Machine.OnChange → run.Resize` hook
> lives in the composition root, and `<app-config-dir>`/the app host don't exist yet.
> Building the `stagePool` primitive now would be a resize engine with no live caller
> (YAGNI). Do it alongside the app-host milestone, when there's something to wire it to.
**Scope:** new `internal/settings`; touches `internal/migrations` (DROPS the `settings` table —
edited in place, per this repo's pre-1.0 convention), `internal/importer` (live pool resize),
`cmd/dev`/future app host (composition root wiring). **Blocked by:** nothing. **Blocks:** impl/06
(`xmpWriteBack`/`xmpConflictResolution`), the keybinding-editor UI, the machine-config UI.
**References:** D16, D18, D19 (generics for varying data), `03-data-model.md` §3.

> **Supersedes D16's storage mechanism, keeps its scoping.** D16 got the *routing* right
> (localStorage / catalog-scope / machine-scope) but picked a catalog-DB KV table for the
> catalog-scoped tier and folded keybindings into it as one JSON blob. This revision (2026-07-07,
> two rounds of discussion) keeps the three scopes but makes all three **plain JSON files**, and
> pulls keybindings out to their own file at the *machine/user* scope, not the catalog scope — a
> keybinding preference is a fact about the person, not the catalog, and D16 had it in the wrong
> place regardless of storage format. `02-decision-log.md` D16 and `03-data-model.md` §3 need an
> addendum when this lands (see the pointer at the bottom of each).

## 1. Three files, three scopes — no DB table

- **`<catalog-dir>/settings.json`** — catalog-scoped: XMP policy, ignore list (D18), import
  defaults, per-view UI state. **`<catalog-dir>` is settled** — verified against
  `internal/sqlite/open.go`: it's exactly the `dir` argument to `sqlite.Open`, the same directory
  `catalog.db`/`catalog.lock` already live in. Cold-loaded when that catalog opens, then watched
  for external edits for as long as it stays open (§7).
- **`<app-config-dir>/machine.json`** — machine-scoped: worker-pool sizes, memory limit, dependency
  path overrides, telemetry consent. Cold-loaded once before any catalog opens (D16's ordering
  constraint, unchanged), then watched for the rest of the process lifetime (§7) — "read once" only
  describes the initial load, not the whole file's lifecycle.
- **`<app-config-dir>/keybindings.json`** — user-scoped: keybinding overrides only, defaults live in
  frontend code. Lives beside `machine.json`, not inside any catalog — switching catalogs must never
  change what `j`/`k` do.

**`<app-config-dir>` is an open question, not a decided path** — unlike `<catalog-dir>`, nothing in
the codebase currently resolves an app-wide config location (no `os.UserConfigDir()` usage exists
yet). Pick this when the app-host milestone lands (it's an OS-appropriate-directory decision, not a
settings-design one); don't invent a concrete path here just to fill the placeholder.

**Why files, not a DB table, for the catalog-scoped tier (reversing my own earlier call):** the
atomicity argument I made previously doesn't hold up against a concrete case — nothing today needs
a settings write inside the same transaction as an asset write. Meanwhile D9 already describes the
catalog directory as "DB + thumbnails + **settings**" — settings was already conceived as a
directory *sibling*, not DB-internal — and thumbnails already prove loose files are safe under the
existing per-catalog instance lock. The KV table was only ever going to hold JSON-encoded blobs per
key (the ignore list, per-pipeline worker objects); one JSON document removes that indirection
instead of adding real structure over it. Net: less code (no repo/scan boilerplate, one
`json.Unmarshal`) and it fits the directory-bundle model the catalog already uses for thumbnails.

**No ownership enum**, still — the file a value lives in (catalog dir vs. app-config dir) is the
scope tag.

## 2. Shapes

```go
package settings

// Settings — catalog-scoped, lives at <catalog-dir>/settings.json.
type Settings struct {
    XMPConflictResolution string          `json:"xmpConflictResolution"` // "xmp_wins" | "catalog_wins"
    XMPWriteBack          bool            `json:"xmpWriteBack"`
    ThumbnailQuality      int             `json:"thumbnailQuality"`      // storage/compression, distinct from worker COUNT (machine-scoped)
    ImportBatchSize       int             `json:"importBatchSize"`
    CatalogBackupCount    int             `json:"catalogBackupCount"`
    UndoStackSize         int             `json:"undoStackSize"`
    UpdateCheckEnabled    bool            `json:"updateCheckEnabled"`
    DefaultSortField      string          `json:"defaultSortField"`
    DefaultSortDir        string          `json:"defaultSortDir"`
    IgnorePatterns        []string        `json:"ignorePatterns"`        // D18 — plain array, user-editable
    UI                    json.RawMessage `json:"ui,omitempty"`          // per-view state, opaque to Go, frontend-owned
}

// Machine — machine-scoped, lives at <app-config-dir>/machine.json.
type Machine struct {
    Workers          WorkerCounts      `json:"workers"`
    MemoryLimitMB    int               `json:"memoryLimitMB"`
    DependencyPaths  map[string]string `json:"dependencyPaths,omitempty"`
    OpenInApps       map[string]string `json:"openInApps,omitempty"`
    TelemetryConsent bool              `json:"telemetryConsent"`
}

// WorkerCounts nests per pipeline — the nesting IS the "Ingest" prefix, for free.
type WorkerCounts struct {
    Ingest IngestWorkers `json:"ingest"`
    // Export ExportWorkers `json:"export"` — added when the export pipeline (separate impl doc) lands.
}

type IngestWorkers struct {
    Hash    int `json:"hash"`
    Extract int `json:"extract"`
    Thumb   int `json:"thumb"`
}

// Keybindings — user-scoped, lives at <app-config-dir>/keybindings.json. Overrides only;
// backend never interprets a command ID or chord, it's the frontend command registry's vocabulary.
type Keybindings map[string]string // commandID -> chord

// configFile wraps ONE JSON file end to end: tolerant load (§3), cached
// in-memory value, atomic save, debounced hot-reload watch (§7), and change
// notification. Settings, Machine, and Keybindings are all *configFile[T] —
// one generic, not three hand-written services with the same shape
// (D19: generics for varying data — the same reasoning already used for
// loadJSON applies one level up, to the whole file wrapper).
type configFile[T any] struct {
    path     string
    mu       sync.RWMutex
    cached   T
    onChange []func(T)
}

func openConfigFile[T any](path string, defaults T) (*configFile[T], error) // subscribes the watch FIRST, then loadJSON (§7 — closes the TOCTOU gap between initial read and watch startup)
func (c *configFile[T]) Get() T
func (c *configFile[T]) Save(v T) error // marshal (indented), atomic temp+rename, update cache, fire onChange
func (c *configFile[T]) OnChange(fn func(T))

// Service is the composition root's single handle onto all three files.
type Service struct {
    Settings    *configFile[Settings]    // per-catalog: opened alongside Store.Open, closed with it
    Machine     *configFile[Machine]     // process-lifetime: opened once at startup, before any catalog
    Keybindings *configFile[Keybindings] // process-lifetime
}
```

## 3. Tolerant loading — the malformed-file degradation path

Two generic functions, not one — cold load (used once, by `openConfigFile`) and live reload (used by
the hot-reload watch, §7) fail differently on purpose, so they're separate functions rather than one
with a mode flag:

```go
// loadJSON is the COLD-START path: no prior in-memory value exists, so a
// malformed file must still produce something to boot with.
func loadJSON[T any](path string, defaults T) T {
    data, err := os.ReadFile(path)
    if errors.Is(err, os.ErrNotExist) {
        return defaults // missing file = defaults, not an error — first run, nothing to warn about
    }
    if err != nil {
        log.Warn("reading config, using defaults", "path", path, "err", err)
        return defaults
    }
    v := defaults
    if err := json.Unmarshal(data, &v); err != nil {
        quarantine(path) // rename to path+".invalid-<UTC timestamp>" — preserved, not deleted
        log.Warn("config file invalid JSON, quarantined and reverted to defaults", "path", path, "err", err)
        return defaults
    }
    return v
}

// reloadJSON is the LIVE path: a good in-memory value already exists (from a
// prior loadJSON or reloadJSON), so failure keeps serving it rather than
// reverting — see §7 for why this diverges from loadJSON's quarantine.
func reloadJSON[T any](path string, current T) (T, bool) { // bool = "did it actually change"
    data, err := os.ReadFile(path)
    if err != nil {
        log.Warn("re-reading config after external edit, keeping previous values", "path", path, "err", err)
        return current, false
    }
    v := current
    if err := json.Unmarshal(data, &v); err != nil {
        log.Warn("external edit produced invalid JSON, keeping previous values (file left untouched)", "path", path, "err", err)
        return current, false
    }
    return v, true
}
```

**Policy:** a malformed config file must never block app or catalog startup.

- **Syntax error (bad JSON)** → whole file quarantined, whole struct falls back to defaults. A
  fresh valid file is **not** auto-written on load — only the next explicit `Save*` call (from a
  settings UI) writes one. This avoids two failure modes: silently clobbering the user's broken
  edit before they've seen the quarantined copy, and leaving nothing to write to if they never
  touch settings again.
- **Bad value, valid JSON** (e.g. `"hash": -3`, an unrecognized `xmpConflictResolution` string) →
  degrades per-field, not per-file. Reuses the exact convention `internal/importer/pipeline.go`'s
  `resolvePools` already applies (`if imp.HashWorkers > 0 { override }` — zero/negative already
  means "unset, use default"). Applying that same clamp after unmarshal means one bad number
  doesn't nuke a file's 11 other good fields. Log at Debug per corrected field (comprehensive
  logging).
- No JSON-schema validation library — a tolerant unmarshal plus per-field sanity clamps covers a
  file with a dozen fields; reaching for a schema validator here would be solving a problem this
  size doesn't have.

Same loader for `settings.json`, `machine.json`, `keybindings.json` — `Keybindings` (a bare map)
degrades the same way: unparseable → quarantine + empty overrides map (defaults win entirely, which
for keybindings just means "frontend's built-in defaults, nothing overridden").

## 4. Read/write hot paths

- **Startup:** `machine.json` + `keybindings.json` load via `loadJSON` before any catalog opens
  (D16's ordering constraint, unchanged). `settings.json` loads when a catalog opens, alongside
  `Store.Open`.
- **Read:** every consumer reads an in-memory struct (loaded once, cached) — no re-read per lookup.
- **Write:** `Save*(path, v)` marshals with indentation (human-editable — these are meant to be
  hand-edited, that's the whole point of choosing files) and does an atomic temp-file+rename, then
  updates the in-memory cache. No general pub-sub: the one live reactor that exists today (worker
  resize, §5) is a direct callback registration, not an event bus — easy to upgrade if a second
  reactor ever needs it.

## 5. Live pipeline resize — mid-run, not just next-run

**The requirement:** a user watching a large ingest run should be able to dial worker counts up
(or down) *during* that run, not just have it apply next time.

**The mechanism — drain current generation, then replace it, same channel throughout:**

```go
// One per stage (hash/extract/thumb). Owns the currently-running worker
// goroutines; resize swaps them out without touching the channel between
// stages, which is just a conduit — nothing in it is lost across a resize.
type stagePool struct {
    mu     sync.Mutex
    cancel context.CancelFunc
    wg     sync.WaitGroup
}

func (p *stagePool) resize(ctx context.Context, n int, in <-chan Item, out chan<- Item, work func(Item) Item) {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.cancel != nil {
        p.cancel()  // current generation: finish in-flight item, then exit — no new item pulled
        p.wg.Wait() // block until fully drained; nothing left in `in`'s buffer is touched or lost
    }
    genCtx, cancel := context.WithCancel(ctx)
    p.cancel = cancel
    for range n {
        p.wg.Add(1)
        go func() {
            defer p.wg.Done()
            for {
                select {
                case <-genCtx.Done():
                    return // priority check: cancellation always wins before touching `in` again
                default:
                }
                select {
                case <-genCtx.Done():
                    return
                case item, ok := <-in:
                    if !ok {
                        return
                    }
                    out <- work(item)
                }
            }
        }()
    }
}
```

The leading non-blocking `select` matters: without it, a plain two-case `select` picks pseudo-
randomly between `genCtx.Done()` and `in` whenever *both* are ready at once, so a busy pipeline could
have a worker pull one more item after cancellation instead of exiting on the spot. Not a
correctness bug (the extra item still gets processed and handed downstream normally) but it defeats
the point of draining promptly — the priority check makes "stop pulling as soon as cancelled" exact
rather than probabilistic.

Because `resize` holds `p.mu` for the whole cancel→drain→relaunch sequence, there is never a moment
where two generations both read `in` — strictly sequential, matching "wait for the pool to drain,
scrap it, spin up a new one" exactly. The channel itself is never recreated, only the goroutines
consuming it — worker count and channel buffer depth are separate knobs, and only the former is
what "how many workers" actually means.

**Why backpressure stays graceful in both directions, not just asserted:**

- **Upstream** (whatever feeds `in` — SCAN for the HASH stage, or the prior stage's own workers):
  during the brief gap between the old generation fully draining and the new one starting to read,
  `in`'s buffer just fills and any upstream send **blocks** — normal Go channel backpressure, not an
  error or a drop. Nothing about resize ever closes `in` or discards what's already buffered; the new
  generation picks up exactly where the old one left off.
- **Downstream** (`out`): a worker mid-flight when cancellation fires finishes its current
  `work(item)` and its `out <-` send *before* returning — `wg.Wait()` doesn't return until that send
  has completed, so no in-flight result is ever discarded. That send blocks like any other channel
  send if the downstream stage is momentarily not draining, which resolves itself once that stage's
  own workers (old or new generation) resume reading.
- **No deadlock if two stages resize at the same moment:** the pipeline is a strict DAG
  (SCAN→HASH→MATCH→EXTRACT→THUMB→WRITE) and `resize` only ever waits on *its own* stage's
  `WaitGroup` — it never waits on a downstream stage's resize to finish. The two singleton stages,
  MATCH and WRITE, are never resized (they're not pools), so the chain always has an always-running
  sink at the end; a worker's blocking send downstream is guaranteed to eventually be accepted rather
  than waiting on another resize that's waiting on it in turn.

**Where this lives:** `internal/importer`, not a new generic pool package — only ingest needs live
resize today (a pipeline run is long-lived enough to matter mid-run; nothing else is yet). Extract a
shared primitive if the export pipeline needs the identical shape later — two consumers, not one, is
when that abstraction earns its keep.

**Wiring (composition root only — see §6):** the app host registers
`svc.Machine.OnChange(func(m Machine) { if run := activeRun(); run != nil { run.Resize(m.Workers.Ingest) } })`.
No active run → no-op; the next run's `resolvePools` already reads current `Machine` values fresh,
so idle-time changes need nothing extra.

**Open item — run-teardown race.** `activeRun()` returning a non-nil run doesn't by itself guarantee
the run isn't already mid-teardown (its stage channels closing) at the exact instant `Resize` runs —
a hot-reload-triggered `OnChange` and a run's own end-of-run cleanup are two independent triggers
that both touch the same `stagePool`. Not resolved here (still spec stage), but the requirement is
concrete: the run's "am I still active" flag and its channel-close must transition under the *same*
lock that `resize` takes, so a `Resize` call either lands cleanly before teardown starts or observes
"already finished" and no-ops — never a partial handoff. Whoever implements this should treat that as
a correctness requirement, not a nice-to-have.

## 6. Interaction with other packages — settings is a leaf; consumers may depend on it

`internal/settings` is a strict leaf: it imports **no** other internal package — no `domain`, no DB,
no `catalog`, no `importer`. That one-directional rule is the load-bearing one (it's what keeps the
dependency graph acyclic).

**Revised (Ari, 2026-07-07):** the original plan had importer/watcher/xmp *never* import settings —
config would only ever reach them as plain constructor fields wired by the composition root. That
held until the D18 ignore list: settings owns the list *and* the matching (`Settings.MatchIgnore` /
`.Ignored`, §8), and the earlier "wire a `func(name)` seam" indirection just scattered nil-guards
and re-implemented, consumer-side, a method that already lives in `settings`. So `internal/importer`
and `internal/watcher` now **import settings and hold a `settings.Settings` value**, calling its
ignore methods directly. This is a deliberate reversal of the leaf-*consumer* rule for those two
packages; settings importing nothing back means no cycle, so the graph stays clean. `internal/xmp`
still doesn't import settings.

The zero `Settings` is the null object (empty patterns → matches nothing), so `Importer{}` /
`Watcher{}` remain valid literals in tests with no fake service required — the testability property
survives the reversal.

## 7. External edits while the app is running (hot-reload)

Files were chosen specifically because they're human-editable (§1) — a user editing `settings.json`
or `machine.json` by hand *while the app is running* isn't an edge case, it's the expected way to
use them, so "restart to pick it up" isn't an acceptable answer.

**Watch each file, debounced, using the dependency already in `go.mod`** — `github.com/rjeczalik/notify`
(already vendored for `internal/watcher`; no new dependency, rung 4 of the ladder). One small watch
per file: `machine.json`/`keybindings.json` for the process lifetime; `settings.json` for as long as
that catalog is open (started/stopped alongside `Store.Open`/close). Debounce ~300ms before acting on
an event — the same order of magnitude as impl/06's outbound-write debounce — because some editors
emit several fsnotify events per logical save (write-then-rename, or in-place multi-write).

**Cold load vs. live reload use the two different functions from §3 on purpose:**

- **Cold load** (`loadJSON`, process/catalog just started, no prior in-memory state): malformed →
  quarantine + defaults, as already specified. There's no "previous good config" to fall back to, so
  a concrete value has to come from somewhere.
- **Live reload** (`reloadJSON`, triggered by the file watcher — `configFile`'s watch callback is
  just `c.cached, changed = reloadJSON(c.path, c.cached)`, firing `OnChange` only if `changed`):
  malformed → do **not** touch the file and do **not** replace the in-memory config — keep serving
  the last-known-good values, log a warning ("external edit to settings.json is invalid JSON, keeping previous
  configuration"). Silently reverting live behavior (worker counts snapping back to defaults,
  `IgnorePatterns` clearing mid-ingest) is more disruptive than just waiting for the user to fix a
  save they're still mid-edit on. Quarantining here would also be presumptuous — the user may well
  save again ten seconds later with the typo fixed.
- **Live reload, valid JSON:** diff against the cached struct, update the cache, fire `OnChange` —
  the exact same path an in-app settings-UI `Save*` call takes (§5's resize hook doesn't know or care
  whether the trigger was a UI action or an external edit).

**No self-write suppression needed.** `Save*` already does atomic temp-file+rename (§4), so the
watcher never observes a half-written file from the app's own writes — it just sees one completed
rename, re-parses identical content, diffs to a no-op, and fires no callbacks. No flag or lock is
needed to distinguish "our write" from "external write."

**Simultaneous external-edit-and-in-app-save collisions are plain last-write-wins**, same as any two
processes touching one file (and the same as VS Code's own behavior here) — not engineered around.
Building any locking for this is solving a problem a single-user desktop config file doesn't have.

**Package boundary unaffected:** the watch is entirely internal to `internal/settings`, which already
owns these files. Consumers still only ever see `OnChange` callbacks, unaffected by whether the
trigger was internal or external — §6's leaf-package property is unchanged.

## 8. Unblocks

- **impl/06:** `xmpWriteBack`/`xmpConflictResolution` are real `Settings` fields (§2).
- **D18:** `Settings.IgnorePatterns` is a plain JSON array now — editing it in the UI is exactly
  editing an array, no key-prefix scheme needed. **Ownership moved here in full (Ari, 2026-07-07):**
  settings owns the list *and* all the matching (`Settings.MatchIgnore`/`Ignored` in
  `internal/settings/ignore.go`), seeded from defaults on first run. The old
  `internal/importer/ignore.go` baked list is gone; SCAN calls
  `Importer.Settings.MatchIgnore(name)` and the watcher `Watcher.Settings.Ignored(name)` — both hold
  a `settings.Settings` value (§6). This resolves the smell that importer's *exported* `Ignored` was
  only ever consumed by the watcher: there is now exactly one owner, and neither consumer carries any
  ignore logic of its own (no scattered nil-guards, no re-implemented matcher).
- **Keybinding editor / machine-config UI:** concrete files + typed structs to read/write.

## Acceptance

- `loadJSON` on a missing file returns `defaults`, no error, no log noise (first run).
- `loadJSON` on malformed JSON: quarantines the file (`.invalid-<timestamp>` sibling exists,
  original content preserved), returns defaults, logs a warning — this is the one non-trivial-logic
  check this doc needs (ponytail: ship one test that fails if the degrade path breaks).
- A single bad numeric field (negative worker count) doesn't zero out the other 10 good fields —
  per-field clamp, not whole-file quarantine.
- `Save*` then `loadJSON` round-trips exactly for all three file types.
- Hot-reload test: writing a valid, changed `settings.json` externally (plain `os.WriteFile`, not
  through `Save*`) is picked up within the debounce window and fires `OnChange` with the new values.
- Hot-reload-invalid test: writing invalid JSON externally leaves `Get()` returning the previous
  values unchanged, and does **not** quarantine the file (still readable/editable as the user left
  it) — the cold-load and live-reload failure paths must diverge exactly here.
- Debounce test: several rapid external writes to the same file within the debounce window produce
  exactly one reload, not one per write.
- Live resize test: pipeline run with 2 hash workers processes N items; mid-run, resize to 4; assert
  every item processed exactly once (no drop, no double-process) and the run finishes with the
  expected total.
- Concurrent-resize test: resize HASH and EXTRACT at (near) the same instant mid-run — asserts no
  deadlock (finishes within a timeout) and the same exactly-once-processing guarantee, exercising the
  DAG argument above under -race.
- Compile-time check: `internal/settings` imports no other internal package (it's a strict leaf, so
  the graph stays acyclic even though importer/watcher now import *it*): `go list -deps
  ./internal/settings/...` contains no other `internal/*`. (`internal/xmp` also still contains no
  `internal/settings`; importer and watcher intentionally do — §6.)
