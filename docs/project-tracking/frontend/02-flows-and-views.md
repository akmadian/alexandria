# Flows and Views

**Status:** design locked 2026-07-07. Constants referenced: C2, C3, C9.

## Design stance

Pro tools live and die by muscle memory; every serious DAM converges on the same skeleton and
users want it that way. **Fresh comes from a few signature concepts on a boring skeleton**, not
from layout invention (the Linear lesson: same lists as Jira; identity came from palette +
keyboard + speed). Alexandria's signature concepts: the query model (`04`), cull speed (`06`),
the Review surface (`07`), transparency-as-chrome (below).

## The four flows

The UI is organized around flows, not features. Every view and shortcut should be justifiable by
one of these:

1. **Ingest day** — card in → import → review the batch → cull → rate/label/tag. High volume,
   keyboard-driven, time-boxed. Benchmark: Photo Mechanic speed, not LrC.
2. **Retrieval** — "I know I have it somewhere" → search/filter → open in app / drag into
   Resolve/InDesign/Finder. The README's core promise; the metric is time-to-file in keystrokes.
3. **Maintenance** — the catalog reporting what changed underneath it: moves, missing files,
   duplicates, XMP conflicts, offline sources. First-class surface (Review), never buried in
   modals. Direct consequence of backend D20 detect-and-flag.
4. **Gardening** — tag hierarchy curation, collection building, metadata presets. Low frequency,
   mouse-friendly, exploratory. Lives in the catalog space.

## The task-shaped test

**An activity gets a full-window takeover ("task view") iff it is task-shaped: it has a
beginning, an end, and you leave when done.** Import, Review, Settings qualify. Search does NOT —
retrieval is how you *dwell* in the catalog (search → refine → browse → search again is one
continuous activity), so it must never fork into a separate view with its own query UI.

Spaces prescribe workflow; view modes don't. This test is the guard against both
over-segmentation and the LrC-module "where am I" tax.

## View inventory

| View | Kind | Notes |
|---|---|---|
| **Catalog** | the one space | Shell below; view modes Grid / Loupe / Compare / Cull (`03`). |
| **Import** | task view | Bespoke flow: source pick, options (dupe-skip, collection-from-import, metadata preset), live pipeline progress, completion summary. |
| **Review** | task view | `07-review.md`. |
| **Settings** | task view | Spawned from the command palette (no chrome real estate spent on a gear icon). |
| **Home** | optional landing | Minimal: recently used collections, saved queries, calls to action (import, find), Review count. Not a dashboard, no greeting. User-disableable — Alexandria works *for them*. |
| **First run** | task view | Empty state with prominent Add Source; keybinding preset picker (`05`). |

Task views never touch catalog view state (C3).

## The catalog shell

Unchanged from the P0 spec, confirmed by this round:

```
+----------------------------------------------------------+
| FilterBar (pills + text field; the query, visibly)        |
+--------+---------------------------------------+---------+
| Browser| Main region (active view mode)        |Inspector|
| Sources|                                       | metadata|
| Collec.|                                       | triage  |
| Tags   |                                       | contain.|
+--------+---------------------------------------+---------+
| StatusBar: context | selection | jobs/health              |
+----------------------------------------------------------+
```

- Panes resize by drag, collapse individually; layout persists (localStorage, pre-paint).
- Browser trees are one reusable hierarchical component across Sources/Collections/Tags modes;
  selecting a node sets the **scope**.
- Inspector adapts per asset type via the type registry (no empty camera-EXIF panels on audio).
- Desktop only; three themes (graphite default / dark / light); chrome is hue-free — hue means
  data (labels, tags, health). CSS Modules + semantic tokens; mono for data values, sans for UI.

## Transparency as chrome

Trust is the product's positioning; *showing the work* is its visual expression. Layers, from
ambient to nerdy:

1. **Status bar** (always visible): left = current query in plain words ("Sources ▸ 2024 ▸
   Iceland · RAW · ★≥3 · 412 assets"); center = selection scope (count, size; hidden when empty);
   right = compact live job/health indicator. Tiny glyph-based telegraphy — box-drawing/block
   characters (`▁▃▆`, `◐`, watcher heartbeat dot) in the mono face; character-swap animation,
   no SVG weight.
2. **Activity drawer** (status bar right zone expands): the Jobs envelope stream rendered
   generically — per-job progress, plain-language history ("Relinked 34 moved files · 2m ago"),
   `message` detail lines.
3. **Dev corner** (deepest drawer tab; discoverable, not advertised): live queue depths, watcher
   event feed, per-stage pipeline timings — the sysde.md observability wishlist as an easter egg.
4. **In-grid marks**: a small corner tick on assets with pending Review items. Subtle,
   unmistakable, never modal.

Principle: chrome that rewards attention without demanding it. The fun lives here and in the few
places color/motion are spent (cull key-feedback overlay, import completion) — never in the way.
