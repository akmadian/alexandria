# Porting a DS component → a colocated CSS Module (React Aria)

The recipe for the ground-up rebuild. This DS owns **look**; React Aria owns
**behavior + structure**. You do **not** ship the DS's `.jsx` or its `.ax-*`
stylesheets — you read each component's `.css` here as a **spec** and re-express
it as a `foo.module.css` next to your `foo.tsx`, driven by a RAC primitive.

Prerequisite: the DS token layer is vendored into `frontend/src/styles`
(`MAINTENANCE.md §8`), so the app speaks the DS's token names.

---

## The whole port is three moves

1. **Mount the RAC primitive** (the table below) — it brings ARIA, focus, and
   keyboard for free. Ignore the DS component's hand-rolled `role=`/click/keyboard
   logic; RAC replaces it.
2. **Class → module class.** `.ax-btn { … }` becomes `.button { … }`, applied with
   `cx(s.button, …)`. Copy the rule bodies as-is.
3. **Remap state selectors** to RAC's `data-*` attributes, nested under the base
   class in your module style. Everything else — every `var(--…)`, every px value,
   every transition — copies **verbatim**, because both sides share the DS tokens.

### State selector map

| DS component `.css` | RAC module |
|---|---|
| `.ax-x:hover` | `&[data-hovered]` |
| `.ax-x:active` | `&[data-pressed]` |
| `.ax-x:focus-visible` | `&[data-focus-visible]` (or keep `:focus-visible`) |
| `.ax-x[disabled]` / `[data-disabled="true"]` | `&[data-disabled]` |
| `.ax-x[aria-checked="true"]` | `&[data-selected]` |
| `.ax-x[aria-checked="mixed"]` | `&[data-indeterminate]` |
| `.ax-x[aria-expanded]` (trigger) | `&[data-open]` |
| `.ax-x[aria-selected="true"]` (list option) | `&[data-selected]` |

RAC also exposes `data-focused`, `data-focus-within`, `data-invalid`,
`data-placeholder`, `data-current` — reach for them when a DS rule keyed on the
equivalent native state.

---

## Worked example — Button

**DS spec** (`components/core/Button.css`, read-only here):

```css
.ax-btn {
  display: inline-flex; align-items: center; justify-content: center; gap: 6px;
  height: var(--control-h); padding: 0 10px;
  font-family: var(--font-ui); font-size: var(--fs-12); font-weight: var(--fw-medium);
  color: var(--text-1); background: var(--control-bg);
  background-image: var(--chrome-sheen); box-shadow: var(--chrome-edge), var(--emboss);
  border: 1px solid var(--border-control); border-radius: var(--r-2);
  cursor: pointer; user-select: none; white-space: nowrap;
  transition: background var(--t-instant) var(--ease-out), box-shadow var(--t-instant) var(--ease-out);
}
.ax-btn:hover  { background: var(--control-bg-hover); }
.ax-btn:active { background: var(--control-bg-active); box-shadow: var(--deboss); }
.ax-btn:focus-visible { outline: 1px dotted var(--focus-ring); outline-offset: 2px; }
.ax-btn[disabled] { color: var(--text-disabled); background: var(--control-bg); opacity: 0.55; cursor: default; }
.ax-btn--primary { background: var(--control-primary-bg); color: var(--control-primary-ink); border-color: transparent; }
```

**Ported module** (`frontend/src/components/button/button.module.css`):

```css
.button {
  display: inline-flex; align-items: center; justify-content: center; gap: 6px;
  height: var(--control-h); padding: 0 10px;
  font-family: var(--font-ui); font-size: var(--fs-12); font-weight: var(--fw-medium);
  color: var(--text-1); background: var(--control-bg);
  background-image: var(--chrome-sheen); box-shadow: var(--chrome-edge), var(--emboss);
  border: 1px solid var(--border-control); border-radius: var(--r-2);
  cursor: pointer; user-select: none; white-space: nowrap;
  transition: background var(--t-instant) var(--ease-out), box-shadow var(--t-instant) var(--ease-out);

  &[data-hovered]  { background: var(--control-bg-hover); }
  &[data-pressed]  { background: var(--control-bg-active); box-shadow: var(--deboss); }
  &[data-focus-visible] { outline: 1px dotted var(--focus-ring); outline-offset: 2px; }
  &[data-disabled] { color: var(--text-disabled); background: var(--control-bg); opacity: 0.55; cursor: default; }
}
.primary { background: var(--control-primary-bg); color: var(--control-primary-ink); border-color: transparent; }
```

```tsx
// button.tsx — behavior is RAC's, look is the module
import { Button as AriaButton, type ButtonProps } from "react-aria-components";
import { cx } from "@/lib/cx";
import s from "./button.module.css";

export const Button = ({ variant = "default", className, ...rest }:
  ButtonProps & { variant?: "default" | "primary" }) => (
  <AriaButton {...rest} className={cx(s.button, variant === "primary" && s.primary, className)} />
);
```

Note the only differences from the spec are the class names and the state-selector
shells. Every token reference is identical.

---

## Which RAC primitive replaces which DS component

**Chrome (port to RAC):**

| DS component(s) | React Aria primitive |
|---|---|
| Button, IconButton | `Button` |
| Toggle | `ToggleButton` or `Switch` |
| Checkbox (+ indeterminate) | `Checkbox` (`isIndeterminate`) |
| SegmentedControl | `ToggleButtonGroup` (or `RadioGroup`) |
| Select | `Select` + `ListBox` + `Popover` |
| Slider, RangeSlider | `Slider` (multi-thumb for range) |
| Input | `TextField` + `Input` |
| Tooltip | `Tooltip` + `TooltipTrigger` |
| Menu, ContextMenu | `Menu` + `MenuTrigger` |
| Popover, PopoverSurface | `Popover` / `Dialog` |
| PickerPopover, FilterPopover, FilterCriteriaPicker | `Popover` + `ListBox`/`GridList` |
| QueryBuilder | composed RAC (`Group`, `Button`, `Menu`, `ListBox`) |
| Tree, TreeRow | `Tree` + `TreeItem` |
| CommandPalette | `Autocomplete`/`ComboBox` + `ListBox` in a modal `Dialog` |
| Toast | `ToastRegion` + `Toast` (RAC toast) |
| ConfirmModal | `Modal` + `Dialog` + `AlertDialog` semantics |
| Banner | plain element (no interaction) — port CSS only |
| InspectorGroup, MetaRow | plain elements — port CSS only |
| RatingStars | `RadioGroup` (or bespoke; store-owned) |
| LabelPicker | `ListBox`/`RadioGroup` |
| KeybindChip, Pill, Badge, Stat, DistributionBar, Progress | presentational — port CSS only |

**Content surfaces (bespoke per `09`, NOT RAC):** GridCell / grid, loupe, cull,
compare, filmstrip — selection is store-owned on `tanstack-virtual`. Their *look*
still ports from the DS `.css`; only the behavior is hand-built. RAC's Virtualizer
is deliberately ruled out here.

---

## Gotchas

- **Read the `.css`, not the `.jsx`.** Component styling now lives in each
  component's sibling `.css` file (extracted from the old runtime injection) — that
  file is the faithful spec. The `.jsx` is a behavior reference only.
- **Tokens must be vendored first.** If a `var(--control-bg)` renders unstyled, the
  DS token layer isn't in `src/styles` yet.
- **Don't reintroduce primitives.** Port keeps semantic tokens (`--control-bg`,
  `--r-2`, `--text-1`); never resolve them down to `--n-42` or hex — the ESLint rule
  and the theme system both depend on the semantic layer.
- **Focus ring:** the DS uses a 1px dotted chrome focus (`--focus-ring`); your
  `global.css` may set a global `:focus-visible`. Pick one; don't double up.
- **`cx` already exists** at `@/lib/cx` — use it, matching the existing button/select
  pattern.
