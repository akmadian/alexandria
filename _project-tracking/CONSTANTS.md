# Constants — cross-cutting load-bearing invariants

**Status:** LOCKED (Ari, 2026-07-07, frontend/UX design round). Every rule here was decided
deliberately, with tradeoffs discussed. Code, docs, and future design sessions may not contradict
this file casually — if a rule stops fitting, reopen it *explicitly* (note the change here with
date + rationale), don't route around it.

This file sits above the per-area docs because these rules span concerns: product principles bind
everything; the vocabulary binds frontend *and* backend; the seam rules bind both sides of the
contract. C-numbers are stable identifiers (like the decision log's D-numbers) — never renumber.

Names are cheap; boundaries are expensive. Renames are an afternoon of LSP + grep and all UI copy
is i18n data anyway. The rules below are almost all *boundary* rules — the ones whose violation
costs a solo dev a lost week.

---

## Product principles

### C11. AI produces data, never verdicts

Every model/algorithm output is a **metadata column** (sharpness, blink probability, phash
cluster, …) — filterable, sortable, thresholdable through the same query system as everything
else. The system may *suggest* (suggested rejects, proposed groups) but only the user *disposes*,
and suggestions arrive as reviewable sets, never applied mutations. This is the UX face of
backend D20 (detect-and-flag) and the positioning line: the AI does the measuring, the user does
the judging.

### C12. NL parses to a visible query, never to opaque results

Natural-language input compiles to AST pills the user can inspect, correct, and save. A search
feature that returns results without showing what it understood is off the table. NL is an
optional tier: with it off, the deterministic parser still runs and leftover words become FTS
terms (today's baseline behavior everywhere).

---

## Vocabulary and state (frontend-owned, backend-shared)

### C1. Vocabulary

Shared between frontend and backend. Code maps to these words; UX copy maps to these words.
Full definitions in `frontend/02-state-model.md`.

| Term | One-liner |
|---|---|
| **Scope** | Where you're looking: source, folder, collection, tag subtree. Navigational, durable, set from the sidebar. |
| **Filter** | Predicate tokens applied within the scope. Ephemeral, cheap to clear. |
| **Query** | Scope + filter. The saveable thing (→ smart collection). Compiles to SQL. |
| **Working set** | Everything the query currently yields. Never a user-chosen subset. |
| **Selection** | The explicitly chosen subset of the working set. Empty by default. |
| **Cursor** | The single focused asset. Exists whenever the working set is non-empty. |
| **Arrangement** | Sort key + direction + grouping. Orders and sections the working set; never changes membership. |
| **View mode** | A pure renderer over shared catalog state: Grid, Loupe, Compare, Cull. |
| **Task view** | A full-window enter-act-leave flow: Import, Review, Settings. |
| **Pill** | The rendered form of one AST leaf in the filter bar (macOS search-token style). |

### C2. The state equation

```
view state = viewMode(query + arrangement, selection + cursor)
```

View modes are **pure renderers** — same working set, arrangement, selection, and cursor; only
the rendering and the input mapping change. One store holds the state; view modes never own
copies of it. (This is what LrC modules got wrong; the sync burden is the rot.)

### C3. Task views never touch catalog view state

Returning from Import/Review/Settings restores the catalog *exactly* as left: query, selection,
cursor, scroll. Task views own their transient state privately.

### C4. The membership/presentation boundary

**Query decides *which* assets. Arrangement decides *order and sectioning*. View mode decides
*rendering*.** Arrangement can never add or remove an asset. This invariant is what keeps
"Save as Smart Collection" honest and group-by a safe toggle.

### C5. Command targeting rule

Verbs act on the **selection if non-empty, else the cursor**. Batch-flavored operations (export,
bulk metadata) always display their target explicitly ("Export 12 selected" / "all 412 results").

---

## Seam rules

### C6. The query AST is the spine

One versioned, **typed-struct** grammar (never stringly key-value maps): filter bar renders it,
smart collections persist it (JSON, with a `version` int from day one), NL search parses *into*
it, the backend compiles it to SQL. Defined once in Go, generated to TS. New capability = new
token type in the registry, not a new UI or a new seam method. Pattern: *interpreter* (GoF).
Full design: `seam/01-queries-and-commands.md`.

### C7. Seam rule: new method ⇢ new result shape, never new predicate

`QueryAssets(query, arrangement, page)` absorbs every predicate over assets. A new seam method is
justified only by a new **result shape** (sidebar tree + counts, duplicate cluster pairs, …).
**Smell: a method name containing a predicate** (`GetSharpAssets`, `GetAssetsByTagSortedByDate`)
— that's an AST node trying to escape. The nightmare scenario this prevents: 200 narrow methods
each supporting some permutation of filters. Also codified as coding-guidelines §10.

Writes mirror it: `UpdateAssets(ids, patch)` with a closed optional-field patch struct (the seam
face of the backend's TriagePatch). Undo (command pattern) lives above it, capturing per-asset
before-state.

### C8. Events: one envelope, a catalog, no ad-hoc emits

All backend→frontend events use one envelope shape (`{topic, type, payload, timestamp}`) over a
small set of named topics (`jobs`, `watcher`, `sync`, `catalog`). Every event type is declared in
one constants catalog per side. An `EventsEmit` with a string not in the catalog is a bug.
Pattern: *pub/sub with typed envelopes* (domain events; Redux actions are the same discipline).

Events are for genuinely async facts only (jobs, watcher, sync). Request/response stays
synchronous typed calls — **no event-driven architecture as insurance**; it trades visible
compile-time coupling for invisible runtime choreography.

### C9. Jobs: no private progress paths

All background work (import, enrichment, backup, export, integrity, RAW dispatch) reports through
the one Job envelope — see `seam/02-events-jobs-and-binary.md` for the reconciled shape. The
status bar and activity drawer render jobs *generically*; a new kind of background work is a new
`kind` string, zero new UI. A feature building its own progress plumbing must justify itself
against this rule in writing.

### C13. Go domain models are the single source of truth

TS model types are *generated* (Wails bindings / tygo), never hand-maintained in parallel.
`frontend/src/models/` retires when the bindings land.

---

## Code discipline (both sides)

### C10. Registry rules

Registries are the dispatch mechanism of the app (type→presentation, actions, tokens, external
programs, backend `assettype` handler table). Rules:

- **Two-call-site rule:** a registry earns existence when ≥2 call sites dispatch on the same key.
  Don't pre-build.
- **Nil capability = one fallback path**, decided once at the dispatch point ("no thumb handler →
  generic type-badge card" lives in exactly one place). Scattered capability-conditionals
  (`if isRAW && hasPreview && !isVideo`) are the LrC-rot pattern; this rule is the antidote.
- **Completeness is enforced**, per side:
  - TS: `satisfies Record<Key, Entry>` on the registry literal + `never`-checked exhaustive
    switches. Incomplete extension = compile error.
  - Go: entries are structs with documented required fields; a `MustValidate()` sweep panics at
    startup wiring *and* runs as a table-driven test, so incompleteness fails the suite and first
    boot, never a user session. `exhaustive` linter on enum switches.
  - Shared Go helper: a small generic `registry.Table[K, V]` (Register panics on duplicate, Get,
    MustValidate with per-registry validators) so enforcement comes free with each new registry.
    Ceiling: if a registry's shape fights the helper, the *pattern + enforcement recipe* is the
    constant, not the shared type.

### C14. All display text is data

Every user-facing string is an i18n key; enums map through display registries; dates/numbers/
sizes through `Intl`. Consequence: terminology stays renameable forever at near-zero cost —
which is why C1 locks *concepts*, not final UI copy.
