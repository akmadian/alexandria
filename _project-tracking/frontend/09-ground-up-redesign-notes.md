# Frontend Architecture — the Ground-Up Redesign Round

**Status:** LOCKED (Ari + Claude, 2026-07-08; round complete, reconciled into the doc set the
same day). This is the **standing frontend architecture record**: state planes, the store, seam
integration, module structure, fetch/perf/retry policy, the token/AST frontend contract, and
optimistic-mutation discipline. Where this file conflicts with 01–08, this file wins for
architecture; CONSTANTS.md (C1–C14) and the seam docs bind everything. Coding standards derived
from this round live in `frontend/CLAUDE.md` (repo side).

**Founding decision:** nothing in `frontend/src/` is sacred — the existing code predates the
2026-07-07 design round and its UI system is not carried forward. Everything is re-derived from
the functional requirements, CONSTANTS, the seam design, and `08-design-language.md`. This
supersedes the EVOLVE verdicts in `07-code-disposition.md` (amended accordingly); *patterns*
called out below are re-derived on their merits, not preserved code.

## Ratified decisions

### State: three planes

1. **Catalog view state (the C2 equation)** — one store, outside React, **Zustand**-mounted
   (the `useSyncExternalStore` external-store pattern; Zustand is that pattern packaged). All
   mutations go through one **reducer-style `dispatch(action)`** — a single pure transition
   function; the action vocabulary is the app's real internal API. Components never write inline
   selectors: the store module exports curated selector hooks (`useCursor()`, `useIsSelected(id)`,
   …) written correctly once; ESLint `no-restricted-imports` keeps store internals private.
2. **Server state** — TanStack Query exclusively (TkDodo's server/client-state separation).
   Long `staleTime` + **event-driven invalidation** (we own the freshness signal; the engine
   pushes C8 events). `refetchOnWindowFocus`/`refetchOnReconnect` off. **Hard coding standard:
   the store never holds borrowed server state.** Settings (catalog settings, machine.json,
   keybindings.json) are server state — fetched/mutated over the seam.
3. **UI-chrome prefs** — localStorage, *only* for what must apply pre-paint and has no engine
   consumer: theme, pane widths, density, current view mode. Not "settings" in the engine's
   sense; anything the engine's settings service manages is plane 2.

### The store (drilled 2026-07-08)

```ts
interface CatalogViewState {
  scope: Scope;                        // sidebar-owned; separate from filter per C1
  filter: WhereNode | null;            // ONE representation: the boolean tree; pills render
                                       // the top-level AND's children; nested groups render
                                       // as a group pill opening the advanced editor
  arrangement: { sortField; sortDir; groupBy: GroupKey | null };
  selection: { kind: "ids"; ids: ReadonlySet<AssetID> }
           | { kind: "all"; except: ReadonlySet<AssetID> };
  cursor: { id: AssetID; index: number } | null;
  viewMode: "grid" | "loupe" | "compare" | "cull";   // only persisted field (partialize)
}
```

- Canonical query + serialized query key are **memoized derivations** (assembler in
  query-model), never stored — pills and cache key cannot disagree.
- **Selection and cursor are id-anchored; indices are derived hints** — recomputed on order
  change, never authoritative. Indices are coordinates that drift when the catalog changes
  underneath (imports, watcher); ids are identity. Ranges are a *gesture, not a storage format*:
  shift-click materializes ids via the ids-slice seam call (`range-committed(ids)` action), then
  selection is pure identity.
- **Guardrail:** selection only ever contains ids we actually hold. Endpoints of a range are
  rendered cells (clickable ⇒ loaded); the interior is materialized by the slice call.
- **Invariants (in the reducer):** scope/filter change ⇒ selection cleared, cursor kept if its
  asset is still in the new working set (LrC behavior; membership + new index via `IndexOfAsset`,
  else reseed to 0). Arrangement change ⇒ **selection kept** (C4: membership unmoved), cursor
  index remapped. Cursor exists iff working set non-empty (`working-set-changed(total)` echo
  action seeds/clears it). Compare mode requires 2–4 selected (registry context predicate
  upstream, reducer as backstop).
- Action vocabulary families: scope/filter (scope-set, pill-added/edited/removed,
  filter-replaced, filter-cleared) · arrangement (sort-set, group-by-set) · selection
  (asset-clicked(id, index, modifiers) — the reducer owns the modifier grammar,
  range-committed(ids), all-selected, selection-cleared) · cursor (cursor-set, cursor-stepped) ·
  view-mode-set · data echoes (working-set-changed(total)).

### Seam integration

- `AlexandriaAPI` contract interface, Wails adapter + mock, all I/O confined to `api/`;
  `ApiError` normalization at the adapter. Types generated from Go (C13) into
  `_generated-types/`.
- **Event pump**: one subscriber for the C8 envelope routing to three sinks — catalog topic →
  TanStack invalidation (one mapping function, `CatalogChange` payload → query-key prefixes);
  jobs topic → jobs store (envelopes kept whole, `message` feeds the activity drawer); watcher/
  sync → connectivity + toasts — plus a bounded ring buffer (~500 envelopes) for the dev corner.
- Bytes never cross IPC; thumbnails via content-addressed immutable URLs (webview HTTP cache
  does the caching; zero JS cache).

### Fetching and performance

- **Grid = AG-Grid-style infinite row model**: fixed-size blocks keyed by
  `(query+arrangement, blockIndex)` via `useQueries`; fetch only viewport+buffer blocks;
  debounce during fling; LRU cap. `total` sizes the scrollbar before any block lands (random
  access, no linear `useInfiniteQuery`). Arrangement is in the key because a block is a window
  into an *ordered* result (client-side sorting is structurally impossible — we never hold the
  set). Grey still-loading cells (LrC convention) for now.
- **Virtualization**: bespoke grid + filmstrip on **tanstack-virtual** (bare positioning
  engine). RAC's Virtualizer is coupled to RAC collections/selection — wrong tool for a grid
  whose selection the store owns.
- **Retry policy** (local IPC, not a network): reads default `retry: false`; per-code opt-in
  (busy database, thumbnail 404 during regeneration) 1–2 retries, short capped backoff; every
  consumer renders an explicit error state with manual retry — nothing spins, everything
  degrades to a rendered state. **Mutations**: idempotent-by-construction (patches carry
  absolute values, never deltas — a coding standard) ⇒ 1–2 quiet auto-retries before visible
  rollback + toast; non-idempotent ops (deleteFromDisk) never auto-retry.
- **Error boundaries**: app-level crash screen (copy details, export logs) + per-pane
  boundaries (browser / main region / inspector fail independently, reload via key bump).
  Boundaries catch render errors only; async errors route through the ApiError switch.

### Module structure

bulletproof-react-style feature structure, kebab-case, **ESLint-enforced import boundaries**
(shared → features → app; features never import features):

```
src/
  _generated-types/   Go-generated models + AST types; never hand-edited
  api/                the seam: contract, wails adapter, mock, event pump, ApiError
  stores/             catalog view-state store + jobs store; curated selector hooks
  query-model/        pure AST domain: builders, assembler, serializer, parser,
                      narration, TOKEN REGISTRY (zero I/O, zero React)
  actions/            input system: action registry, context dispatch, keybindings
  asset-types/        per-type presentation registry
  components/         shared domain-blind primitives (RAC-based chrome)
  hooks/              useMotion, useRafLoop, …
  styles/             tokens, themes, motion primitives
  i18n/
  features/           browser, filter-bar, grid, loupe, cull, compare, inspector,
                      status-bar, palette, review, import, settings, home
  app/                shell, task-view host, providers/, boot
```

- Registries live in their **domain** modules (no `registries/` mechanism-dir).
- `query-model/` named to avoid the TanStack "Query" collision; it is pure functions.
- Data flow: `ui → [store ⊕ tanstack(api → backend)]`, joined in feature hooks. Store never
  calls the API; API holds no state; TanStack config lives in `app/providers/` only.

### UI toolkit

- **React Aria Components for chrome** (menus, dialogs, trees, popovers, palette, forms) —
  decided in a parallel session. Content surfaces (grid, loupe, cull, compare, filmstrip) are
  bespoke.
- **Motion**: library-free, CSS-first (transform/opacity only — compositor rule); rAF via a
  shared `useRafLoop` hook (auto-cancel on unmount) for per-frame content mutation (mono
  ticker, dither/dissolve); one `useMotion()` gate for `prefers-reduced-motion`. Sanctioned
  moments are dedicated components, not a general system.

### Task views

Catalog stays **mounted but hidden** under task views (C3 restoration free; virtualized DOM is
tiny). Freshness on return comes from event invalidation — no bespoke return hook. Keyboard
context switching per the action registry's context predicates.

## Seam requirements discovered by this round (feed the backend query round BEFORE it locks)

1. **`AssetIDSlice(query, arrangement, fromIndex, toIndex) → []id`** — ids-only window; powers
   range selection materialization. Trivial SELECT (id column only) over the compiled ordering.
2. **`IndexOfAsset(query, arrangement, id) → index | null`** — powers cursor keep-if-present
   across query changes and cursor index remap across arrangement changes.
3. **`UpdateAssets` target gains `exceptIds`** — target = `{ids} | {scope, where, exceptIds}`;
   backend applies as ONE SQL statement (`… WHERE <compiled query> AND id NOT IN (…)`), never an
   id-materialized loop.
4. **ORDER BY must always append a unique tiebreaker (`…, id`)** — deterministic total order is
   what makes index ranges/slices meaningful across calls.
5. **Distinct-values lookup for suggestable fields** (camera make/model, …) — powers parser/
   editor suggestions. (Exact shape TBD in the token-registry drill-in.)
6. **Bulk-undo acceptance test**: triage patch on 300k assets, undone, redone — no perceptible
   stall (single-statement apply; batched-transaction restore).

## Undo design notes (backend-owned; frontend surface is undo()/redo() + HistoryState + events)

- **Command pattern, strict LIFO** — no cherry-picking from the middle (before-images are only
  valid at the top of the stack). SQL is not invertible from its own text; stack entries hold
  **data**: per-asset **before-images** for value writes (bulk SELECT of touched fields before
  the update; restore is one batched transaction), or **structural inverses** for membership ops
  (add↔remove, soft-delete↔restore) which are nearly free. Two entry flavors, one interface.
- **Byte budget** needed alongside the depth-50 cap (fifty 300k-row before-images ≈ hundreds of
  MB) — evict oldest when over.
- **Undo vs external writes** (XMP sync / watcher writing between do and undo — undo restores
  the before-image over the top): needs an explicit decision in the backend undo design, not an
  accident. Flagged, unresolved.
- Frontend: an undo is just another mutation — events invalidate, blocks refetch, no special
  path. Optimistic updates for `all`-shaped targets invalidate rather than patch rows.

## Token & AST drill-in (locked 2026-07-08)

### The triad (locked vocabulary, extends C1)

**Token** = a *definition*: one filterable dimension with its operators, value kind, parsing and
display rules — a registry entry. **Leaf** = an *instance*: one predicate
`{field, cmp, value}` — an AST node. **Pill** = the *rendering* of one leaf (C1). Tokens are the
vocabulary, leaves are sentences, pills are typography; the registry is the dictionary, keyed by
`field`. Frontend registry owns parse/render; backend registry owns SQL compile; the **field-name
list is the shared spine** (defined in Go, generated to TS).

### Value kinds — the collapse that keeps tokens cheap

Tokens cluster into ~7 **value kinds** — `enum`, `numeric`, `date-range`, `text`, `tag-ref`,
`entity-ref`, `free-text` — and the kind, not the token, owns the expensive machinery:
**editors are per-kind** (7 total; tokens pass config), the **pill is one generic component**
(renders from the token's formatted output; tokens contribute data + pure functions, never
components), and parsing splits the same way (parser core = structure; kind = value grammar,
e.g. ONE date grammar shared by captured/added; token = vocabulary/aliases). New token = pick a
kind, fill in a row — that's the C6 extension flow made concrete.

### v1 vocabulary

`type` (enum) · `rating` (numeric 0–5) · `label` (enum) · `flag` (enum) · `tag` (tag-ref; has/
under) · `captured`/`added` (date-range) · `source` (entity-ref) · `filename` (text) ·
`width`/`height` (numeric) · `camera` make/model (suggestable text) · `status` (enum) ·
per-metadata-field text tokens (title, caption, creator, copyright, lens, … — **each its own
registry row**, all `kind: text`; a generic "metadata" token smuggling a field selector in the
value would break field-keyed dispatch) · `text` (free-text FTS; unresolved parser words land
here). **Absence = the `empty` operator** on base tokens with parser aliases ("unrated" →
`rating empty`), never separate absence tokens. Signals (`sharpness`, …) arrive later as numeric
rows. Registry entries carry a **`category`** tag (`triage`/`capture`/`file`/`metadata`/…) for
picker + editor grouping.

### AST wire shapes (design target; generated from Go, C13)

```ts
type Query     = { version: 1; scope: Scope; where: WhereNode | null };
type Scope     = { kind: "library" } | { kind: "collection"; id } |
                 { kind: "folder"; sourceId; path; recursive? } | { kind: "tag"; id };
type WhereNode = GroupNode | Leaf;
type GroupNode = { op: "and" | "or" | "not"; children: WhereNode[] };
type Leaf      = { field: TokenField; cmp: TokenOperator; value: unknown };
```

**Wire typing (locked):** `field` and `cmp` are **generated string-literal unions** sourced from
Go (`TokenField`, `TokenOperator`) — no bare string constants in code, no TS `enum` keyword
(literal unions + `as const` maps are the idiom). Value shapes stay loose at the wire;
**constructors are strict** (`tokens.rating.leaf("gte", 3)` — wrong operator/value = compile
error) and one **validation gate** sits where persisted trees enter (loading a smart collection:
`validate()` per leaf; failures render as an inert "unknown token" pill, never a crash or a
dropped predicate). Full per-token discriminated unions rejected: a truly strict leaf is a
three-way dependent type that fights every dynamic constructor, and tree-walking code treats
leaves uniformly anyway. `satisfies Record<TokenField, Token>` gives registry completeness at
compile time (C10).

### Negation (Ari ruling 2026-07-08)

Negation exists at BOTH levels, deliberately: **negated operators** on tokens (`neq`,
`notEmpty`, `lacks`/`not-under` for tag, …) for the value-level "rating ≠ 3" case — a leaf
concern, maps directly to SQL — AND **`not` as a tree op** for logical composition over groups.
The dual-representation hazard (NOT(x=3) vs x≠3 producing different cache keys / saved queries)
is closed by **assembler normalization**: `not` wrapping a single leaf with a negatable operator
canonicalizes to the negated operator; `not` survives only over groups and non-negatable leaves.
One canonical form per meaning; the serializer sees only normalized trees.

### Registry entry contract

```ts
interface Token<Kind extends ValueKind> {
  field: TokenField;                      // generated from Go — the shared spine
  kind: Kind;                             // picks editor, value grammar, operators
  category: TokenCategory;                // picker/editor grouping
  operators: readonly TokenOperator[];
  kindConfig: KindConfig[Kind];           // enum members / bounds / suggestion source
  aliases: readonly string[];             // parser vocabulary ("stars", "unrated", …)
  parseValue(raw: string, vocabulary: Vocabulary): Value<Kind> | null;
  leaf(cmp: TokenOperator, value: Value<Kind>): Leaf;   // strict constructor
  validate(leaf: Leaf): boolean;                        // persistence-boundary gate
  formatValue(value: Value<Kind>, i18n): string;        // pill + narration share it
  narrationKey: string;                                 // i18n template per operator
}
```

`Vocabulary` (user's tag/source/camera names for parsing + suggestions) is **passed in, never
fetched** — the registry stays pure; feature hooks supply TanStack-cached vocabulary (the
distinct-values seam method).

### Dates: anchor + signed duration, half-open (Ari shape, refined)

Every date expression normalizes to `{ anchor: ISODate | "now", duration: signedDuration }`,
interpreted as the half-open interval `[min(a, a+d), max(a, a+d))` (half-open kills the Dec-31
fencepost). "2025" → `{2025-01-01, +1y}`; "2020-2025" → `{2020-01-01, +6y}`; "last 7 days" →
`{"now", -7d}`. **Rolling-ness = `anchor: "now"`**, resolved by the backend at compile time —
one shape covers frozen and rolling; a saved "last 7 days" smart collection re-evaluates on
every open (LrC behavior). `in` is the only containment operator (the range IS the value;
`between` dissolves); `before`/`after` compare against the anchor point. Backend compile note:
calendar-unit durations and timezone semantics for capture dates are query-round concerns —
flagged there.

### AST versioning policy

Catalog schema version and AST grammar version are **distinct counters, coupled in one
direction**: an AST version bump always ships inside a catalog migration (the run-once
rewrite hook for stored smart collections — even if no schema change rides along); catalog
migrations mostly don't touch the AST. Forward-only v(n)→v(n+1) upgraders at migration time,
after the automatic backup; **downgrade transformers never exist** (the P0 schema gate stops old
code opening newer catalogs). Frontend and backend cannot skew (one binary; TS is generated from
the Go it ships with) — so the **frontend never migrates ASTs**. Vocabulary evolution without
version bumps: token added = old queries unaffected; field renamed = mechanical migration
rewrite; field REMOVED = leave the leaf, render unknown-token pill, surface the affected smart
collection for user repair — never silently drop a predicate (D20-grade trust rule).

## Optimistic mutation × undo (locked 2026-07-08)

The problem: keystroke-speed feedback (Photo Mechanic benchmark) on a confirmation loop
(write → event → invalidate → refetch → render) inherently slower than a keystroke. The bet:
these writes almost always succeed, so patch the TanStack cache immediately and reconcile
behind. The design question: who wins when server snapshots and optimistic predictions coexist —
such that values never flicker backwards and undo always targets what the user thinks.
(Canonical pattern; TkDodo "Concurrent Optimistic Updates" / "Mastering Mutations" + the
TanStack optimistic-updates guide — copy it, don't invent.)

1. **Cancel-on-mutate + invalidation gate.** When a catalog-editing mutation fires: cancel
   in-flight refetches for the queries it patches (a stale response must not land on the
   patch); while ANY catalog-editing mutation is in flight (tracked by mutation key /
   `isMutating`), invalidations mark-stale only — the refetch runs when the in-flight count
   hits zero, so the snapshot that replaces optimistic state is at least as new as it.
   Kills the 3-then-4 flicker race with our own events.
2. **One ordered lane** for ALL catalog-editing calls — mutations, undo, redo — a small FIFO
   in the seam layer (next dispatches when previous settles). Undo deterministically lands
   after the command it follows; no wrong-target undo. IPC is fast; the lane costs nothing
   perceptible.
3. **Undo/redo render pessimistically** (event → invalidate → refetch): the before-image for
   bulk ops lives backend-side, undo is deliberate not hot-path, and it still lands in tens of
   ms. `HistoryState` events drive menu label/tooltip independently.
4. **Optimism scope:** ids-targeted triage patches + collection membership = optimistic cache
   patch (prior row values saved for rollback). `all`-shaped targets = invalidate + refetch
   visible blocks (can't enumerate; already decided). Destructive ops (deleteFromDisk) never
   optimistic (they sit behind confirmation modals anyway). Failure after quiet retries:
   revert patched rows + ApiError-mapped toast — loud, never silent.
5. **No cross-seam coalescing:** one keystroke = one command = one undo step (predictable
   Cmd+Z). The invalidation gate absorbs bursts; coalescing is an optimization to add only if
   command volume ever measurably hurts.

## Round close-out (2026-07-08)

Write-up complete: this doc is the architecture authority; coding standards written into
`frontend/CLAUDE.md`; dated amendments applied to `00`, `02`, `03`, `07`; the new seam-method
requirements appended to `../seam/01-queries-and-commands.md` §Additions (and flagged in
`../backend/04-open-questions.md` #4); master head updated. Implementation still gates on the
backend query round + seam round — sequencing unchanged.

Pending confirmation from the parallel framework-selection session: virtual-grid library
(tanstack-virtual recommended here; RAC Virtualizer ruled out for content surfaces) and
command-palette approach (RAC-based recommended).
