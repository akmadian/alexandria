# Alexandria Design System — integration guide for Claude Code

You are integrating the **Alexandria Design System** into the Alexandria app
(Wails v2 · Go engine + React 19 UI · Bun · CSS Modules · React Aria Components).
This bundle is the design system's distributable. **This file is authoritative for
how the app consumes it.** Read it fully before writing UI.

The design system is the **source of truth for look**. You own **behavior and
structure** (React Aria + your store/seam). Never invent visual values — pull them
from the tokens here.

---

## What's in this bundle

```
alexandria-ds.css     The entire visual foundation, one file: @font-face (4 faces) +
                      the full token layer. Import once at app boot.
fonts/                Font binaries referenced by alexandria-ds.css. Fully offline.
pharos-ascii.js       The <pharos-ascii> WebGL brand mark (web component).
assets/               Ready-to-use brand renders — icon + flat glyph (see "Brand assets").
components-spec/*.css  Per-component VISUAL SPECS (read to port; do NOT import — they
                      use global .ax-* classes and are reference only).
PORTING.md            The deeper component-port reference.
CLAUDE.md             This file.
VERSION               Semver of this build.
```

## Install (once)

```ts
// src/main.tsx (or app boot)
import "@/styles/alexandria-ds.css";   // tokens + fonts; keep fonts/ beside this file
import "@/lib/pharos/pharos-ascii.js"; // registers <pharos-ascii>; only where used
```

`alexandria-ds.css` sets tokens on `:root` and theme scopes. Set the theme/material
on a root element (the app already does this in `index.html`):

```html
<html data-theme="middle" data-material="flat">
```
- `data-theme`: `middle` (default) · `light` · `dark`
- `data-material`: `flat` (default, opaque + shadowless) · `glass` (neumorphic emboss/deboss)

Since this bundles all four faces, it is the single source of type. You can drop a
separate `geist` npm dependency and rely on this (harmless if you keep both).

---

## Golden rules (non-negotiable — these are how the app stays on-brand)

1. **Semantic tokens only.** Use the semantic names below. **Never** use a primitive
   (`--n-42`, `--label-red`) or a raw hex/px color in a component. If you need a value
   that has no semantic token, that's a missing token — add it to the DS, don't reach
   down a layer. (ESLint enforces this in the repo.)
2. **Chrome owns no hue.** The UI is achromatic in every theme. Color = **user data
   only** (color labels, tag hues, health). `--accent` is **UNSET by default**; read it
   as `var(--accent, <neutral fallback>)` and **never let a state depend on it** —
   selection/focus/hover must read with the accent absent (via lightness, borders,
   inversion).
3. **Canvas is `--canvas` and constant across themes.** It is the photo backdrop
   (exact mid-grey so user images read true). **Never put text or chrome on the canvas**
   — text lives on panels (`--panel*`).
4. **Flat construction.** Structure comes from borders + lightness steps. **No shadows
   in flat mode.** Elevation/shadow tokens (`--emboss`, `--deboss`, `--chrome-sheen`,
   `--chrome-edge`) resolve to `none` in flat and only turn on under `[data-material="glass"]`.
5. **Blur is transient-only.** `--glass-*` + `backdrop-filter` on popovers/modals/toasts/
   palette. Persistent chrome is opaque.
6. **Motion is evidentiary.** Use `--t-*` durations; animate `transform`/`opacity` only;
   the tokens already collapse to `0ms` under `prefers-reduced-motion`. No decorative loops.
7. **Data values render in mono**, tabular: `font-family: var(--font-mono);
   font-feature-settings: var(--mono-settings);` — counts, filenames, dimensions, EXIF.
8. **No emoji.** Unicode block/geometric glyphs in the mono face are the sanctioned
   telegraphy.
9. **Compact density.** Body text 11–13px (`--fs-11`…`--fs-13`); controls are `--control-h`.
10. **Iconography:** the brand mark is `<pharos-ascii>` / the DS glyph language for
    identity, status, and empty-state moments — never a stock set *for those*. (Functional
    chrome icons in this app come from `lucide-react`; keep that consistent, but reach for
    the DS glyph language for brand/status/empty states.)

---

## Token reference (the vocabulary you code against)

**Surfaces** (text goes on panels, never on `--canvas`):
`--canvas` · `--panel` · `--panel-raised` · `--panel-sunken` · `--lightsout` (cull/loupe) ·
`--app-bg` (card substrate, glass only).

**Text:** `--text-1` (primary) · `--text-2` (secondary) · `--text-3` (tertiary) ·
`--text-disabled` · `--text-invert` (on inverted/selected grounds).

**Borders:** `--border-strong` (pane seams) · `--border-subtle` (rules) ·
`--border-control` (control outlines).

**Selection & focus (§6):** `--select-ground` (selected pad; accent-tintable) ·
`--select-ink` · `--cursor-ring` (NEVER accent-tinted) · `--hover-ground` ·
`--focus-ring` (chrome keyboard focus, dotted).

**Controls:** `--control-bg` · `--control-bg-hover` · `--control-bg-active` ·
`--control-primary-bg` / `--control-primary-ink` / `--control-primary-bg-hover`
(primary = inversion, not hue). Metrics: `--control-h` (26px) · `--control-h-lg` (30px) ·
`--row-h` · `--filterbar-h` · `--statusbar-h` · `--titlebar-h`.

**User-data color (only place hue is allowed):** `--data-danger` · `--data-success` ·
`--data-warning`; the six label hues live in the primitive layer — surface them only as
user data (labels/health), never as chrome.

**Radii:** `--r-1` (7px) · `--r-2` (11px) · `--r-3` (15px) · `--r-pill`.

**Spacing:** `--sp-1`…`--sp-9` = 2 · 4 · 6 · 8 · 12 · 16 · 20 · 24 · 32 px.

**Type:** faces `--font-ui` · `--font-mono` · `--font-pixel` · `--font-display`
(Instrument Serif — wordmark/splash/about only). Sizes `--fs-10`…`--fs-18`
(10/11/12/13/15/18). Weights `--fw-regular|medium|semibold`. Tracking
`--tracking-label` / `--tracking-wide`. `--mono-settings` for tabular data.

**Motion:** `--t-instant` (60) · `--t-fast` (100) · `--t-move` (160) · `--t-moment` (420ms);
`--ease-out` · `--ease-in-out`; `--dither-steps` (glyph/empty-state entrances).

**Z:** `--z-chrome` · `--z-popover` · `--z-modal` · `--z-palette` · `--z-toast` · `--z-moment`.

**Transient material:** `--scrim` · `--glass-bg` · `--glass-border` · `--glass-blur`.

---

## Building a component

**Chrome → React Aria Components** (aliased `Aria*`), skinned with a colocated CSS
Module. **Content surfaces** (grid, loupe, cull, compare, filmstrip) are **bespoke** on
`tanstack-virtual` with store-owned selection — not RAC.

To build a chrome component, read its spec in `components-spec/<Name>.css` and port it
in three moves:

1. **Mount the RAC primitive** (table below) — it brings ARIA/focus/keyboard.
2. **Class → module class:** `.ax-btn { … }` → `.button { … }`, applied via `cx(s.button, …)`.
   Copy rule bodies as-is; every `var(--…)` is already correct.
3. **Remap state selectors** to RAC `data-*`, nested under the base class:

| Spec `.css` | RAC module |
|---|---|
| `:hover` | `&[data-hovered]` |
| `:active` | `&[data-pressed]` |
| `:focus-visible` | `&[data-focus-visible]` |
| `[disabled]` / `[data-disabled="true"]` | `&[data-disabled]` |
| `[aria-checked="true"]` | `&[data-selected]` |
| `[aria-checked="mixed"]` | `&[data-indeterminate]` |
| `[aria-expanded]` (trigger) | `&[data-open]` |
| `[aria-selected="true"]` (option) | `&[data-selected]` |

### RAC primitive per DS component

Button/IconButton→`Button` · Toggle→`ToggleButton`/`Switch` · Checkbox→`Checkbox`
(`isIndeterminate`) · SegmentedControl→`ToggleButtonGroup`/`RadioGroup` ·
Select→`Select`+`ListBox`+`Popover` · Slider/RangeSlider→`Slider` · Input→`TextField`+`Input` ·
Tooltip→`Tooltip`+`TooltipTrigger` · Menu/ContextMenu→`Menu`+`MenuTrigger` ·
Popover→`Popover`/`Dialog` · Tree→`Tree`+`TreeItem` · CommandPalette→`Autocomplete`/`ComboBox`
in a modal `Dialog` · Toast→`ToastRegion`+`Toast` · ConfirmModal→`Modal`+`AlertDialog` ·
Banner/InspectorGroup/MetaRow/KeybindChip/Pill/Badge/Stat/DistributionBar/Progress →
presentational, port CSS only.

Full example and gotchas: `PORTING.md`.

---

## Pharos brand mark

`import "@/lib/pharos/pharos-ascii.js"` (side-effect registers the element), then render
`<pharos-ascii mode="…">` in a bespoke surface (first-run, feature-reveal). React 19
handles custom elements natively. It's a live WebGL component — never bake it to frames.

---

## Brand assets (`assets/`)

- `logo-icon.svg` / `logo-icon-512.png` / `favicon-32.png` — the hero app icon (glass
  lighthouse, lit prism lantern, beam, on the dark squircle). Launcher, splash, About,
  favicon. The prism lantern is the one sanctioned hue in the mark — **do not recolor** the
  hero icon or add hue anywhere else.
- `logo-glyph.svg` — **the flat glyph**, in `currentColor`: the monochrome lighthouse
  silhouette for toolbars, favicons, dev-corner, any small chrome. It inherits text color —
  set `color` on a parent. This is the default small-chrome mark. Prefer the SVG (scalable +
  recolors via `currentColor`).
- `logo-glyph-beam.svg` — flat glyph with the beam, for feature-reveal accents.
- `logo-glyph-white-64.png` / `logo-glyph-ink-64.png` — raster glyphs where SVG isn't convenient.

Drop these under `src/assets/brand/` (or `public/`) in the app. For the flat glyph in JSX,
prefer the DS `LogoGlyph` treatment (currentColor) so it themes with the chrome.

## Updating this bundle

The DS is authored separately; there is **no live link**. When a new DS release is cut:
re-vendor this bundle (via the repo's `scripts/vendor-ds.sh`), review `git diff`, rebuild.
Token/CSS/SDF tweaks are drop-in (no call-site edits); renamed tokens or changed pharos
attributes are the only breaking changes and are called out in the DS changelog.

## Do NOT

- Import `components-spec/*.css` (they're global `.ax-*` specs — reference only).
- Use primitives or raw hex/px in components (semantic tokens only).
- Add shadows in flat mode, or make any state depend on `--accent`.
- Put text/chrome on `--canvas`.
- Wrap or ship the DS's original `.jsx` components — rebuild behavior on React Aria.
