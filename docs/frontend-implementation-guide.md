# Frontend Implementation Guide

A hands-on companion to `frontend-ui-architecture.md`. That doc is the *why*; this is the *how* — what's already built, how to run it, the conventions with copy-paste snippets, and where to go next. Written for picking up implementation yourself.

---

## 1. What's built (the foundation)

The Untitled UI starter is gone. What replaced it runs today against the mock catalog — `bun run dev` shows the full shell: browser tree, virtualized asset grid, inspector, filter bar, status bar, three themes, keyboard triage.

```
src/
├── api/                    # UNTOUCHED — the settled seam (contract, mock, queries)
├── models/                 # UNTOUCHED — domain shapes
├── app/                    # composition root
│   ├── app.tsx             #   providers + root error boundary + crash screen
│   ├── shell.tsx           #   the CSS-Grid layout, pane resizing, per-pane boundaries,
│   │                       #   derives the ListQuery and fetches it once
│   ├── shell.module.css
│   ├── library-state.tsx   #   LibraryProvider: the one reducer (target/filters/selection/view)
│   ├── library-state.test.ts
│   └── error-boundary.tsx  #   the single class component
├── components/             # owned primitives (RAC-based, unstyled → our CSS)
│   ├── button/  input-field/  select/  modal/  tag-chip/  toast/  icon/  tree/
│   └── (each: name.tsx + name.module.css)
├── features/               # domain views — each owns its components + styles
│   ├── browser/            #   mode selector + Tree + adapt.ts (domain→TreeNode)
│   ├── filter-bar/         #   search/type/rating/sort/density/theme controls
│   ├── grid/               #   GridView (virtualized) + AssetCard
│   ├── inspector/          #   InspectorView + RatingControl + MetaSection
│   └── jobs/               #   StatusBar + useJobs (push-event chrome)
├── lib/                    # cross-cutting singletons, one concern per file
│   ├── cx.ts  theme.ts  logger.ts  format.ts  enum-display.ts  keys.ts
│   └── keys.test.ts
├── i18n/                   # index.ts (init) + locales/en.json
├── styles/                # tokens.css + themes/{dark,light}.css + global.css
└── test/                  # setup.ts + render.tsx (renderWithApp helper)
```

What is deliberately **stubbed or deferred** (real work left for you):
- **GridView fetches nothing itself yet** — the shell hands it the whole result set. The sparse-window upgrade (fetch pages by visible range) is the marked next step; see §7.
- **Folder tree** — `browser/adapt.ts` produces flat lists; `sourcesToNodes` has a `TODO` where `useFolderTree` + lazy expansion plug in.
- **No loupe, no command palette, no settings/keybinding UI, no import/drop** — all designed, none built.
- **Toast/logger sinks** — logger batches to a no-op until the backend `logBatch` binding exists.

---

## 2. Running it

```bash
bun run dev         # vite dev server against the mock catalog
bun run check       # typecheck + lint + tests — the definition of green, run before committing
bun run test:watch  # vitest in watch mode while building
bun run coverage    # v8 coverage report (ratchet, not a gate yet)
```

`bun run check` must pass. It's `tsc -b --noEmit && eslint src && vitest run`.

---

## 3. Your four questions, answered

**Does RAC Virtualizer help the grid?** No — it needs the whole collection up front and only does sequential loading, so it can't express our sparse random-access window over a 500k result set (scrollbar-jump to the middle, load that page), and it has no Tree support. We use `@tanstack/react-virtual` instead. (Full note in `frontend-ui-architecture.md` §6.)

**Does RAC do page layout?** No — RAC is components + behavior hooks only. No grid/flex system, no responsive utilities. Layout is our own CSS Grid (`shell.module.css`) plus the `auto-fill minmax()` asset grid. That's fine; desktop-only means layout is static and CSS handles it in ~40 lines.

**Command palette** — worth doing, and nearly free: it's "the keybinding action registry, made searchable." `lib/keys.ts` already exposes `ACTIONS` with labels and combos. A palette is RAC `Autocomplete` + `Menu` in a `Modal`, bound to `mod+k` (the action is already registered as `command_palette`). Lives in a future `features/command-palette/`. See §6 for the sketch.

**DropZone / ingest** — yes, pass paths, ingest as usual, but RAC `DropZone` alone isn't enough: it yields browser `File` objects, never filesystem paths, and ingest is path-based (bytes never cross the seam). The real path source is Wails' native `OnFileDrop` runtime API → absolute paths → one new `startImportPaths(paths)` job binding → normal pipeline, progress via `job:*`. RAC DropZone stays useful for *internal* drag (assets → collection/tag), where the payload is our own ids. The open product question is what a drop *means* (which source? copy or reference?) — decide before wiring. (Recorded in `frontend-ui-architecture.md` §15.)

---

## 4. The conventions, with snippets

### i18n — never hardcode display text

Every user-visible string is a key in `i18n/locales/en.json`, namespaced by feature. Enum labels and keybinding labels are keys too.

```tsx
import { useTranslation } from "react-i18next";

function Thing() {
    const { t } = useTranslation();
    return (
        <>
            <button>{t("inspector.close")}</button>
            {/* interpolation + plurals: en.json has count_one / count_other */}
            <span>{t("filterBar.count", { count: total })}</span>
            {/* enum → label key, then translate (never switch on the code yourself) */}
            <span>{t(fileTypeDisplay(row.fileType).labelKey)}</span>
        </>
    );
}
```

Adding a string: add the key to `en.json` (keep the feature namespace), use `t("...")`. Never build a sentence by concatenating translated fragments — add a full key with interpolation. Dates/numbers/sizes never go through i18n — see `format.ts` below.

### Formatting — always `lib/format.ts` (Intl), never a date lib

```tsx
import { formatDate, formatDateTime, formatBytes, formatDuration } from "@/lib/format";

formatDateTime(asset.capturedAt);  // "Jun 24, 2026, 4:41 AM" — locale-aware
formatBytes(asset.sizeBytes);      // "10.9 MB"
formatDuration(asset.durationSecs);// "0:28"
```

These read the active i18n locale and memoize their `Intl` instances. Data stays as ISO strings / numbers in the cache; convert only at render.

### Enum display — one place, forward-compatible

Seam convention 6: a new enum value from a newer backend must degrade, not crash. `lib/enum-display.ts` is the only place enum codes map to `{ icon, labelKey }`, and every map has a fallback:

```ts
export function fileTypeDisplay(t: FileType | string): EnumDisplay {
    return FILE_TYPES[t as FileType] ?? FILE_TYPE_FALLBACK;  // unknown → generic icon + label
}
```

Add a new file type on the Go side → grid renders it generically until you add a row here. Never `switch (row.fileType)` in a component.

### Server state — hooks only, never `api` directly

Components call the `api/queries.ts` hooks; they never import `mock-api`/`wails-api` (ESLint enforces this). The one sanctioned direct `api` use is event subscription (no request/response), which is why `features/jobs/use-jobs.ts` imports `api` for `onJobProgress`/`onJobDone`.

```tsx
const { data, isPending } = useAssets(query);   // list
const { data: asset } = useAsset(selectedId);   // full record on selection
const patch = usePatchAssets();                 // optimistic triage
patch.mutate({ target: { ids }, patch: { rating: 5 } });
```

### Client state — the LibraryProvider reducer

Shared view state (browse target, filters, selection, view mode) lives in one reducer. Read and dispatch from split contexts:

```tsx
import { useLibraryState, useLibraryDispatch } from "@/app/library-state";

const { selection, target, filters } = useLibraryState();
const dispatch = useLibraryDispatch();
dispatch({ type: "select", id, additive: e.metaKey, rangeIds });  // same action mouse & keyboard use
```

Anything derivable is derived, not stored: the `ListQuery` is `deriveListQuery(state)` (pure, tested), the inspector subject is `useAsset(lastSelectedId)`. If you're tempted to add a field to the reducer, check it can't be derived first.

### Styling — semantic tokens, CSS Modules, nesting

Component styles are co-located `*.module.css`. Use **semantic** tokens only (`--bg-surface`, `--text-primary`, `--accent`), never primitives (`--grey-800`) or raw hex. That's what makes the three themes work for free.

```css
.card {
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    color: var(--text-secondary);
    &[data-selected] { background: var(--bg-selected); }  /* native nesting, no Sass */
}
```

Two typographic utilities live in `global.css`: `.u-label` (small-caps section headings — the app's signature) and `.u-data` (mono, tabular-nums for counts/dimensions/timestamps). Reuse them; don't re-create.

Chrome stays hue-free in every theme. Hue is reserved for **data**: `--label-{red,orange,…}` (color labels, tag colors) and the single `--accent`. If you're adding a colored chrome element, you're probably wrong — reach for a grey.

### Toasts — module-level, callable from anywhere

```ts
import { toast } from "@/components/toast/toast";
toast("error", t("some.key"), { label: t("retry"), onPress: () => mut.mutate(vars) });
```

Only *unexpected* failures earn a toast (seam doc §9). Degraded states (source offline, missing thumb) get calm inline treatment — no red alert.

### Logging

```ts
import { log } from "@/lib/logger";
log.error("import failed", { jobId, kind });  // ring buffer + batched to backend; error also flushes now
```

`window.onerror`, unhandled rejections, and error boundaries already feed the logger. Don't log per-keypress on triage paths.

---

## 5. Adding a primitive component

Pattern: one folder, RAC foundation if the interaction is non-trivial, our CSS. Example — a `Checkbox`:

```
components/checkbox/checkbox.tsx        # wraps RAC <Checkbox>, applies s.checkbox
components/checkbox/checkbox.module.css  # semantic tokens; key on RAC's data attributes
```

```tsx
import { Checkbox as AriaCheckbox, type CheckboxProps } from "react-aria-components";
import { cx } from "@/lib/cx";
import s from "./checkbox.module.css";

export const Checkbox = ({ className, ...rest }: CheckboxProps) => (
    <AriaCheckbox {...rest} className={cx(s.checkbox, typeof className === "string" ? className : undefined)} />
);
```

RAC exposes state as data attributes — style with `&[data-selected]`, `&[data-hovered]`, `&[data-focused]`, `&[data-disabled]`. The `Aria*` import alias keeps RAC components visually distinct from ours. Trivial semantics (a chip, a plain span)? Skip RAC, use the native element.

---

## 6. Adding a feature

Rules: a feature owns its components/styles/hooks; it may import `components/`, `lib/`, `api/`; it must **not** import another feature (coordinate through LibraryProvider state). Cross-feature shared UI moves to `components/`.

### Sketch: command palette (a good first feature to build)

```
features/command-palette/command-palette.tsx
```

```tsx
import { Autocomplete, Menu, MenuItem, useFilter } from "react-aria-components";
import { Modal } from "@/components/modal/modal";
import { ACTIONS, comboFor } from "@/lib/keys";
// open state: local useState toggled by the `command_palette` action handler
// (register it in shell.tsx or a small hook via registerHandlers).

// filter ACTIONS by the current context + query, render label + comboFor(a.id),
// onAction → invoke the same handler the keyboard would. Zero new state concepts.
```

The action registry (`ACTIONS`) already carries everything a palette shows. This is why the keyboard system was built as a registry, not scattered `useHotkeys`.

### Wiring keyboard actions in a feature

```tsx
import { registerHandlers } from "@/lib/keys";

useEffect(() => {
    return registerHandlers({          // returns the unregister cleanup
        rate_5: () => patchSelected({ rating: 5 }),
        flag_pick: () => patchSelected({ flag: "pick" }),
    });
}, []);  // register once; read live state via refs (see grid-view.tsx for the pattern)
```

Handlers must be registered for action ids that exist in `ACTIONS` (it throws on typos). To add a *new* binding: add an `ActionDef` to `ACTIONS` (id, context, defaultCombo, labelKey), add the label to `en.json` under `actions.*`, register a handler. Conflicts are checked synchronously (`setOverride` returns the conflicting id) — no backend round-trip.

---

## 7. Suggested next steps, in order

Each leaves the app green against the mock.

1. **Sparse grid windowing** (the one marked ceiling-raiser). Move fetching into `GridView`: map the virtualizer's visible row range → page offsets → `useAssets({ ...query, page })` per window, keep a sparse array by offset. The shell stops passing whole `rows`; it just passes the derived query. This is the single most load-bearing piece for real catalogs — do it before anything cosmetic. Seam doc §5 has the design and the id-snapshot upgrade behind it.
2. **Folder tree.** Fill in `browser/adapt.ts`: `useFolderTree(sourceId)` on first expand, `FolderNode` → `TreeNode`, dispatch a `{kind:"folder"}` target (add it to `BrowseTarget` + `deriveListQuery`). The Tree already supports lazy expand via `onExpand`.
3. **Loupe view.** `features/loupe/` consuming `useAsset` + `previewURL`; `viewMode: "loupe"` already exists in state and swaps the main region. Prev/next walks the current grid window (open question §17.1 in the architecture doc — decide result-set vs window).
4. **Settings + keybinding UI.** A `Modal` with `useSettings`/`useKeybindings`; the keybinding editor uses `setOverride`'s synchronous conflict return for inline validation. Good moment to propose the contract slimming to `getKeybindingOverrides`/`saveKeybindingOverrides` (architecture doc §9).
5. **Command palette** (§6).
6. **Import + drop** — needs the backend `startImportPaths` binding and the product decision on drop semantics first.

Alongside all of it: add tests as you go (the reducer, adapters, and keys already have them as the template), and add the `renderWithApp` component tests for each feature's key behavior.

---

## 8. Gotchas hit while building (so you don't re-hit them)

- **`preview_click` vs `onPointerDown`.** Asset cards select on `pointerdown` (feels instant for triage). Browser-automation "click" tools that fire only `click` won't select — dispatch a real `PointerEvent` in tests/automation.
- **Node 26 + happy-dom `localStorage`.** Node 26 ships a disabled global `localStorage` that shadows happy-dom's, which doesn't expose one anyway. `test/setup.ts` installs a Map-backed polyfill. The `ExperimentalWarning: localStorage is not available` line during tests is harmless noise from Node, not our code.
- **ESLint 9, not 10.** `typescript-eslint` 8 targets ESLint 9's API; ESLint 10 crashes it. Pinned to `^9`.
- **`react-hooks` 7 bundles React-Compiler rules.** We keep the classics + `set-state-in-effect` (it caught a real smell in the filter bar) but disabled `react-hooks/refs` and `react-hooks/incompatible-library` — they fight legitimate patterns (encapsulated imperative hooks, TanStack Virtual) when the compiler isn't running. Re-enable if you adopt the compiler. Config comments explain each.
- **Grid column count needs a callback ref, not an effect.** The scroll element mounts only after `isPending` clears, so an empty-dep effect measures nothing. `GridView` uses a callback ref (with React 19 cleanup) to observe exactly on attach. If you refactor the grid, preserve that.
- **RAC `onSelectionChange` can hand you `null`/`"all"`.** Guard both (see `select.tsx`, `tree.tsx`).

---

## 9. The mental model, in three sentences

Data crosses the seam as codes and ISO strings and stays that way in the TanStack cache; it becomes pixels only at the render edge, through `format.ts` and `enum-display.ts`. Server state is Query, the one bit of shared client state is the LibraryProvider reducer, and everything else is props or `localStorage`. Primitives know nothing about the domain, features compose primitives with hooks and never import each other, and the whole thing is one window whose "navigation" is just two fields of reducer state.
