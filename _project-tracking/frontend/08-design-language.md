# Design Language

**Status:** design round 2026-07-08 (Ari + Claude, worked from the `~/Desktop/design-inspo` set).
This doc is the visual-language authority. It **supersedes** the visual decisions embedded in the
current `frontend/src/styles/` (tokens, themes, amber accent — that implementation is throwaway)
and **amends** the chrome/theme lines of `01-flows-and-views.md` where they conflict (amendments
repeated here; `01` remains authoritative for flows, views, and the transparency-as-chrome
layers). Feeds the Claude Design handoff for the full design system.

## Stance

**Precision instrument, selectively delighted.** Neutral, dense, mono-inflected chrome that gets
out of the assets' way; the delight budget is spent on a small set of icons and state-reporting
microinteractions. Non-software register: Teenage Engineering restraint (not their website),
Apple Liquid Glass materiality — used transiently, never persistently.

## Color

- **Chrome owns no hue.** (Carries forward from `01`.) Selection, focus, hover, and highlight
  are achieved with *neutral* means: lightness steps, borders, inversion. There is **no accent
  color by default** — the previous amber accent is dead.
- **Optional user accent.** The user may pick an accent to sprinkle through the UI; the system
  must be designed *accent-optional*, never accent-shaped. Every state must read with the accent
  unset. The accent, when set, is an enhancement layer (e.g. tinting the selection border),
  never the sole signal.
- Hue appears in exactly three places:
  1. **User data** — color labels, tag colors, health/danger states (fixed meanings, identical
     across themes).
  2. **Icon chips** — tiny fixed color accents inside otherwise-monochrome glyphs (the
     heart-rate-widget pattern: grey chart, three colored dots doing the semantics).
  3. **Gradient moments** — rationed events, not decoration: first-run splash, import
     completion, new-feature reveal. Soft prismatic gradients on neutral ground. P3 color
     welcome here (macOS first-class).

## Surfaces and themes

- **The canvas is photographic middle grey**: 18% reflectance, L\* = 50, ≈ `#777777` sRGB
  (deliberately not `#808080`). Canvas = where assets live: grid backdrop, loupe surround,
  compare/cull grounds.
- **Panels sit darker than the canvas** (sidebars, inspector, filter bar, status bar — roughly
  L\* 30–38 with light text), so chrome reads as a frame *on top of* the neutral field and
  recedes; thumbnails pop against the lighter canvas they sit in. Exact steps are the design
  system's to tune; the relationship (panels darker than canvas, text-contrast on panels not on
  canvas) is the rule.
- **Three themes; the middle-grey default is first-class, light and full-dark are second-class.**
  Same semantic-token architecture (default token values = the middle theme; others override).
  Light register when built: paper-and-ink (warm neutral surfaces, ink text, tiny precise type).
  Dark register: near-black for dim-room culling.
- **Construction is flat.** Borders and lightness steps define structure. No shadow stacks, no
  elevation ramp.
- **Blur is the layering material — transient only.** Overlays that enter and leave (command
  palette, modals, drawers, popovers) differentiate by backdrop-blurring and dimming the app
  behind them, not by depth. **Persistent chrome is always opaque** — translucent sidebars would
  leak asset color into the UI, defeating hue-free chrome. Modal-over-modal: the stack dims/blurs
  progressively (the modal-stack reference).

> **Amended 2026-07-10 (Ari): the app default adopts the DS `glass` material**, superseding the two
> bullets above (flat construction + transient blur). `<html data-material="glass">` is now the
> default; `data-material="flat"` is the opt-out. What this changes and preserves:
> - **Depth returns.** Persistent chrome gains the DS's neumorphic construction —
>   `--emboss`/`--deboss`/`--chrome-edge`/`--chrome-sheen`/`--card-float`, plus card radius and
>   inter-card gaps. This reverses "construction is flat / no shadow stacks."
> - **Still opaque, so hue-free holds.** Glass mode's `--glass-bg` and panels are opaque (it is
>   *neumorphic* glass, not translucent); no asset color leaks into chrome — the load-bearing
>   rationale for "persistent chrome always opaque" is intact.
> - **Transient blur is off by default in glass** (`--glass-blur: 0`): overlays read as opaque
>   floating cards (lift + light edge) rather than frosted. If we want the frosted-overlay
>   treatment *on top of* glass chrome, re-enable blur for transient surfaces in glass mode (a DS
>   token tweak) — flagged, not yet done.

## Selection, cursor, and focus (spec requirement)

Selection is neutral and must be *specced*, not improvised. The design system must deliver
distinct, instantly-readable treatments for:

- hover · **cursor** (focused asset, C1) · **selected** · **selected + cursor** — four distinct
  cell states, per the vocabulary's cursor/selection split;
- **multi-selection and discontinuous selection blocks** — ranges must read as ranges, gaps as
  gaps, at grid densities from huge tiles to tiny;
- the acid test: **selection reads on any image, in any theme, with or without a user accent**
  (lightness shift + border/inversion is the proven neutral toolkit; LrC's cell-brightening is
  prior art);
- keyboard focus ring for chrome controls, also neutral, distinct from asset selection.

## Iconography: the dot-matrix glyph system

The strongest inspo through-line, and continuous with the status bar's block-character
telegraphy (`01` §transparency): **a custom dot-matrix / pixel-grid glyph language.**

- Glyphs are monochrome by default, drawn on a strict pixel grid (align to device pixels at
  target sizes; bitmap-crisp, never anti-aliased mush).
- A handful of **hero positions** (app icon, view-mode marks, maybe source-type badges) get the
  colored-dot or gradient-orb treatment — the "small sparingly-colored icon" warmth budget.
- The native transition for this language is **dither/dissolve** — glyphs and empty-state
  graphics materialize as dots (the Basedash empty-state reference). This doubles as the
  ambient-motion signature.
- Logo direction (exploration, not locked): notched-star / sparkle / burst marks from the inspo
  set suit the Alexandria name (star of the library, lighthouse); dot-matrix wordmark is on-theme.

## Motion

- **Ambient and evidentiary**: motion exists to report state, never to garnish. Numbers tick on
  change; status glyphs morph; the watcher heartbeat pulses; dot-graphics dissolve in; hover
  reveals. Character-swap animation in the mono face stays (no SVG weight).
- **Big expressive motion stays confined to the sanctioned moments** (cull key-feedback overlay,
  import completion, first-run) — unchanged from `01`.
- Fast and interruptible; respect `prefers-reduced-motion` (ambient layer degrades to instant
  state swaps).

## Typography

- **Split stands: sans for UI, mono for data values/counts/telegraphy.**
- **DECIDED (Ari, 2026-07-08): build with Geist Sans + Geist Mono.** OFL, designed as a pair,
  precision-instrument register, variable. System stacks (`system-ui` / `ui-monospace`) remain
  the fallback in the token stacks. Constraint held: **OFL-licensed only** (GPL app, fonts ship
  in the bundle).
- **Geist Pixel is the sanctioned pixel/display accent face** — thrown in *sparingly* for visual
  interest (dev corner, telegraphy moments, wordmark exploration); native companion to the
  dot-matrix glyph system. (Departure Mono was the runner-up for this role.)
- **Back pocket** (experiment candidates, all OFL, swap = token change): IBM Plex Sans/Mono (the
  warmth option, awesomely engineered), JetBrains Mono (solid mono alternate), Iosevka
  (fantastic glyph library; the data-density wildcard). Inter rejected — too flat/generic.
- **Possible future setting:** user font customization (pick from bundled faces, or system) —
  cheap because everything routes through `--font-ui`/`--font-data` tokens; note only, not
  committed roadmap.
- Small-caps + tracking label voice carries forward.

## Layout amendments (supersede `01` shell sketch details)

- **Sidebars are full-height and hem in the center space**; the center region (and sidebars) run
  the window's full height.
- **The filter bar lives INSIDE the center pane** — it operates on that view's contents, so it is
  visually scoped to it. It never spans across the sidebars.
- **Integrated macOS title bar** (traffic lights inline with app chrome) is sanctioned.
- Governing principle: **layout speaks the hierarchy** — components are visually scoped to what
  they govern, so structure reads without labels or verbosity.

## Platform

**macOS is first-class** (rendering, color management, real backdrop-blur materials, P3 for
gradient moments); Linux/Windows second-class. Mostly an implementation concern, but it licenses
the design system to assume high-DPI and real blur.

## Anti-goals

- No forced accent color; no state that only reads via accent.
- No shadow/elevation stacks; no persistent translucent chrome.
- No hue in chrome; no decorative gradients outside the sanctioned moments.
- No generic line-icon set (Lucide/Feather et al.) — the glyph language is custom or it isn't.
- No filter bar spanning over sidebars; no motion that reports nothing.
