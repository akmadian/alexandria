# Frontend Architecture

How the web frontend talks to the Go backend over Wails: the binding surface, access patterns, caching, resource discipline, error handling, and the expected load envelope.

This document assumes the backend architecture in `docs/original prd/02-architecture.md` (Wails confined to `app/`, typed IPC, frontend never owns catalog state) and refines its "no cache, always re-query" rule into something that is also resource-respectful.

---

## 1. Framing: load is human-bounded, not throughput-bounded

Alexandria is a **single-user local desktop app**. No network, no concurrent clients. IPC is sub-millisecond; indexed SQLite reads are sub-10ms. "TPS" in the server sense does not apply — load is **bursty and interaction-bounded**. The design goal is *burst smoothness*, not sustained throughput.

This tells us where **not** to spend complexity. The metadata query path (SELECTs) is cheap; a normalized client store or aggressive metadata cache would be over-engineering. The genuinely finite resources are three, and the whole design spends its budget on them and stays dumb everywhere else:

1. **Thumbnail decode + disk IO** — the real hotspot.
2. **Render churn** in the virtualized grid.
3. **Write amplification** from high-frequency triage input.

---

## 2. The seam: four channels, six conventions

The seam is the **only** place that touches `window.go.*` or `runtime.*`. Everything else imports the `AlexandriaAPI` interface. This keeps the app testable against a mock and makes the real backend a one-line swap (`createMockApi()` → `createWailsApi()`).

*Implemented* in `frontend/src/api/`, one job per file:
- `contract.ts` — the **contract**: seam types, the `AlexandriaAPI` interface, and `ApiError`. A runtime leaf with no dependency on any implementation or on React. Re-exports the domain models, so app code has one door: `import type { … } from "@/api/contract"`.
- `models/` — the **domain shapes**, one file per entity (`asset.ts`, `source.ts`, `collection.ts`, `tag.ts`) plus `enums.ts` and a barrel. Hand-written today; this directory is where Wails-generated types (from Go `internal/domain`) land later, invisibly to everything above it.
- `mock-api.ts` — `createMockApi()`, the in-memory implementation the frontend runs against today. `wails-api.ts` (`createWailsApi()`) joins it later; `mock-api.check.ts` is a framework-free behavioral check (`node src/api/mock-api.check.ts`). `mock.ts` holds seed data + format helpers.
- `queries.ts` — the **query layer** (§7): the TanStack hooks components consume, plus the live `api` singleton at its top (its only consumer). Swapping `createMockApi()` → `createWailsApi()` there is the one-line change to go real. Hooks stay separate from the contract so `contract.ts` remains framework-agnostic.

The boundary is **thin**: type translation, event subscription, and error normalization. It does **not** contain caching, debouncing, or coalescing — those live one layer up in the query layer (§7), because they are consumer concerns, not transport concerns.

The seam has exactly **four channels**. Every feature, present or future, maps onto one of them:

| Channel | Direction | Carries |
|---|---|---|
| **Commands** (Wails bindings) | JS → Go, Promise | queries and mutations — structured data only, never file bytes |
| **Events** (Wails runtime) | Go → JS, push | change notifications, job progress, status |
| **Asset URL scheme** (Wails asset handler) | webview → Go http.Handler | binaries: thumbnails, previews, originals |
| **Undo stack** (backend command pattern) | implicit | every undoable mutation routes through it in the app layer |

### Conventions (the actual API contract)

The binding tables below are the *initial* surface, not the final one. What is stable is the set of rules that governs how the surface grows:

1. **Resource-oriented verbs.** Each entity gets `list / get / create / update / delete` plus a small number of entity-specific verbs. The surface grows with *entities* (rare — the domain has ~9, including deferred ones), never with *features* (common). Known-entity ceiling: roughly 50 bindings at full maturity.
2. **Envelopes absorb field growth.** A new filterable attribute is a field on `AssetFilter`; a new editable attribute is a field on `AssetPatch`; a new preference is a field on `Settings`. **Never add per-field bindings** (`setRating`, `setFlag`, …). If a feature can be expressed as an optional field on an existing envelope, it must be.
3. **One job envelope.** Every long-running operation (import today; reconciliation scans, integrity checks, XMP resync, thumbnail rebuilds tomorrow) is started by one binding returning a `jobId` and reports through the shared `job:*` event shapes. New job kind ≠ new event shape.
4. **Binaries never cross IPC.** Anything that is file bytes goes over the asset URL scheme (§6). Commands carry metadata about files, never their content.
5. **Codes cross the seam, not strings.** Errors and enums arrive as stable identifiers (`ApiError{kind, code}`, `FileType`, …); display text is the frontend's job. This is the i18n constraint from the PRD applied to the seam.
6. **The frontend tolerates unknown enum values.** A new `FileType` or job kind added on the Go side must degrade gracefully (generic icon, generic label), not crash the grid. Forward compatibility is a frontend rendering rule, not a versioning scheme.

Domain types are **generated, never hand-written**: the Go `internal/domain` structs are the single source of truth and Wails emits the TS models from the bound method signatures. `api.ts` re-exports the generated types; nothing else defines a domain shape. (Watch item: verify the v2 generator handles `Opt[T]` and `time.Time` acceptably; if not, the app-layer DTOs use explicit JSON-friendly shapes and the generics stay backend-internal.)

### Command surface (JS → Go, Promise-returning)

**Assets**

| Binding | Kind | Undoable | Notes |
|---|---|---|---|
| `listAssets(query) → {items: AssetRow[], total}` | read | — | §3–4; bursty on scroll/filter, coalesced |
| `getAsset(id) → Asset` | read | — | full record; inspector only |
| `patchAssets(target, patch)` | write | ✓ | `target = {ids} \| {scope, filter}` — single, multi-select, and select-all-matching through one verb |
| `setAssetTags(id, tagIds)` | write | ✓ | |
| `removeFromCatalog(target)` | write | ✓ | soft delete |
| `deleteFromDisk(ids)` | write | ✗ | double-confirmed; **ids only, never a query** — destructive ops require explicit enumeration |
| `openAsset(id)` / `openWith(id, app)` / `revealInFileManager(id)` | command | — | the "open" in find-see-open |
| `getFolderTree(sourceId)` | read | — | sidebar filesystem view |

`target` accepting a query matters because "select all matching this filter, then rate/tag/collect" must not ship 100k UUIDs over IPC. Query-target mutations are still undoable: the backend command captures the affected ids and before-values *at execution time*, so the inverse is always concrete.

**Sources**

| Binding | Undoable |
|---|---|
| `listSources` / `createSource(def)` / `updateSource(id, patch)` / `removeSource(id)` | ✗ (per PRD) |
| `pickDirectory() → path` — native directory dialog via Wails runtime, needed by Add Source | — |

**Import & jobs**

| Binding | Notes |
|---|---|
| `startImport(sourceId) → jobId` | progress/summary arrive via `job:*` events |
| `cancelJob(jobId)` | generic — cancels any job kind |

**Tags**

| Binding | Undoable |
|---|---|
| `tagTree` / `createTag` / `updateTag(id, patch)` / `deleteTag(id)` | update/delete ✓ (catalog edits) |

`updateTag`'s patch covers rename, reparent, and color — one verb, envelope absorbs the rest.

**Collections**

| Binding | Undoable |
|---|---|
| `listCollections` / `createCollection` / `updateCollection(id, patch)` / `deleteCollection(id)` | ✓ where they edit catalog state |
| `addToCollection(id, target)` / `removeFromCollection(id, target)` | ✓ |

**Settings & keybindings**

| Binding | Notes |
|---|---|
| `getSettings` / `updateSettings(patch)` | one envelope, grows with the Settings struct |
| `listKeybindings` / `setKeybinding(action, combo)` / `resetKeybindings()` | `setKeybinding` returns `ErrKeybindingConflict` (§8 handles it) |

**History**

| Binding | Notes |
|---|---|
| `undo()` / `redo()` | menu state comes from `history:changed` events, not polling |

**Deferred, pre-named to prove the conventions hold** (do not implement): `groupAssets` / `ungroupAssets` / `setGroupCover` (P1 groups); `listDuplicates` / `resolveDuplicate` (duplicate review); `startIntegrityCheck` (job). Each lands as standard verbs on an existing channel.

### Event surface (Go → JS, push)

| Event | Payload | Consumer |
|---|---|---|
| `catalog:changed` | `{ scope?, ids? }` | active view re-query (§5, §7) |
| `job:progress` | `{ jobId, kind, done, total, stage? }` | progress chrome (import panel today) |
| `job:done` | `{ jobId, kind, summary?, error? }` | import summary modal, toasts |
| `source:status` | `{ sourceId, status }` | source dots in sidebar |
| `history:changed` | `{ canUndo, canRedo, undoLabel?, redoLabel? }` | Edit menu state |
| `update:available` | `{ version, url }` | update indicator |

---

## 3. The query model: scope × filter

A collection is not a filter. A filter is a *predicate* ("rating ≥ 4"); a collection is an *extensional set* — a specific, user-curated subset of the library. Conflating them (stuffing `collectionID` into the predicate struct) muddles both. Lightroom Classic gets this right with two separate UI affordances: the source panel (*where* you're looking) and the filter bar (*which of those* you see). The seam mirrors that:

```ts
ListQuery {
  scope:  { kind: "library" }
        | { kind: "collection", id: string }        // manual or smart — backend resolves either
        | { kind: "folder", sourceId, path, recursive }
        // P1, reserved: { kind: "group", id }
  filter: AssetFilter        // pure predicate — no sort, no paging, no scope
  sort:   { field, dir }
  page:   { limit, offset }
}
```

Every "view" in the app is a `ListQuery`: Recent = library scope + sort by added desc; Picks = library scope + `flags:["pick"]`; a collection = collection scope (+ whatever the filter bar says); Previous Import = collection scope (it *is* a collection, per the PRD); the filesystem sidebar = folder scope. **Smart collections evaluate on the backend** — the frontend passes a collection scope and never sees or interprets the stored query JSON (except, later, in the editor UI).

`AssetFilter` is predicate-only. Beyond the backend's current fields it needs, per the PRD's own search requirements:

- **Absence queries** — `unrated`, `unflagged`, `unlabeled`, `untagged` booleans. Triage lives on "show me what I haven't triaged"; the current exact/min fields cannot express `IS NULL`.
- `fileStatus` — surface missing/offline assets as a filterable state.
- Planned predicate fields (add when the filter bar UI reaches them, envelope rule): camera make/model, dimensions, duration.

Backend note: the flat AND-only `AssetFilter` cannot express the deferred smart-collection query JSON (`{and:[…], or:[…]}`). Either smart collections are AND-only in P1, or the repository grows a nested-expression path then. Flagged now so the `collections.query` format isn't treated as settled.

### The folder view: derived, never stored

Folders are not an entity. No folders table, no folder CRUD bindings — the tree is **derived from asset paths** (`source_id` + `relative_path`; the path includes the filename, the directory is the path minus its last segment). Consequences, all deliberate and all matching LrC:

- Only folders containing indexed assets appear. Empty directories don't exist as far as Alexandria is concerned.
- File moves and renames (watcher) update `relative_path`; the tree re-derives. There is no folder state to reconcile, ever.
- **Path invariant:** separators are normalized to `/` at ingest on every platform (Windows included); folder grouping is byte-wise on the stored path.

One binding:

```ts
getFolderTree(sourceId) → FolderNode
FolderNode { name, path, directCount, totalCount, children: FolderNode[] }
```

Built in Go from a single index-only walk of `idx_assets_source_path` (payload and build cost scale with *distinct directories* — hundreds to low thousands in a typical archive — not with assets), cached backend-side per source, invalidated by `catalog:changed`. On the frontend it's Tier-1 reference data (§7), fetched when a source is first expanded in the sidebar.

Browsing a folder is the already-reserved scope: `{kind:"folder", sourceId, path, recursive}` (path `""` = source root; `recursive` defaults true, with an LrC-style "include subfolders" toggle). The backend serves it as a prefix query on the existing unique index (`relative_path GLOB 'p/*'`; non-recursive adds `NOT GLOB 'p/*/*'`).

`ponytail:` derived-on-demand tree + GLOB prefix on the existing index; upgrade path if profiling objects is an ingest-written `dir_path` column + `(source_id, dir_path)` index — a migration, not a redesign.

---

## 4. Projections: `AssetRow` vs `Asset`

The domain `Asset` is ~40 fields including the `ExtendedMetadata` blob. Shipping 200-row pages of that through JSON IPC is the base64-thumbnail mistake in milder form. Two fixed projections:

- **`AssetRow`** — what a grid card needs, nothing more: `id, filename, extension, fileType, fileStatus, rating, colorLabel, flag, width, height, durationSecs, capturedAt, thumbURL`. Returned by `listAssets`, ~15 fields, cheap to serialize by the hundreds.
- **`Asset`** — the full record. Returned by `getAsset`, fetched once per inspector selection.

`listAssets` returns `{items, total}` where an item is *structurally open*: when P1 asset groups land, a grid card is asset-or-group, and `items` gains a `kind` discriminator (`AssetRow | GroupRow`) without reshaping the hottest endpoint in the app. This is why the return type is not a bare `AssetRow[]`.

Two fixed projections capture what field-selection query languages (GraphQL et al.) would buy here, without the schema/resolver machinery — there is one client, both sides are one codebase, and IPC has no network cost to optimize away.

---

## 5. Access patterns

- **Grid:** virtualized. Renders the visible window plus a padding buffer (start ±1 viewport; tune against WebKitGTK). Queries pages on demand via `page.limit/offset` with a small prefetch-ahead. The grid holds a **sparse windowed array** indexed by offset, never the whole library.
- **Filter change:** debounce text input ~200ms; discrete controls (type, min-rating) fire immediately. A change resets the window and **supersedes** in-flight list queries (§7).
- **Scope change** (sidebar click): fires immediately, resets window, clears filter bar or preserves it per UX decision — either way it's just a new `ListQuery`.
- **Inspector:** `getAsset(id)` on selection; cached by id.
- **Reference data:** `listSources` / `listCollections` / `tagTree` fetched once at startup, refreshed only on `catalog:changed`.
- **`total` caveat:** `COUNT(*)` over a filtered 500k catalog (especially with FTS) can be the slowest query on the page. If profiling shows it, decouple: return rows immediately, deliver `total` lazily (or adopt the id-snapshot below, which makes `total` free).

### Windowed queries over a filtered, sorted set

The grid's window is a contiguous slice of the *result set* (scope × filter × sort), not of the table — matching rows are scattered across the file, and that's fine. `ORDER BY … LIMIT n OFFSET m` is exactly "give me result-set positions m..m+n": SQLite walks an index that matches the sort order, applies the residual predicates per row, discards the first `m` matches, returns `n`. Non-adjacency on disk costs page reads, not correctness.

**Determinism rule:** every `ORDER BY` appends `id` as the final tie-breaker. Without it, equal sort keys — burst shots sharing a capture timestamp, identical ratings — make page boundaries nondeterministic, and rows silently duplicate or vanish at page seams even when the result set itself is stable.

Two real problems, one shared upgrade path:

1. **`OFFSET` is O(m).** The walk-and-discard means a query at offset 400k scans 400k index entries first. Fine at small offsets (normal scrolling); tens of ms at the far end of a 500k catalog (scrollbar yank to the bottom). Mitigate first with covering indexes on the common sort fields so the discard phase never touches the main table.
2. **The result set moves under the window.** Triage writes and import batches change membership and order between page queries — rows duplicate or vanish at page seams. Mostly mitigated by UX policy, not code: **the grid does not live-reorder on triage writes** (an asset rated while sorted-by-rating keeps its position until the view is re-entered or refreshed — LrC behaves this way, and live reflow during keyboard triage is hostile anyway). `catalog:changed` reconciliation refreshes the visible window in place.

**Named upgrade — the id snapshot** (`ponytail:` don't build until profiling demands it): on view change, materialize the ordered id list once (`SELECT id … ORDER BY …`, index-only, ~50–100ms worst case at 500k, off the interaction path) and hold it in Go. Then: window fetch = `WHERE id IN (page slice)` at O(page); `total` = `len(ids)`, killing the COUNT query; window stability = free, because positions are pinned until the snapshot rebuilds on `catalog:changed`/filter change (coalesced by the existing debounce). ~4–8MB at 500k ids. One mechanism retires both problems and the `total` caveat — which is why it's *the* designated ceiling-raiser, and why nothing cleverer (keyset pagination hybrids) should be attempted before it.

---

## 6. The binary channel: thumbnails, previews, originals

All file bytes are served through the **Wails asset handler** (a Go `http.Handler` behind the webview's app origin — this *is* "reading directly from disk", just through a controlled origin; see §14 FAQ). Three endpoints, one URL scheme:

| Path | Generated | Serves |
|---|---|---|
| `/thumb/{assetId}?v={thumbHash}` | at ingest (existing pipeline) | grid tiles |
| `/preview/{assetId}?v={hash}` | **on demand**, cached | fullscreen/detail view for non-webview-renderable formats (RAW, PSD, AI, INDD, Affinity) |
| `/original/{assetId}` | — | streamed original for webview-renderable types; **must support HTTP Range** (video/audio scrubbing) |

**Previews are generated on demand, not at ingest.** Rationale: most assets never get opened fullscreen; ingest stays fast; disk stays small. "Generate" is a ladder, fastest rung first:

1. **Extract embedded preview** (`exiftool`, milliseconds): modern RAW (ARW, CR3, NEF, DNG…) almost always embeds a full-resolution JPEG — serve it as-is, no re-encode. PSD embeds a full-size composite *if* saved with "Maximize Compatibility" (usually on). Affinity/INDD embed moderate-resolution previews — fine for fullscreen, this is the documented ceiling for those formats anyway.
2. **Render** (libraw / ImageMagick / Ghostscript subprocess, seconds): the fallback when there's no usable embedded preview — always the path for AI/PDF/SVG, occasionally for old RAW with small embeds or PSDs saved without compatibility mode.

Either way the result lands in an app-data `previews/` directory, LRU-pruned, rebuildable, never a backup target (same contract as thumbnails). Extraction being the common case is what makes on-demand feel instant.

**Deferred, additive:** pixel-perfect zoom / true full-quality renders. When needed, it's a new size tier (`/preview/{id}@2x`) or a tiled endpoint on this same channel — no command or event changes. Deferring it costs nothing structurally.

URLs are content-addressed (`?v={hash}` from `ThumbnailAt`/preview hash) so regeneration busts caches, and served with far-future `Cache-Control: immutable`.

> **Do not** ship image bytes as base64 over IPC. It defeats the browser cache, bloats every `listAssets` payload, and turns the cheapest-to-cache resource into the most expensive.

> **Measured (2026-07, `spikes/grid-cache-spike`, macOS/WKWebView, Wails v2.12):** there is **no durable HTTP cache** on the `wails://` custom scheme — only WebKit's size-bounded in-memory image cache. A 21MB thumbnail working set (3k tiles) scrolled back with ~1% handler refetches; a 213MB set (10k × 21KB) refetched 87% on scrollback. **And it doesn't matter:** the 87%-refetch pass was frame-for-frame identical to the cached one (p95 18ms, 1.35% dropped, sustained fling over 10k tiles). Handler re-serves of ~20KB files are frame-rate-neutral. Consequences: keep the immutable header (free win while the memory cache holds), build **no** frontend thumbnail LRU, and treat "hundreds/s thumbnail demand" in §11 as genuinely fine even uncached. macOS perf gate: **passed**. Remaining gate: the same run on WebKitGTK/Linux (spike README has instructions).

---

## 7. Caching & throttling: TanStack Query + a custom thumbnail loader

The PRD's "no cache" is reconciled as: **no normalized store, but a thin stale-while-revalidate layer** — and that layer is **TanStack Query**, not hand-rolled machinery. Single-flight dedupe, SWR, LRU (`gcTime`), request cancellation, and query-key-based superseding are exactly its feature set, it's the canonical "fewer, well-maintained packages" dependency, and it deletes roughly half of this section's implementation burden.

*Implemented* (`frontend/src/api/queries.ts`, `providers/query-provider.tsx`): typed hooks (`useAssets`, `useAsset`, reference-data hooks, `usePatchAssets`), the query-key map below, optimistic triage (§10) via `onMutate`/rollback across both list and detail caches, and `useCatalogSync()` — the `catalog:changed`/`job:done`/`source:status` → `invalidateQueries` bridge, mounted once at the library root. Components consume hooks only; they never touch `api`. Verified end-to-end in the browser: grid renders `AssetRow` pages, selection fetches the full `Asset`, a rating edit updates the selected card and inspector instantly while leaving other rows untouched. The thumbnail loader (below) stays the one bespoke piece and is not built yet.

Query-key conventions:

- `["tags"]`, `["collections"]`, `["sources"]` — reference data; `staleTime: Infinity`, invalidated only by `catalog:changed`.
- `["assets", query]` — list pages, keyed by the serialized `ListQuery`; SWR, `gcTime` ~small (a browsing cache, not a DB mirror).
- `["asset", id]` — inspector details.

Invalidation: `catalog:changed` → `invalidateQueries` — coarse by default, scoped when the payload allows (§8). A burst of events during an import collapses via a trailing debounce (~75ms) before invalidating, and only *mounted* queries refetch (TanStack's default behavior — background views cost nothing).

What stays custom, because it's genuinely bespoke:

- **Thumbnail loader:** TanStack caches query *data* (JSON), not `<img>` resource loads — image bytes never enter it. The loader is a small standalone module: `fetch()` + `AbortController` per tile, a bounded priority queue (~8–16 in-flight, visible-first, cancel off-screen, pause during fast scroll / resume on settle), object URLs handed to `<img>`. Grid-resolution during scroll; preview-resolution only after settle.
- **Input debouncing:** ~200ms on filter text; continuous controls (rating scrub, if any) debounce writes ~300ms so the backend never sees intermediate states.

~~Contingency: an in-memory LRU of object URLs in the loader if the webview cache fails.~~ **Resolved by the §6 spike measurement: don't build it.** Scrollback with an 87% cache-miss rate was frame-for-frame identical to fully-cached scrollback — handler re-serves are free at thumbnail sizes. The loader owns the fetch path for priority/cancellation reasons only.

Net: idle → ~0 calls. Active scroll → a handful of coalesced <10ms list queries + cache-served thumbnails. Triage → discrete writes at human speed. Nothing sustained.

---

## 8. Event granularity

`catalog:changed` can be **coarse** (no payload → invalidate active queries) or **scoped** (`{scope, ids}` → skip invalidation when the change is outside the current view, or patch just those rows).

**Decision:** the payload type carries optional `scope`/`ids` from day one, but consumers may ignore them and invalidate coarsely. Start coarse (correct and already respectful); adopt scoped invalidation only if profiling shows wasteful refetches. `job:*` and `source:status` are separate, non-catalog events that update chrome without touching the query cache.

---

## 9. Error handling & surfacing

Four client categories, mapped from the backend's typed-error tiers (`docs/original prd/10-error-handling.md`), each with a distinct surface:

| Category | Examples | Retry | Surface |
|---|---|---|---|
| **Transport / IPC** | backend gone, missing binding | no | app-level banner ("Backend not responding — restart") |
| **Expected / degraded** | `SourceOffline`, thumbnail missing | n/a | inline status only (offline badge, placeholder tile) — never a red toast |
| **Typed domain** | validation, `ErrKeybindingConflict`, not-found | no | contextual: inline field error, or a specific prompt |
| **Unexpected** | unhandled Go error | reads: 1 auto-retry; writes: never | toast + log, with manual retry |

Principles:

- **Reads are idempotent → auto-retry once** with small backoff. **Writes never auto-retry** (avoid double-apply); offer manual "Retry" in the toast.
- **Degraded ≠ error.** A NAS being offline is the *normal* state the app is designed around — calm inline treatment, never an alarm.
- **Lean on the undo stack, not custom rollback.** Destructive actions surface a toast with an Undo affordance backed by the Go command stack, rather than the frontend maintaining its own inverse state.

The boundary module normalizes every failure into a single `ApiError { kind, code?, detail? }` so consumers switch on `kind`/`code` rather than sniffing strings — and so display text stays frontend-owned (convention 5).

---

## 10. Optimistic updates — one considered deviation from the PRD

The PRD says "no optimistic updates, always re-query." We carve out **one narrow exception: single-asset triage edits** (rating / flag / color label). These are the highest-frequency, most latency-sensitive interaction — a photographer rating 2,000 photos. A full write → `catalog:changed` → refetch round-trip per keypress adds visible flicker to exactly the workflow that must feel instant.

**Approach:** optimistically reflect rating/flag/label in the local window immediately, fire the write, reconcile on `catalog:changed`, and on error revert + toast (TanStack's `onMutate`/`onError` rollback is the implementation). Everything else stays pessimistic/re-query. This is a real trade-off (a brief on-screen lie if a write fails) confined to the three cheapest-to-reverse fields.

This lives entirely in the query layer; the binding module stays neutral.

---

## 11. Expected load envelope

| Path | Idle | Active | Backend cost |
|---|---|---|---|
| Writes (triage) | 0 | ~1/s sustained, 3–5/s burst | 1–5ms SQLite write; no batching needed |
| `listAssets` | 0 | ≤5/s typing (→1 on settle); 5–10/s scroll fling | <10ms each, coalesced |
| Thumbnails | 0 | hundreds/s *demand* on fast scroll | one-time generation; steady state cache-served |
| Previews | 0 | ~1 per fullscreen open | on-demand generate (fast path: embedded-JPEG extraction), then cached |
| Reference data | ~0 | ~0 | event-driven only |

The takeaway: sustained backend load is near zero. The only high-*volume* path is thumbnails, and it is designed to be one-time cost + cache.

---

## 12. Change playbook

The test of the seam is not the v1 feature list — it's what each *likely future change* touches. Gamed out:

| Change | Touches | New bindings |
|---|---|---|
| Show another metadata field in the inspector | `domain.Asset` field + extractor; regenerated TS; inspector component | 0 |
| Make a field filterable | `AssetFilter` field + WHERE clause + index; filter-bar control | 0 |
| Make a field user-editable (title, caption, capture date…) | `AssetPatch` field + validation + XMP mapping; inspector control. Undo is generic over patches — no new command code | 0 |
| Show a new datum on grid cards | `AssetRow` field | 0 |
| Support a new file type | dispatcher map entry (+ maybe a thumbnailer); frontend renders unknown `FileType` generically (convention 6) | 0 |
| New long-running operation (integrity check, XMP resync, thumbnail rebuild, batch rename) | one `startX` binding; `job:*` envelope carries progress/summary | 1 |
| New entity (duplicate review, persons/faces for ML tagging) | standard verb set + `catalog:changed` | 3–5 |
| Asset groups (P1) | `items` kind discriminator (§4, reserved) + `scope {kind:"group"}` (§3, reserved) + group verbs | 3 |
| Smart collections (P1) | backend query evaluator; browsing already works via collection scope; editor updates `collection.query` (+ optionally one preview-evaluate binding) | 0–1 |
| Zoom / full-quality renders | new size tier or tiled endpoint on the URL scheme | 0 |
| Localisation | nothing at the seam — codes already cross, strings don't (convention 5) | 0 |
| AI tagging (P2) | tags arrive with `source:"ai"` (already in `AssetTag`); confidence = new field; review UI = filter by tag source | 0 + entity verbs if faces ship |

**Redesign tripwires** — if any of these appear in a diff, a convention has been violated and it's time to stop and rethink: a per-feature progress event shape; a per-field mutation binding; the frontend parsing smart-query JSON; file bytes in a command payload; a second list endpoint that duplicates `listAssets` for some scope.

---

## 13. Wails v2 now; v3 migration is contained

Wails v3 (services model, static-analysis binding generator, multi-window) is still **alpha** as of July 2026. Build on **v2**. The exposure is deliberately contained: v3 migration touches `app/` (bound structs become registered services) and the internals of `api.ts` (generated import paths) — nothing above the seam. v3's services-can-serve-HTTP feature maps cleanly onto the binary channel (§6) when the time comes.

---

## 14. FAQ: why not…

- **…`file://` URIs straight into the webview?** Webviews treat `file://` as a separate, privileged origin and block it from the app origin by default (per-platform overrides exist, are inconsistent, and forfeit versioned URLs + path control). The asset handler *is* direct disk reading — a Go `http.Handler` streaming the file inside the app's origin, no IPC copy, with Range support and cache-busting.
- **…GraphQL?** Field selection and schema evolution solve *network cost* and *many independent clients*. Here: one client, one codebase, sub-ms IPC. Two fixed projections (§4) capture the fetch-only-what-you-need benefit; a Go GraphQL runtime + resolvers + codegen would replace Wails' free typed bindings with strictly more machinery.
- **…an IDL (Smithy/protobuf-style) for shared domain types?** That pattern earns its keep at N services × M languages × many teams. Here there are two languages in one process, and the Go domain package already *is* the schema — Wails generates the TS side. An IDL would be a third representation to keep in sync with the two you get for free.

---

## 15. Open design areas

Known-undesigned, deliberately so. None blocks the seam; each needs a decision before its feature ships:

1. **Video fullscreen playback.** `/original` + Range covers delivery, but webview codec reality doesn't: WebKitGTK codecs vary by distro, and ProRes/MKV/AVI won't play in any of the three webviews. Recommended v1 posture: play what the webview can, poster-frame + prominent "Open in app" for the rest. Bundled-ffmpeg transcoding is a rabbit hole — don't.
2. **Multi-select inspector.** Mixed-value display and editing semantics ("rating: ✱ (varies)") are undesigned. The write side is already covered (`patchAssets` with multiple ids); the read side may want a batch `getAssets(ids)` if profiling shows N inspector fetches mattering. UX design task first.
3. **Selection semantics with query targets.** After "select all matching" (a query-target selection), what happens when the user then edits the filter? Pure UX decision; the seam supports either answer.
4. **Drag-out to other apps.** Dragging assets to Finder/Photoshop is a known weak spot of webview shells (Wails v2 has no first-class file drag-out). Internal drag (assets → collections) is plain frontend DnD and fine. Likely deferred or needs a small native shim; decide when the workflow demand is real.
5. **Startup choreography.** The PRD's <3s-interactive target implies skeleton-first rendering with queries filling in behind. Implementation detail, but worth a pass when the shell exists.
6. **FTS + sort at scale.** `searchText` combined with a non-relevance `ORDER BY` over 500k means FTS candidate set → join → sort. Belongs in the synthetic-catalog microbenchmark, not in speculation.

---

## 16. Decisions

Resolved (previously "pending confirmation"):

1. **Optimistic single-asset triage edits** (§10) — **yes**, confined to rating/flag/label.
2. **`catalog:changed` scope** (§8) — scoped-capable payload, coarse consumption to start.
3. **Thumbnail delivery** (§6) — Wails asset handler, immutable content-addressed URLs. *Verified on macOS (2026-07, `spikes/grid-cache-spike`):* no durable custom-scheme HTTP cache, but re-serves are frame-rate-neutral — no frontend cache needed. WebKitGTK run still open.
4. **Previews** (§6) — generated on demand with embedded-preview fast path; zoom/full-quality deferred as an additive tier.
5. **Query layer** (§7) — TanStack Query; custom code only for thumbnail loading discipline.
6. **Query model** (§3) — scope × filter, collections are scopes not predicates.
7. **Wails v2** (§13) — v3 revisited at beta; migration contained to `app/` + `api.ts` internals.
