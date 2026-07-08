# State Model

**Status:** vocabulary LOCKED 2026-07-07 (C1/C2). This doc is the full definition set behind the
constants table.

## The equation

```
view state = viewMode(query + arrangement, selection + cursor)
```

Everything on screen derives from five things. One store holds them (the existing
`LibraryProvider` reducer is the right home); view modes are pure renderers and never own copies.
Consequence: view-mode switches are instant, stateless, and workflows *flow* — scan in Grid → E
on a suspect frame → C to compare neighbors → X the loser → G back out, selection intact
throughout. Nothing to learn for beginners (Grid forever is fine); fluency is emergent.

## Query = scope + filter

Mechanically both are predicates and share one WHERE-clause machinery (`../seam/01`). The
distinction is UI grammar, kept for three reasons:

1. **Extensional vs intensional.** A folder or manual collection is a *membership list* (these
   assets, because I said so) — not expressible as an attribute predicate. A filter is a
   *predicate* (whatever matches). Scope is where extensional things live. Smart collections are
   the deliberate bridge: a saved query promoted into the scope tree.
2. **Stickiness.** Scope is navigational and durable — set from the sidebar, survives filter
   changes, returnable. Filters are ephemeral and cheap to clear. Different reset behavior =
   different concept.
3. **Different questions.** Scope: "where am I looking?" Filter: "which of these do I care about
   right now?" Collapsing them makes the filter bar a junk drawer.

The status bar always narrates the current query in plain words; the app never shows a mystery
subset.

## The three tiers (and the targeting rule)

| Tier | Definition | Never confused with |
|---|---|---|
| **Working set** | Everything the query yields. What the grid and filmstrip show, what Cull iterates, what "select all" and export default to. | a user-chosen subset |
| **Selection** | Explicitly chosen subset of the working set. Empty by default. | the working set |
| **Cursor** | The single focused asset (keyboard focus / Loupe subject). Exists whenever the working set is non-empty. | selection |

**Command targeting rule (C5):** verbs act on the selection if non-empty, else the cursor.
Batch operations always name their target ("Export 12 selected" / "Export all 412 results") so
the tiers are visible, never guessed.

## Arrangement

Sort key + direction + grouping (later: pins/custom order, per functional requirements). One
concept spanning both sides of the seam: the backend sees it as ORDER BY input, the frontend as
sectioning keys — implementation location doesn't drive the concept. The invariant that justifies
minting it (C4): **arrangement never changes membership**. Group-by is a derived sort key plus
section headers; sort and grouping are genuinely siblings.

Sort fields include both ingest date and capture date; sequence-rollover handling (sort by
timestamp when filenames roll IMG_9999→IMG_0001) is an arrangement concern.

## View modes

> A **view mode** is a pure renderer over shared catalog state. All view modes present the same
> working set, arrangement, selection, and cursor; only the rendering and the input mapping
> change.

| Mode | Key | Rendering | Notes |
|---|---|---|---|
| **Grid** | G | Virtualized tile grid | The default and the foundation. Density control; configurable card overlays. |
| **Loupe** | E / Enter | Cursor asset large + filmstrip | Per-type body via the type registry: zoomable image, video player, waveform, font specimen, PDF pages. |
| **Compare** | C | 2–4 selected at equal size | Triage controls available without leaving. |
| **Cull** | D | Fullscreen Loupe variant tuned for speed | Lights-out chrome, auto-advance, key-feedback overlay — `05-culling-and-signals.md`. |

- Switches are single keys, instant (<100ms), stateless — no navigation stack, no lens-on-lens.
- **Escape always steps down toward Grid.** Grid is home.
- Space anywhere in Grid = quick preview (Finder Quick Look convention), dismiss with Space —
  a peek, not a mode change.
- Per-type presentation lives *inside* modes: the mode is the frame, the type registry fills it.
- The set is extensible (Map is a future view mode) but each addition must satisfy the pure-
  renderer definition — a candidate needing its own private query UI is a task view, not a mode.

## Persistence

Per the settled settings architecture: layout/theme/density/current view mode → localStorage
(pre-paint chrome); saved queries (smart collections) → catalog; keybindings →
`keybindings.json`. Selection, cursor, and un-saved filters are session state — deliberately not
persisted.

## Pattern lineage

Single store + pure renderers = the Flux/Redux "derive, don't store" discipline (already codified
in frontend/CLAUDE.md). Extensional/intensional is set-theory vocabulary; the scope/filter split
mirrors Finder sidebar+search-tokens and LrC sources+Library-filter, minus their inconsistencies.
