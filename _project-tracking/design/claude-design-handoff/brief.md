# Alexandria — Design System Brief

Handoff for Claude Design. Self-contained: everything binding is in this file; the accompanying
inspiration images show register and mood, not layouts to copy. Repo design authority, if deeper
context is ever needed: `_project-tracking/frontend/08-design-language.md` (visual language),
`_project-tracking/CONSTANTS.md` (product invariants), `_project-tracking/frontend/01–07`
(flows, state model, per-surface UX).

## 1. Product

Alexandria is a local-first desktop Digital Asset Manager for solo creative professionals —
photographers, designers, videographers. It indexes assets where they live (local drives,
external drives, NAS), never moves or owns files, works offline, no cloud, no subscription.
Positioning is **user respect and trust**: the engine detects and flags, never acts behind the
user's back; AI measures, the user judges. Desktop only (Wails: Go engine, React UI), macOS
first-class, Linux/Windows second. No mobile, no breakpoints.

The UI's job is to get out of the way of the assets. Pro-tool skeleton (filter bar, sidebar,
grid, inspector, status bar — muscle-memory conventional); identity comes from a few signature
concepts: the visible query, cull speed, the Review surface, and transparency-as-chrome.

## 2. Stance

**Precision instrument, selectively delighted.** Neutral, dense, mono-inflected chrome; the
delight budget is spent on a small set of icons and state-reporting microinteractions.
Non-software register: Teenage Engineering restraint; Apple Liquid Glass materiality, used
transiently, never persistently.

## 3. Hard rules (binding — the system must not violate these)

1. **Chrome owns no hue.** Color-critical work: the UI around images stays neutral grey in
   every theme. Hue appears only as (a) user data — color labels, tag colors, health states;
   (b) tiny fixed chips inside otherwise-monochrome icons; (c) rationed gradient moments (§8).
2. **No accent color by default.** Selection, focus, hover, highlight all read through neutral
   means (lightness steps, borders, inversion). The user MAY set an optional accent that tints
   these states; no state may *depend* on it. Design accent-optional, never accent-shaped.
3. **Canvas is photographic middle grey** (L\* = 50, `#777777`) — grid backdrop, loupe surround.
   **Panels sit darker** (L\* ~32–38, light text) so chrome reads as a receding frame and
   thumbnails pop. Text contrast lives on panels; never rely on text sitting on the canvas.
4. **Flat construction.** Borders and lightness steps define structure. No shadows, no
   elevation ramp.
5. **Blur is the layering material — transient only.** Command palette, modals, drawers,
   popovers differentiate by backdrop-blur + dim of what's behind. **Persistent chrome is
   always opaque** (translucency would leak asset color into the UI). Modal stacks dim/blur
   progressively.
6. **Motion is ambient and evidentiary** — it reports state, never garnishes (§8).
7. **Iconography is a custom dot-matrix/pixel-grid glyph language** (§7). No stock line-icon
   set (Lucide/Feather et al.).
8. **All display text is data**: every user-facing string is an i18n key; dates/numbers/sizes
   via `Intl`; enums map through display registries. Design with realistic-length strings, not
   short English.
9. **Layout speaks the hierarchy**: components are visually scoped to what they govern.
   Sidebars are full-height and hem in the center space; the **filter bar lives inside the
   center pane** (it operates on that view — never spans over the sidebars); integrated macOS
   title bar (traffic lights inline with chrome).
10. **Density is compact.** This UI respects screen space: small type (11–13px body), tight
    spacing, information-dense panels. Mono face for all data values, counts, and telemetry.

## 4. Color and themes

Decided values below are binding; everything else (ramp steps, exact borders, selection
lightness values, label hues) is yours to propose within the rules.

- **Middle theme (default, first-class):** canvas is exactly `#777777` (L\* = 50); panels in the
  L\* 32–38 range (≈ `#4b4b4b`–`#565656`) with near-white text; neutral selection treatment.
- **Light theme (second-class):** paper-and-ink register — warm neutral surfaces, ink text,
  tiny precise type. Same semantic tokens, overridden.
- **Dark theme (second-class):** near-black, for dim-room culling.
- **Data colors are identical across themes** — six labels (red, orange, yellow, green, blue,
  purple), danger/success/warning. They mean something; they never restyle per theme.
- Architecture: primitives → semantic tokens; components consume semantic tokens only; themes
  override the semantic layer.

## 5. Typography

- **Geist Sans** = UI face; **Geist Mono** = data face (metadata values, counts, file names,
  telemetry, status bar). Both OFL, bundled, variable. System stacks as fallback.
- **Geist Pixel** = sanctioned display/accent face, used *sparingly*: dev-corner headers,
  telegraphy moments, wordmark exploration. It is the dot-matrix motif as a font.
- Scale (compact): roughly 11 / 12 / 13 / 15 / 18 px — refine as needed, but keep the compact
  ceiling. Section-label voice: small caps + tracking.

## 6. Selection, cursor, focus (spec this carefully)

Vocabulary: the **cursor** is the single focused asset (always exists when results are
non-empty); the **selection** is an explicitly chosen subset (empty by default). They are
independent and must be visually distinct. Required:

- Four grid-cell states: rest · hover · **cursor** · **selected** · **selected + cursor**
  (cursor ring reads *through* selection).
- **Multi-selection and discontinuous selection blocks**: ranges read as ranges, gaps as gaps,
  at every tile density (huge → tiny).
- Acid test: **selection reads instantly on any image, in any theme, with or without a user
  accent.** Neutral toolkit: cell-ground lightness shift + border/inversion.
- Chrome keyboard-focus ring is neutral and visually distinct from asset selection.

## 7. Iconography — the dot-matrix glyph system

Custom pixel-grid glyphs, continuous with the status bar's block-character telegraphy
(`▁▃▆`, `◐`, heartbeat dot in the mono face).

- Monochrome by default; drawn on a strict pixel grid, crisp at target sizes (16/20/24px).
- A few **hero positions** (app icon, view-mode marks, source-type badges) carry the warmth
  budget: tiny colored dots or a soft gradient orb inside the glyph.
- Native transition: **dither/dissolve** — glyphs and empty-state graphics materialize as dots.
- Empty states are dot-matrix illustrations, not stock art.
- Logo direction (exploration): notched-star / sparkle / burst marks; dot-matrix wordmark.

## 8. Motion

- **Ambient layer** (always): numbers tick when values change; status glyphs morph;
  character-swap animation in the mono face (no SVG weight); watcher heartbeat pulses; hover
  reveals. Fast (100ms), interruptible. Nothing animates that isn't reporting something.
- **Sanctioned moments** (the only expressive motion + gradient use): cull key-feedback overlay
  (big transient "★3" / "REJECT"), import completion, first-run splash, new-feature reveal.
  Soft prismatic gradients on neutral ground; P3 welcome.
- `prefers-reduced-motion`: ambient layer degrades to instant state swaps.

## 9. Component inventory (design these, with all states)

**Shell**: full-height left sidebar (Sources | Collections | Tags
segmented modes, one reusable tree) · center pane containing the filter bar + active view +
its scrollbars · full-height right inspector · bottom status bar · integrated macOS title bar.
Panes drag-resize and collapse.

- **Filter bar + pills**: a pill = one query-AST leaf (macOS
  search-token style) — click to edit operator/value, × to remove; free-text field remainder;
  "save as smart collection" affordance; honest pending-state annotation on still-computing
  signal pills ("· 214 not yet scored"). States: rest, hover, editing (operator/value popover),
  invalid, pending-annotation.
- **Sidebar tree**: hierarchy, counts (mono), disclosure, selected-scope state, drag targets,
  offline-source and Review-count badges.
- **Grid cell**: thumbnail + optional badges (rating stars, label chip, flag, type badge,
  duration, Review corner-tick). All selection states (§6). Density slider range (tiny → huge).
- **Loupe / Compare / Cull**: same state, different renderers. Cull = fullscreen, lights-out,
  filmstrip, key-feedback overlay; filmstrip shows suggested-reject dimming.
- **Inspector**: metadata groups (mono values), triage controls (rating/flag/label/tags/note),
  "contained in" section. Adapts per asset type — no empty camera panels on audio.
- **Status bar**: left = query narrated in plain words; center =
  selection scope (hidden when empty); right = compact live job/health telegraphy in mono
  glyphs. Expands into the **activity drawer**: per-job progress rows, plain-language history,
  deepest tab = dev corner (queue depths, event feed — the easter egg).
- **Command palette**: the one launcher (actions, search mode via
  Cmd+F, settings entry). Backdrop-blurs the app. Fuzzy match, frecency, keybinding hints.
- **Task views** (full-window, enter-do-leave): Import (source pick → options → live pipeline
  progress → completion summary), Review (categorized queue rows — moves, duplicates, missing,
  XMP conflicts, import errors; bulk resolution is the norm), Settings, First-run (empty state
  + keybinding-preset picker).
- **Primitives**: buttons (neutral; no accent dependency), inputs, toggles, segmented controls,
  dropdown/popover, toast, confirm modals (destructive = double-confirm pattern), progress
  (bar + tiny inline mono progress glyphs), tooltip, empty states (dot-matrix), keybinding chip.

## 10. Anti-goals

- No forced accent; no state that only reads via accent color.
- No shadow/elevation stacks; no persistent translucent chrome.
- No hue in chrome; no decorative gradients outside the sanctioned moments.
- No stock line-icon set; the glyph language is custom or it isn't.
- No filter bar spanning over the sidebars; no motion that reports nothing.
- No dashboard-y home, no greetings, no mascots; density over whitespace theater.

## 11. What we want back

Full token architecture (primitives + semantic + three theme overrides) · component specs with
states per §9 · the selection system per §6 · the dot-matrix glyph set direction (a starter set
of ~24 glyphs + the hero treatment) · motion spec (ambient + sanctioned moments, durations,
reduced-motion) · the three gradient-moment treatments (first-run, import complete, feature
reveal).
