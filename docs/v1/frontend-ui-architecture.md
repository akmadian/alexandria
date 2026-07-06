# Frontend UI Architecture

Internal structure of the frontend: code layout, components, styling, state, and the system pieces (i18n, logging, testing, linting, error boundaries). Companion to `frontend-architecture.md`, which owns the API seam, caching, and call patterns — nothing here changes that document. `src/api/` and `src/models/` are settled and treated as given.

This document supersedes the Untitled UI starter kit. Its component library, Tailwind, and icon packs go. What replaces it is a small set of owned components built on **react-aria-components** (kept — unstyled behavior + accessibility, no styling opinions) and native platform features.

Desktop only. No mobile, no tablet, no breakpoints. That single constraint deletes most of what layout frameworks exist to solve.

---

## 1. What leaves, what stays

The starter kit ships ~200 component files (payment icons, marketing headers, carousels, QR codes) of which Alexandria uses a handful of buttons and inputs. The five real components (`components/library/*`) are ours and small; everything else is cargo.

**Removed** (directories and dependencies together):

| Dependency | Replaced by |
|---|---|
| `components/base|application|marketing|foundations|shared-assets` | ~10 owned primitives (§6) |
| `tailwindcss` + plugins, `tailwind-merge`, `tailwindcss-animate` | CSS Modules + design tokens (§7) |
| `react-aria` (the standalone hooks package) | `react-aria-components` alone covers our use |
| `@untitledui/icons`, `@untitledui/file-icons` | `lucide-react` |
| `react-router` | nothing — one window, view modes, no URLs (§3) |
| `react-hotkeys-hook` | own dispatcher over UI-defined, user-remappable actions (§9) |
| `motion`, `embla-carousel`, `recharts`, `input-otp`, `qr-code-styling` | nothing; CSS transitions cover our animation needs |

**Kept:** `react`, `react-dom`, `@tanstack/react-query`, `react-aria-components`, `typescript`, `vite`, `prettier`.

**Added:**

| Dependency | Why it clears the "not a few lines" bar |
|---|---|
| `@tanstack/react-virtual` | grid windowing over 500k rows; same family as Query |
| `i18next` + `react-i18next` | plurals, interpolation, fallback, language switching — the classic hand-roll trap |
| `lucide-react` | ~40 line-style icons, tree-shakeable, fits the aesthetic |
| dev: `vitest`, `@vitest/coverage-v8`, `@testing-library/react`, `@testing-library/user-event`, `happy-dom`, `eslint` + `typescript-eslint` + `eslint-plugin-react-hooks` + `eslint-plugin-jsx-a11y` | §13, §14 |

Eight runtime dependencies total, down from ~20. Anything not in this table needs a reason this table can't give.

---

## 2. Source layout: feature folders, thin shared core

Mirrors the Go rule (`coding-guidelines.md` §0): organize by concern, not by technical kind. No `utils/`, no `helpers/`. (`src/models/` predates the rule and is settled data-layer — it stays.)

```
src/
├── api/            # settled: contract, mock, queries (the seam doc owns this)
├── models/         # settled: domain shapes
├── app/            # composition root: App, shell layout, providers, error boundaries
├── components/     # owned primitives, reused across features (§6)
│   ├── button/     #   one folder per component: button.tsx + button.module.css
│   ├── input-field/
│   ├── modal/
│   ├── tag-chip/
│   ├── tree/       #   the flagship reusable — folders, collections, tags all use it
│   ├── toast/
│   └── …
├── features/       # domain views — each owns its components, hooks, styles
│   ├── browser/    #   left pane: tree selector (sources|collections|tags), BrowserView
│   ├── grid/       #   GridView, AssetCard, virtualization, selection interaction
│   ├── loupe/      #   full-size render of selected asset
│   ├── inspector/  #   right pane: metadata display + editing
│   ├── filter-bar/ #   toolbar: search, type, rating, sort, density
│   ├── jobs/       #   import progress, status bar chrome
│   └── settings/   #   settings + keybindings modal
├── lib/            # cross-cutting singletons, each a named concern, one file each:
│   ├── logger.ts   #   §11
│   ├── keys.ts     #   keybinding dispatcher §9
│   ├── format.ts   #   Intl-based date/size/duration formatters §10
│   └── enum-display.ts  # enum code → icon/label key, with unknown-value fallback
├── i18n/           # init + locales/{en,…}.json
└── styles/         # tokens.css, themes/, global.css (§7)
```

Rules:

- **Features may import `components/`, `lib/`, `api/`; never each other.** Cross-feature coordination happens through state owned by `app/` (§4). If two features need the same component, it moves to `components/`.
- **Components import nothing above themselves** — no `api/`, no feature code. A primitive that fetches is a feature in disguise.
- A component folder is `name.tsx` + `name.module.css` + (when non-trivial) `name.test.tsx`. No barrels, no `index.ts` re-export layers — import the file.

---

## 3. Page structure: one shell, no router

Alexandria is one window. "Navigation" is *what the main region shows*, which is a function of two pieces of state — the browse target and the view mode — not a URL. A router would add route definitions, layout nesting, and 404 handling to an app with no addresses. Dropped.

Adding one later is cheap *because* of §4: everything a URL would encode (target, view mode) already lives in one place, LibraryProvider. Retrofitting a router is "serialize that state to a path and back" — a contained change in `app/`, touching no feature code. The design keeps the escape hatch by never letting navigation state scatter.

```
┌─────────────────────────────────────────────────────┐
│ FilterBar (title, search, filters, sort, density)   │
├──────────┬───────────────────────────┬──────────────┤
│ Browser  │  GridView ⇄ LoupeView     │  Inspector   │
│ (Tree +  │  (main region, swaps on   │  (selection  │
│ selector)│   view mode)              │   details)   │
├──────────┴───────────────────────────┴──────────────┤
│ StatusBar (job progress, source status, counts)     │
└─────────────────────────────────────────────────────┘
```

The shell is one CSS Grid in `app/shell.module.css`:

```css
.shell {
  display: grid;
  grid-template:
    "filterbar filterbar filterbar" auto
    "browser   main      inspector" 1fr
    "status    status    status"    auto
    / var(--pane-browser) minmax(0, 1fr) var(--pane-inspector);
  height: 100dvh;
}
```

- **Pane resizing:** a 4px drag handle per pane writes `--pane-browser` / `--pane-inspector` on the shell element; widths persist to `localStorage`. ~30 lines of pointer-event code, no library.
- **Pane collapse** (hide inspector, hide browser) sets the variable to 0 — the grid does the rest.
- Modals (`Settings`, import summary, delete confirm) overlay the shell via `<dialog>` (§6); they are not "pages".
- Full-screen loupe is `viewMode: "loupe"` occupying the main region; a later immersive mode can overlay the whole shell without structural change.

**Responsiveness/scaling without a framework:** desktop-only means the shell never reflows — panes resize, the main region absorbs the rest via `minmax(0, 1fr)`. Inside the main region, the asset grid is `repeat(auto-fill, minmax(var(--tile-size), 1fr))`; the density control just changes `--tile-size`. All spacing/type tokens are `rem`-based, so global UI scaling (a "UI scale" setting later) is one `font-size` on `:root`. That is the entire scaling system.

---

## 4. State model

Three kinds of state, three owners. Nothing else. No Redux, no Zustand, no atoms — the seam doc already put server state in TanStack Query, and what remains is small.

| Kind | Examples | Owner |
|---|---|---|
| **Server state** | assets, trees, settings, job events | TanStack Query via `api/queries.ts` hooks — components never touch `api` directly (settled, seam doc §7) |
| **Session view state** | browse target (scope), filter bar values, sort, view mode, **selection**, loupe index | `LibraryProvider` — one context in `app/` (below) |
| **Machine-local prefs** | theme, pane widths, density, language override | `localStorage`, read/written by the owning component; not backend Settings (those are catalog-scoped) |

### LibraryProvider

The one piece of shared client state, defined in `app/library-state.tsx`:

```ts
interface LibraryState {
  target: BrowseTarget;          // what the Browser selected → becomes ListQuery scope+filter
  filters: FilterBarState;       // search text, type, min rating, sort, density
  viewMode: "grid" | "loupe";
  selection: Set<string>;        // asset ids
  anchorId: string | null;       // range-select anchor
}
```

Implemented with `useReducer` + context — actions like `selectTarget`, `setFilter`, `select({id, additive, range})`, `enterLoupe`. A reducer (not scattered `useState`) because selection semantics (click/ctrl/shift/select-all, clear-on-target-change) are one cohesive state machine and the keyboard dispatcher (§9) needs to dispatch the same actions the mouse does.

Why context and not props: selection is read by GridView, Inspector, FilterBar (count badge), LoupeView, and the keyboard layer — five consumers across three panes. That's past the prop-drilling threshold. Everything below these five consumers still takes plain props.

Split into two contexts (`state`, `dispatch`) so dispatch-only consumers don't re-render on every selection change. That is the entire performance strategy for state; anything fancier waits for a measured problem.

**Derived, never stored:** the `ListQuery` is computed with `useMemo` from `target + filters` (as `library.tsx` does today). The selected asset's full record is `useAsset(lastSelectedId)`. If it can be derived, it is not in the reducer.

---

## 5. Data flow: who fetches what, when

Components consume hooks from `api/queries.ts` exclusively. The complete map:

| Consumer | Hook(s) | Fetch timing | Notes |
|---|---|---|---|
| `BrowserView` | `useSources`, `useCollections`, `useTags` | mount (app start); refreshed only by `catalog:changed` | reference data, `staleTime: Infinity` |
| `BrowserView` (per source) | `useFolderTree(sourceId)` | first expansion of that source | |
| `GridView` | `useAssets(query)` | query change + scroll windowing | `placeholderData` keeps the old page during transitions — no flash |
| `FilterBar` | reads `total` from the same `useAssets` result via LibraryProvider-adjacent prop | — | fires no queries itself |
| `InspectorView` | `useAsset(id)`, `useTags` (for tag editing), `usePatchAssets`, `useSetAssetTags` | on selection change | multi-select shows shared-field editing per seam doc §15.2 when designed |
| `LoupeView` | `useAsset(id)` + `previewURL` | on entering loupe / navigating | prev/next order comes from the grid's current row window |
| `StatusBar` / jobs | `onJobProgress`/`onJobDone` subscription via a `useJobs()` hook in `features/jobs/` | push | ephemeral chrome state, never enters the query cache |
| `SettingsModal` | `useSettings`, `useKeybindings` | on open | |
| root | `useCatalogSync()` | once, in `app/` | the event→invalidate bridge (settled) |

**Marshaling at the render edge.** Data crosses the seam as JSON-safe codes and ISO strings and *stays that way* in the cache — no `Date` objects, no enriched view models, no normalization pass. Presentation conversion happens at render time through two small modules:

- `lib/format.ts` — memoized `Intl.DateTimeFormat` / `Intl.NumberFormat` instances keyed by current locale: `formatDate`, `formatBytes`, `formatDuration`. Native `Intl` covers all of it; no date library.
- `lib/enum-display.ts` — maps enum codes (`FileType`, `Flag`, `SourceStatus`, job kinds) to `{ icon, labelKey }`, with a mandatory fallback entry. This is the single place seam convention 6 (*tolerate unknown enum values*) is implemented; components never switch on enum codes themselves.

---

## 6. Components

Two tiers, matching the notes: **primitives** (`components/`) that know nothing about the domain, and **feature components** (`features/*/`) that compose primitives with hooks.

### Primitives

Foundation rule: **react-aria-components (RAC) for anything with non-trivial interaction semantics; bare native elements where the semantics are trivial.** RAC is unstyled — all appearance comes from our CSS Modules + tokens (§7), so it costs nothing aesthetically and buys focus management, keyboard patterns, and ARIA wiring maintained by someone else. The `Aria*` import-prefix convention from the old setup carries over.

| Component | Foundation | Notes |
|---|---|---|
| `Button` | RAC `Button` | variants: `primary`, `ghost`, `danger`; sizes `sm`/`md`; icon slots; consistent hover/press/focus states via render props |
| `InputField` | RAC `TextField` | label, error text, prefix icon; that's all |
| `Select` | RAC `Select`/`ListBox` | styled listbox with option icons comes free — the reason not to settle for native `<select>` |
| `Modal` | RAC `Modal` + `Dialog` | focus trap, `Esc`, overlay dismissal handled. Wrapper adds title bar, close button, size variants |
| `TagChip` | `<span>`/`<button>` | trivial semantics — native |
| `Toast` | own ~60-line store | module-level `toast(kind, msg, action?)` + a `<Toasts/>` outlet in the shell; error surfaces from seam doc §9 land here |
| `Tooltip` | RAC `Tooltip` | hover/focus timing and positioning handled |
| `ContextMenu` | RAC `Menu` + `Popover` | APG menu pattern for free |
| `Tree` | RAC `Tree` | see below |
| `Icon` | thin wrapper over lucide | fixes size/stroke to the design system so call sites can't drift |

### Tree — the flagship reusable

One component serves the three sidebar hierarchies (folders-in-sources, collections, tags) and any future one. Generic over the node payload:

```ts
interface TreeNode<T> {
  id: string;
  label: string;
  children?: TreeNode<T>[];
  data: T;                    // Source | FolderNode | Collection | Tag — Tree never looks inside
}

interface TreeProps<T> {
  nodes: TreeNode<T>[];
  selectedId: string | null;
  onSelect(node: TreeNode<T>): void;
  onExpand?(node: TreeNode<T>): void;   // lazy loading: folder trees fetch on first expand
  renderDecoration?(node: TreeNode<T>): ReactNode;  // count badge, status dot, color chip
}
```

- Wraps RAC's `Tree`/`TreeItem`, which supplies the full APG tree pattern — roving tabindex, arrow-key navigation, `aria-expanded`, typeahead — so our component is adapter + styling + decoration slots, not keyboard plumbing. This was the riskiest hand-rolled piece in the earlier draft; keeping RAC erases it.
- Expansion state lives inside Tree, persisted per tree-id to `localStorage`.
- Each feature adapts its domain to `TreeNode<T>` in a pure function (`features/browser/adapt.ts`): `foldersToNodes(FolderNode)`, `tagsToNodes(Tag[])`, etc. Tree stays domain-free.
- `BrowserView` = segmented selector (Sources | Collections | Tags) at top + the Tree for the active mode, per the reference image. Selecting a node dispatches `selectTarget` (§4).
- Drag-onto-node (assets → collection/tag) is a later additive: HTML5 DnD props on treeitems, no structural change.

### Feature components

`AssetCard` (in `features/grid/`) renders one `AssetRow`: thumbnail `<img loading="lazy">` with `thumbURL`, filename, rating/flag/label glyphs, selection ring, file-type badge via `enum-display`. Pure — takes the row and selection flags as props, emits pointer events upward. `React.memo`'d; it is the most-instantiated component in the app.

`GridView` owns virtualization: `useVirtualizer` over *rows* of N columns, where N derives from container width ÷ `--tile-size` (via a resize observer). It maps visible row range → page offsets → `useAssets` calls, per the seam doc's sparse-window design (§5 there). Scroll position resets on target change, survives filter tweaks.

`InspectorView` is read-mostly display plus the triage edit controls (rating, flag, label, note, tags) wired to `usePatchAssets`/`useSetAssetTags` — optimistic path already settled. Metadata sections (file, EXIF, location) are dumb subcomponents taking `Asset` slices.

---

## 7. Styling: CSS Modules + design tokens, no preprocessor

The "Sass-type thing" request, resolved down the ladder: modern CSS natively has **nesting**, custom properties, `color-mix()`, `@layer`, `:has()` — all supported by both target webviews (WKWebView, WebKitGTK). Vite compiles `*.module.css` with scoping and hashing out of the box, zero config. What Sass would still add is mixins/functions, which a token system makes largely unnecessary.

`ponytail:` no Sass; if a real mixin need appears, `bun add -D sass-embedded` and rename to `.module.scss` — Vite picks it up with no other change.

- **`styles/tokens.css`** — the design system as custom properties on `:root`, two layers:
  - *Primitives:* the grey ramp (`--grey-0…1000`), accent, spacing scale (`--space-1…8`, rem), type scale (`--text-xs…xl`), radii, borders, shadows, z-indices, durations.
  - *Semantic:* `--bg-surface`, `--bg-raised`, `--bg-sunken`, `--text-primary/secondary/tertiary`, `--border-default/strong`, `--accent`, `--danger`, `--focus-ring`… Components use **semantic tokens only**; primitives are reserved for theme files.
- **`styles/themes/*.css`** — each theme overrides semantic tokens under `[data-theme="…"]`. Ship three: **`graphite`** (neutral grey, the default — color-critical work demands a hue-free chrome), `dark`, `light`. A theme is ~40 variable assignments; adding a fourth is one file. Theme choice is `data-theme` on `<html>`, persisted to `localStorage`, applied before first paint by an inline script in `index.html` (no flash).
- **Component styles** are co-located `*.module.css`, using nesting and semantic tokens. No utility classes, no global class names outside `global.css` (reset, focus-visible ring, scrollbar styling, `@media (prefers-reduced-motion)` kill-switch).
- The retrofuturistic identity (type choices, corner treatments, line-work, the restrained accent) lives in the token values and a handful of decoration components — it is deliberately *not* an architectural concern. One place to iterate: `tokens.css`.

Color discipline for a DAM: chrome stays neutral in every theme; hue is reserved for user data (color labels, tag colors) and single-accent interactive states. This is a token-file policy, enforceable by review of one file.

---

## 8. Error boundaries

React error boundaries still require a class component; it's ~25 lines, written once in `app/error-boundary.tsx` — not worth the `react-error-boundary` dependency.

Placement — one outer, three regional:

- **App root:** full crash screen — restart hint, "copy error details", "export logs" (§11). Catches anything the regional ones don't.
- **Browser pane, main region, inspector pane:** a crash in one pane degrades to an inline "this panel failed — [reload panel]" card while the rest keeps working. A broken EXIF renderer must not take down the grid mid-triage. "Reload panel" just remounts via a `key` bump.

Every boundary catch goes through `lib/logger.ts` with component stack. Async/event errors don't hit boundaries — they surface through the seam-doc §9 category table (toast / inline / banner), also logged.

---

## 9. Keyboard system

Ownership: **keybindings are UI configuration; the backend is only their persistence.** The frontend defines the action vocabulary, default combos, contexts, and conflict rules; the DB just remembers the user's overrides so they survive reinstalls and roam with the catalog.

- `lib/keys.ts` is the definition site: an action registry (`rate_5`, `flag_pick`, `toggle_loupe`, …) with default combo + context (`global|grid|detail|import`) per action. The effective map = defaults ⊕ persisted overrides.
- Dispatch: one window-level `keydown` listener; normalizes the event to a combo string; resolves `(activeContext, combo) → action`; invokes the registered handler. Features register handlers on mount (`registerActions("grid", { rate_5: …, flag_pick: … })`); handlers call the same LibraryProvider dispatch / mutation hooks the mouse path uses.
- Active context derives from app state: modal open → swallow except `Esc`; loupe → `detail`; else `grid`. Text inputs opt out naturally (listener ignores events from editable elements except bindings marked global, e.g. `mod+z`).
- **Conflict detection moves frontend-side:** the keybindings settings UI checks a new combo against the in-memory effective map synchronously — no round-trip, no `ErrKeybindingConflict`.

Suggested contract slimming (the current surface predates this ownership split): `listKeybindings` / `setKeybinding` / `resetKeybindings` collapse to `getKeybindingOverrides()` and `saveKeybindingOverrides(overrides)` — a dumb persistence envelope. The backend stops knowing what an action or a conflict is.

**Command palette** (planned): the action registry makes it nearly free — a palette is "the same registry, searchable". RAC `Autocomplete` + `Menu` inside a `Modal`, bound to `mod+k`, listing actions filtered by availability in the current context, showing their current combos. Lives in `features/command-palette/`; adds zero new state concepts.

---

## 10. i18n

- `react-i18next`, initialized in `i18n/index.ts`; catalogs are flat JSON per locale in `i18n/locales/`. English ships; the mechanism is day-one so strings never accumulate as literals (retrofitting i18n is the expensive path).
- Keys are stable identifiers namespaced by feature (`inspector.rating.label`, `jobs.import.done`), matching seam convention 5: codes cross the seam, display text is frontend-owned, and `enum-display.ts` returns label *keys*, not strings.
- Plurals/interpolation via i18next's ICU-style handling; dates, numbers, and file sizes never go through catalogs — always `lib/format.ts` (`Intl`), which reacts to the active locale.
- Language selection: follow system locale, with an override stored in `localStorage`. (If it should roam with the catalog instead, `locale` becomes one more optional field on the backend `Settings` envelope — zero new bindings.)
- Lint guard: `jsx-a11y` + a review rule that bare string literals in JSX are a smell; `i18next` ESLint plugin can enforce later if drift appears.

---

## 11. Logging & observability

Requirement: a user having an issue can hand us a log file. Files are the backend's job (the webview can't write them); the frontend's job is to get its telemetry across the seam.

- `lib/logger.ts`: `log.debug|info|warn|error(msg, fields?)`. Dev → console. Always → an in-memory ring buffer (last ~2000 entries) and a batch queue flushed to the backend every ~5s, on `error`, and on `pagehide`. Entries carry timestamp, level, session id, app version.
- One addition to the contract (fits the conventions — an envelope, growing by fields not verbs): `logBatch(entries: LogEntry[]): Promise<void>`. The Go side merges frontend entries into its rotating structured log, so **one file tells the whole interleaved story** — backend job failing while frontend shows a stuck progress bar is one timeline.
- Global capture: `window.onerror`, `unhandledrejection`, error-boundary catches, and every `ApiError` surfaced at toast/banner level — all through the logger automatically. Triage-speed paths log nothing per keypress.
- The support surface is a Help → "Export logs" action: backend zips its log directory (which now includes frontend entries) to a user-chosen location. The "upload" in the notes is this export handed to a bug report — no telemetry service, nothing leaves the machine on its own.

`ponytail:` no metrics/tracing system. The observability need is post-hoc debugging of a local app; structured logs with session ids cover it. If perf work later needs numbers, `performance.mark` + logging the measures is the upgrade path.

---

## 12. Accessibility

The floor that never gets simplified away:

- RAC supplies the hard ARIA patterns (Tree, Menu, Dialog, Select); native elements cover the trivial rest. Nothing hand-implements a WAI-ARIA pattern.
- The virtualized grid is the one custom interaction surface: `role="grid"`-lite semantics (`aria-multiselectable`, `aria-selected`) on our own windowed markup, since selection and paging are bespoke. (RAC `Virtualizer` evaluated 2026-07 against its docs: it requires the full collection up front and supports only sequential loading — no random-access sparse pagination, no scrollbar-jump into a 500k result set, and no Tree support. Wrong shape for our windowed model; `@tanstack/react-virtual` stands.)
- Visible `:focus-visible` ring on everything interactive (global.css, token-driven, all themes).
- All three themes hold WCAG AA contrast for text tokens — checked once per theme file, at token level, not per component.
- `prefers-reduced-motion` disables non-essential transitions globally.
- `eslint-plugin-jsx-a11y` in CI keeps the basics honest.

---

## 13. Testing

Vitest (Vite-native, zero duplicate config) with two projects in one `vitest.config.ts`:

- **unit** (node env): `lib/` modules, reducers, adapters (`adapt.ts`), `enum-display`, format functions. The existing `mock-api.check.ts` migrates into a proper `mock-api.test.ts` here.
- **component** (happy-dom): Testing Library rendering against **`createMockApi()`** — the mock seam is the test harness, which is exactly why it exists. A ~20-line `renderWithApp(ui)` helper mounts QueryClient + LibraryProvider + i18n. Tests assert behavior ("clicking a collection node updates the grid title and count"), not markup.

Coverage: `@vitest/coverage-v8`, reported always. Thresholds start as *ratchet, not gate*: record the number, fail CI only on regression once the refresh stabilizes. What must be covered from day one: the LibraryProvider reducer (selection semantics), the Tree adapters, the keybinding dispatcher (combo normalization, context resolution, override merging), `enum-display` fallback.

`ponytail:` no E2E/Playwright against the Wails shell, and no Storybook — mock-driven `vite dev` in a browser *is* the component workbench. E2E enters only if seam-boundary bugs (real Wails vs mock drift) actually bite.

---

## 14. Linting & formatting

- ESLint 9 flat config: `@eslint/js` recommended + `typescript-eslint` recommended-type-checked + `react-hooks` + `jsx-a11y`. Two custom `no-restricted-imports` rules enforce the architecture mechanically: features can't import features; nothing outside `api/` imports `mock-api`/`wails-api` directly.
- Prettier stays as configured (`.prettierrc` + import sorting); drop `prettier-plugin-tailwindcss` with Tailwind.
- `bun run check` = typecheck + lint + test; the command CI runs and the definition of green.

---

## 15. Deferred — named so they land cleanly, not designed

- **Map view:** a `features/map/` sibling of grid/loupe consuming the same `useAssets` result (rows already could carry lat/lng via an `AssetRow` field — envelope rule). Rendering: MapLibre GL when it ships. Generalizing coordinates to searchable place names ("Marseille", "the Dolomites") is **reverse geocoding = backend work** — an offline gazetteer (GeoNames) at ingest, producing plain text metadata fields the *existing* search path covers. The frontend gets it for free; no map required for place search to work.
- **Loupe zoom / pan / compare:** new preview size tier per seam doc §6; loupe component grows internally.
- **Multi-select inspector** mixed-value editing: UX design first (seam doc §15.2); `InspectorView` is structured field-by-field so "varies" states slot in per field.
- **Virtualized Tree:** only if a tag taxonomy reaches tens of thousands of nodes; `@tanstack/react-virtual` is already in the bundle.
- **UI-scale setting:** one root `font-size` binding, tokens are already rem-based.
- **Drag-in import (files dropped onto the window):** two layers with different jobs. RAC `DropZone` is the *affordance* (drop-target styling, a11y) — but it only yields browser `File` objects, never filesystem paths, and ingest is path-based (binaries never cross the seam; the backend reads from disk). Paths come from **Wails' native `OnFileDrop` runtime API** instead. Integration: drop → absolute paths → one job-envelope binding (`startImportPaths(paths, targetSourceId?) → jobId`) → normal ingest pipeline, progress via `job:*`. The open *product* question is what a drop means (register into which source? copy or reference?) — decide that before wiring anything. RAC DropZone stays relevant for *internal* DnD (assets → collections/tags), where the drag payload is our own ids, not files.

---

## 16. Migration order

Each step leaves the app running against the mock:

1. **Excise:** delete untitledui component directories, remove the dependencies (§1 table), replace `frontend/CLAUDE.md` (it documents the deleted system) with a short pointer to this doc + conventions.
2. **Foundation:** `tokens.css` + three themes + global.css; primitives (`Button`, `InputField`, `Modal`, `TagChip`, `Icon`, `Toast`) — port the five existing library components onto them, dropping Tailwind classes as they go.
3. **Shell:** CSS Grid shell + pane resizing + LibraryProvider; delete router and pages/.
4. **Tree + BrowserView:** the selector + RAC-based Tree, replacing the current flat sidebar.
5. **Virtualized GridView** + selection model + loupe mode.
6. **Systems, continuously from step 2 on:** ESLint config and Vitest land with step 2 (tests accompany each component); logger and i18n wrap-up land with step 3; error boundaries with the shell.

---

## 17. Open questions (decisions Ari should make, none blocking steps 1–3)

1. **Loupe navigation scope:** prev/next walks the *filtered result set* (proposed) or only the loaded window? Result-set order needs a fetch policy for gaps when the user arrows past loaded pages.
2. **Filter persistence per target:** switching collections — do filter-bar settings reset, persist globally (proposed: persist, matches LrC), or persist per-target?
3. **Theme roaming:** theme/density/pane widths are machine-local here (`localStorage`). If they should travel with the catalog instead, they become `Settings` envelope fields — say so before step 2 hardens the localStorage habit.
4. **Fourth theme?** `graphite` default + `dark` + `light` proposed. A near-black "carbon" variant for dim-room culling is one token file whenever wanted.
