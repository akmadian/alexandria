# The Alexandria Design Constitution

The design authority for the frontend rebuild. Everything visual traces to a rule here;
anything visual that can't is drift. Subordinate to `docs/CONSTANTS.md` and
`docs/decisions.md` (which win every conflict); absorbs and supersedes the prototype's
`DESIGN-DOCTRINE.md`. Implementation mechanics stay in `frontend/CLAUDE.md`.

**The root idea:** Lightroom Classic's structure and density, rendered in the modern
flat grammar of the reference set (`~/Desktop/design round 2/`), on light chrome that
never compromises text rendering, around content whose pixels we custody but never
touch. One signature aesthetic: the dot-matrix voice. Sections are numbered for
citation (§n). Values marked **PIN** are decided-in-shape but not yet pinned to
numbers; the pinning worksheet is §30.

---

## Part I — Light and color

### §1 Two luminance worlds

The app is two worlds with different physics:

- **Chrome** — every panel, rail, bar, and the grid well. Theme-following,
  achromatic (OKLCH C ≈ 0), needs only *consistency*, not color accuracy.
- **The stage** — the surface behind assets in loupe/compare/survey. Theme-
  **independent**, neutral, dark (default **PIN** ~OKLCH L 0.30–0.38), user-adjustable
  (photographers expect it; LrC precedent). Carries almost no text.

Text never sits in the dead band (~L 0.45–0.65): no theme places chrome there (§21),
and the stage carries only minimal, small, light ink. This is the resolution of the
middle-gray problem: it stops being a typography problem because it is only ever a
photo surround.

### §2 The cliff rule

Large luminance jumps are legible only as figure/ground — one object on a field (the
loupe). Adjacent *docked* surfaces stay within small deltas of one family (register
steps, §7). A dense grid is never surrounded by a cliff; the grid well is part of the
chrome family. Grid = contact sheet on paper; loupe = one photo in a dim room.

### §3 The contrast budget (the separation ladder)

Every element gets contrast proportional to its information content. Content ink is
strong; labels are muted; *separation* (hairlines, fills, register deltas) sits just
above perception threshold. Separate by the cheapest sufficient means, always the
highest rung that works:

> alignment → whitespace → value/fill → hairline → enclosure → shadow

Cheap separation funds generous rhythm: dense and calm are cause and effect. Bevels,
borders-on-borders, and decorative shadow are contrast spent on non-information —
defects by definition.

### §4 The axis algebra

Three orthogonal channels, each owning one job, for every element in the app:

- **Hue = meaning.** Which semantic system is speaking (§5). Chrome is otherwise
  achromatic; machinery is achromatic *by law*.
- **Lightness = interaction state.** Hover/press/select shift L only — never hue,
  never chroma. Direction is per-family (§7).
- **Style rung = prominence.** The escalating ink ladder:
  `dot → ghost → outline → tint → fill → hero`. How loudly something speaks,
  orthogonal to what it says.

One unary operator: **disabled = chroma drained + one ink step down.** Read-only is
NOT disabled (§25).

### §5 The hue ledger

Every use of hue in chrome is a registered row. Adding color = adding a row = a
collision review. The ledger:

| Hue | Meaning | Where | Constraint |
|---|---|---|---|
| Label hues ×5 (LrC-compatible: red/yellow/green/blue/purple) | User judgment: color label | Cell slot, filter bar, inspector | Solid swatch style only; user-toggleable off (hidden option) — rows conditional |
| Tag palette ×~12 **PIN** | User meaning: tags | Tinted chips with text | Fixed L/C, engineered light+dark variants; users pick from palette only |
| Accent **PIN** | Functional signal: focus ring, drop-target fill, toggles-on, links | Outline-first; drop-target is its only large fill | Must pass ring contrast on all themes (§21); not user-swappable |
| Attention (user-picked from tag palette; default **PIN**) | "Needs a human decision" (§10) | Dot/outline (advisory) → tint/fill (blocking) | One hue for ALL attention states; identity via glyph + word, never hue |
| Error red | Invalid input (forms, chips) | Field hairline + message row | Conventional; independent of attention choice |
| Polychrome light | Hero register (§17) | Primary CTA, marquee progress | One per view, area-capped, never adjacent to stage/cells |

Hue-as-state is budgeted, not banned; small, critically evaluated doses enter as rows.

### §6 The occlusion axiom

**Shadow marks occlusion — one surface passing under another — never height.**
Exactly two lawful shadows:

1. A transient layer occluding the page (popover, menu, dialog, toast, drag preview).
2. A clipping edge occluding scrolled content — the **tunnel**: content slides under
   flat chrome, which casts a short, soft shadow inward onto it. Scroll-linked
   (present only when clipped on that edge), a few px, never cast onto chrome.

Docked chrome is flat: seams (one hairline) + register steps, no shadow, ever.
Transients are **theme-following** (light popovers on light chrome), distinguished by
occlusion shadow + hairline + the roundest radius rung (§24). One fixed-dark
exception: **tooltips and keyboard hints** — labels, not surfaces (genre convention;
every tooltip in the reference set).

### §7 Register shifts (interaction states)

Interaction states are luminance steps within a surface family. Each family declares
one field in the token source — `direction: recess | raise`:

- **Chrome families recess** (hover/selected fills darken on light themes) — the
  reference-set grammar: a field you can type in is a slightly darker well; a hovered
  row darkens; an open dropdown gains a border.
- **The asset-cell family raises** toward white (LrC matting): a selected cell must
  never put a dark surround beside a photograph; lighter mats are the print
  convention.

The validator checks per-family monotonicity in the declared direction. Discrete
pinned steps, not alpha overlays (checkable contracts beat computed composites).

---

## Part II — Space, type, structure

### §8 Density mechanics

- **Vertical tight, horizontal generous.** Space lives *inside* rows (identical,
  generous side insets; text never touches an edge), not between blocks. Gutters
  between assets are waste; padding inside cells is structure.
- **One quantum: 4px.** Every spacing value is a multiple; the validator enforces.
- **Two control heights** in chrome (**PIN**, hypothesis 24/28px), one icon size
  (16px) in ≥24px hit targets, uniform row heights per surface. Ragged heights read
  cramped at any density; uniform rhythm reads calm at almost any density.
- **px-locked, no rem.** Pro-tool convention: chrome does not scale with OS font
  settings. App zoom is out of scope for v1; content zoom (loupe) is a view feature.
- **The 1× floor:** if a hairline and the smallest text are crisp on a 1080p 1×
  display, everything above is free. Sub-pixel (0.5px) refinements are 2×-only
  garnish, never load-bearing.

### §9 Typography

The cast (all bundled, identical on every platform — per-platform fonts rejected:
the density system is calibrated to one set of metrics; SF Pro is additionally
license-ineligible off Apple platforms):

- **Geist Sans** — the UI voice. Every label, name, control.
- **Geist Mono** — the data voice: metadata values, counts, filenames, EXIF. The
  deciding requirement was the matched pair: shared skeleton and vertical metrics,
  so mixed sans/mono rows never jitter. Mono *is* hierarchy: data, not chrome.
- **Instrument Serif Italic** — the joy voice (§16 only). Banned from working chrome.
- **Geist Pixel** — the dot voice in type. Rarest register (§16).

Rules: hierarchy by **weight and ink, never size** within a surface; UI sizes live in
the 11–13px band (**PIN** exact scale); tabular numerals everywhere numbers align;
sentence case; no uppercase micro-labels. Section heads are sentence-case bold at
body size (the reference-set move). Windows 11px rasterization: verification pending
(§28).

### §10 Domain states and the attention channel

States grouped by author, systematized by the axis algebra (§4):

- **Judgment adornments** (user-authored, persistent): rating, flag, label, tags,
  note-present. Hue-bearing per the ledger; live in reserved cell slots (§20).
- **Machinery status** (system-observed, transient): queued/ingesting, thumbnail
  generating, preview building, sync pending, scanning, import running. Achromatic,
  rendered in the dot-matrix voice (§14), and **normal is silent** — machinery is
  visible only while working; completion dissolves to nothing.
- **Attention states** (system-detected, need a human): missing file, pending review
  (detect-and-flag), failed ingest, volume offline, metadata conflict. One attention
  hue (§5); severity = style rung (advisory dot/outline, blocking tint/fill);
  identity = glyph + word. If nothing glows, nothing needs you.
- **Interaction states** (ephemeral, any element): §25.

### §11 Content custody (assets are sacred)

- **Fit, never crop.** Composition is content; thumbnails letterbox inside the cell.
- The UI never touches asset pixels: no CSS filters, blends, opacity, or tinted
  overlays on content. Selection shades the *cell mat*, never the photo.
- Every derived raster the engine emits is color-managed at decode (honor embedded
  ICC) and baked to one canonical space — **sRGB for v1**; untagged = assumed sRGB;
  P3 re-bake later is a registered rebuild, not a migration. Monitor calibration is
  the OS's job once content is tagged; ours is never to emit an untagged or
  twice-converted pixel. (Backend residue: `_project-tracking/ideation/color-space-observation.md`.)
- Color-critical *editing* is out of scope forever: "Open in external editor" is the
  escape hatch. No soft-proofing, no render intents.
- Grain, prism, dither, and the light register never sit adjacent to the stage or
  cells.

### §12 Structure (the zones)

LrC's skeleton, kept deliberately:

- **Left rail** — sources: one tree at a time under tabs (Folders / Collections /
  Tags), icons + indent guides + muted right-aligned counts.
- **Center** — the well (grid on chrome) or the stage (loupe).
- **Right rail** — inspector: histogram/preview, judgment controls, metadata rows.
- **Top** — the filter bar: a *place*, not a popover. Always-visible attribute
  toggles + chip grammar (`field:op:value ×`, "+ Add filter", "N more" overflow).
  Chips are a rendering of `ast.Query` — never parallel filter state.
- **Bottom** — filmstrip (persistent selection context across views) + status
  readout (§19).

Panels: collapsible AND resizable (v1). Every panel is the one row grammar: label
left, value/count right (mono, muted), uniform height, disclosure chevrons.

### §13 Truncation, counts, overflow

- Names end-truncate with ellipsis; **filenames middle-truncate** (info lives at both
  ends; Finder convention). Hover reveals the full string. Follow platform
  convention everywhere; deviation is friction.
- Counts: exact to 4 digits, abbreviated beyond (`20.3k`), exact on hover, always
  tabular. Tree counts are scent; inspector/status counts are answers (always exact).
- Overflow is signaled by the tunnel (§6), never by fades on chrome or decorative
  gradients. Scrollbars: overlay, thin, appear on scroll.

---

## Part III — Components

### §14 Iconography

Two families, two jobs, never mixed:

- **Structural** (things you can do): Lucide-sourced, stroke-based, one stroke
  constant (**PIN** ~1.5px logical at 16px), one chrome size (16px; 20px only in
  toolbar hero spots). Icons are ink — they ride the ink ramp and interaction
  states like text; never bring their own color (ledger only). **Fill = on**
  (flag picked, star rated); state never by color alone. Every icon is a registered
  concept beside the vocabulary: same glyph = same meaning, one glyph per concept.
- **Machinery** (things happening): the 5×5 dot-matrix family, sourced from
  dot-matrix-animations.vercel.app (60 loaders; spinner/progress/ambient/agent/
  status ≈ our machinery taxonomy). Appear only while working (§10). Dots vs
  strokes = happening vs actionable — the observation/judgment split, drawn.

### §15 The selection model

Four concepts, four distinct looks: **hover** (pointer preview, means nothing),
**focus** (where keys land; accent ring, `focus-visible` only), **selection** (the
set verbs act on), **active** (exactly one member: inspector subject, loupe photo,
range anchor).

- **Verbs act on the selection; navigation moves the active.** One rule, every view,
  no modes — plain navigation *collapses* selection to the new item, so solo triage
  (arrow, rate, arrow, rate) is always singleton-safe, and a multi-selection exists
  only by deliberate construction (batch intent). This deliberately refuses LrC's
  mode-dependent apply semantics.
- Platform conventions wholesale: click/arrow = select-and-collapse; shift = range
  from active; cmd = toggle; cmd+arrow = move focus only, space = toggle at focus;
  esc/well-click = clear. Empty selection is legal (inspector shows source summary).
- **One asset selection**, shared by grid/filmstrip/loupe (three renderings of one
  object). The source tree is a separate scope. Selection in an unfocused pane drops
  one register step.
- **Selection ⊆ visible result set.** Query changes prune selection; a verb can
  never strike an asset the user cannot see. Pruning is readout-visible.
- **Mixed values** are first-class: inspector fields that vary across the selection
  show the mixed state (em-dash); editing applies to all, with fan-out feedback.
- **The readout** (status bar): `1,204 · 3 selected · _DSF4926.RAF`, plus transient
  confirmation on fan-out (`★★★ → 3 assets`). The model is always observable.

### §16 The joy register

Signature aesthetic: **the dot-matrix voice** — halftone, dither, grain; images made
of dots; the photographic print vernacular. Rules:

- Joy lives in **the voids**: empty states, first-run, long-operation moments,
  completion, about. Never on the working surface mid-task; one joy moment at a time.
- Media: generative dot/halftone art, Instrument Serif italic, Geist Pixel.
  **Never the user's own photos** (custody: dithering their work is modifying it).
- Empty states are invitations to act (§18), with the art as setting, not message.
- Everything respects `prefers-reduced-motion` by construction.

### §17 The light register (hero)

The prismatic thread from the reference set: a moving polychrome gradient refracting
through frosted glass (prototype: `GlassFlowButton`). The top prominence rung.

- **Permitted meanings:** primary call-to-action; marquee progress (the one long job
  the user is actively awaiting). Dots remain the *ambient* progress voice; the
  light-bar is the marquee only — one sentence, two progress languages prevented.
- One per view, area-capped, never adjacent to stage or cells (§11), static under
  reduced motion. Glass is lawful only here and on transients ("glass = transient or
  hero").
- Tokens: the palette-indexed gradient, flow duration/easing, glass face recipe
  (blur/saturate/face-alpha/inset highlights), glow halo (**PIN** values).

### §18 Writing

Words are design material. User-side vocabulary only (people manage *tags*, not
*junction rows*); active voice; a control names its exact effect ("Save changes");
an action keeps one name through the whole flow; sentence case; plain verbs. Errors
say what happened and how to fix it, never apologize, never vague. Empty screens are
invitations to act. **UI nouns come from the generated vocabulary (C15)** — the
filter bar's field names are `ast` vocabulary field names; consistency is an artifact
of the schema compiler, not discipline. The icon registry (§14) extends the same rule
to pictures.

---

## Part IV — System machinery

### §19 The grid cell

Locked aspect-ratio cell, interior padding (photos never touch each other or the cell
edge), 1px inner hairline on the thumbnail (~6–8% ink; invisible until a high-key
image needs containment). Reserved slots: index (corner), rating, flag, label swatch,
type badge, machinery dot. Fixed slots are what make a grid scannable rather than
merely viewable.

**Every asset type declares its cell face** — a capability row in the `assettype`
registry; the frontend renders the declared contract, never special-cases:

1. **Captured faces** (photo, video poster, document page): content pixels — full
   custody rules (§11).
2. **Generated faces** (font → live specimen; text/code → opening lines in mono;
   audio → waveform): authored by us = chrome; rendered on a substrate token,
   ink-ramp rules, no custody constraints.
3. **Glyph faces** (unpreviewable): the type's icon, large, on substrate — dot-matrix
   treatment so fallback reads intentional.

### §20 Cell states

| State | Encoding |
|---|---|
| well | family base (darkest; the family *raises*, §7) |
| rest | +1 step |
| hover | +1 more, transient |
| selected | +1 more |
| active | family ceiling (the reserved near-white) + 1px ink hairline frame |
| focus (keyboard) | accent ring, outline only, `focus-visible` |
| unfocused-pane selection | its step −1 |
| drop-target | accent tint fill (accent's only large fill) |
| attention | per §10 (glyph + attention hue at the declared rung) |

Active is redundantly encoded (ceiling + frame): one register step judged through a
thin mat around a busy thumbnail is at threshold; the frame removes doubt without
spending hue. Dark-world duals of all rows exist for the dimmed loupe context
(**PIN**).

### §21 Themes

A theme is a re-assignment of L values to the same roles, shipped only if it passes
every contract. The family (**PIN** values): **Paper** (~0.97, default), **Linen**
(~0.90), **Graphite** (~0.30), **Carbon** (~0.20). No theme in the dead band — that's
not losing the UX battle, that's refusing to ship the setting that made LrC's text
bad. The stage is theme-independent (§1). Honest cost note: each theme requires
real per-family tuning and re-validation — structurally cheap, design-non-free.
Density modes: deferred; if ever wanted, a quantum re-assignment through the same
validator (same mechanism, no redesign).

### §22 Token architecture

Three tiers, industry-standard shape:

1. **Primitives** — raw scales: the gray ramps, tag/label hue scales, spacing quanta,
   duration/easing values. Open Props is a *quarry* for primitives where good
   (spacing scale, easings) — never the semantic layer, never rem-based values,
   never its shadows/type scale.
2. **Semantic roles** — where all doctrine lives: surfaces (per family, with
   `direction`), inks, stage, accent/attention/error, radius rungs, z-layers,
   motion tokens. Components consume roles, exclusively.
3. **No hand-authored component tier.** Tokens name axes and operators, never
   combinations (`button-tinted-hover-bg` must not exist). Exceptions require a
   doctrine amendment, not a one-off value. Capability tables cap the state matrix
   per element.

**Pipeline (C15 applied to design):** one declarative token source (W3C DTCG-format
JSON) compiled by our generator into CSS custom properties, TypeScript types, the
docs table, and the contract validator. Hand-written parallel definitions are a
defect.

### §23 Contracts and enforcement

The validator runs in `make check` and fails when the system contradicts its own
reasoning — internal consistency first, usage-linting second:

- Every declared ink/surface pair meets its promised APCA Lc (**PIN** targets;
  working hypothesis: primary ink ≥ Lc 75, secondary ≥ Lc 60, hairlines Lc 10–20,
  at the smallest declared text size) — re-checked per theme, per family.
- Register families are monotonic in their declared direction; steps within family
  ≤ the declared delta cap.
- Every spacing/size value is a quantum multiple; radius/z/shadow values come from
  registered rungs only.
- The hue ledger is the exhaustive list of chroma in chrome; the attention hue may
  not equal an enabled label hue.
- Usage layer: no raw color/size literals in components (stylelint); tokens only.

### §24 Registered scales (shape decided, values PIN)

- **Radius by detachment:** docked = 0 (seams); controls = small; transients =
  large. Three values total.
- **Z-order registry:** `docked < tunnel content < small transient (tooltip/menu/
  popover) < dialog/sheet < toast`. Each layer's treatment (shadow recipe, radius
  rung, polarity) is a row here; there is no other elevation.
- **Shadows:** exactly two recipes — occlusion (transients) and tunnel (§6).
- **Motion tokens:** durations (~3 steps in the 80–250ms band) + easings (ease-out
  family; the reference set's `cubic-bezier(0.16, 1, 0.3, 1)` is the working
  default) as tokens like colors.

### §25 Interaction-state registry (per element, capability-capped)

`rest · hover · pressed · selected · active · focus-visible · disabled ·
read-only · drop-target · invalid · mixed`

- **Disabled** = chroma drained + ink step down (can't act, nothing behind it).
- **Read-only** = normal inks + lock glyph + suppressed verbs (real things you
  temporarily can't act on — offline volume's assets are not "disabled").
- **Invalid** = error-red hairline + message row in ink (§5); never color alone.
- **Mixed** = em-dash state on any value component (§15).

### §26 Motion doctrine

Motion exists to answer *"what moved where"* when a state change would otherwise be
teleportation. Three laws:

1. **Identity is continuous.** The object the user cares about persists across every
   transition; the world rearranges around it. Grid→loupe: the clicked thumbnail
   itself travels and scales to the stage while chrome dims and recedes — nothing
   cuts. (The room adapting to the task is UX, not flair.)
2. **Register shifts are state readouts, not animations** — instant to ~80ms. Hover
   fills may *slide* between adjacent siblings (the highlight is one physical object
   moving). Transients grow from their anchor, fast.
3. **Only working machinery loops.** Chrome moves in 100–250ms ease-out; durations
   and easings are tokens; every animation ships its `prefers-reduced-motion` form
   by construction.

Text-scramble and other flavor effects are joy-register only, never working chrome.

### §27 Selection of platform conventions

Where a platform convention exists (selection bindings §15, truncation §13, tooltip
polarity §6, scrollbars §13), adopt it unmodified. Our originality budget is spent on
the dot voice, the light register, and the two-world luminance architecture — never
on re-inventing mechanics users already know.

### §28 Platform honesty list (open empirical checks)

- Geist at 11px on Windows ClearType at 1× — verify before pinning the type floor.
- WKWebView (macOS) vs WebView2 (Windows) divergence: everything tagged sRGB behaves
  identically; exotic content is where they fork — another argument for §11's
  conservative canonical space.
- P3 display headroom for the tag palette and light register: optional refinement,
  OKLCH makes it expressible when wanted.

### §29 Scope ratifications (decided, dated 2026-07-12)

- Single density mode. Whole-app zoom out. px not rem. One bundled font family
  cross-platform. Panels collapsible + resizable. Soft-proofing never (until real
  users beg). Dark mode = the Graphite/Carbon themes, not a separate system.
  Labels user-toggleable. Attention hue user-picked from the palette. Tooltips
  fixed-dark (the one polarity exception). User photos never used as joy content.

---

## §30 The pinning worksheet (all that remains)

Open values, each constrained by written contracts and closed by running the
validator + the 1× eyeball test on the Library-view mock (with the
`Alexandria Photos Slice` as content):

1. **The accent hue** — the only genuinely open *choice*. Criteria: passes ring
   contrast on all four themes; not confusable with a label hue at swatch scale;
   distinct from the attention default; P3 headroom welcome.
2. Surface family values: 5 steps × (chrome families + cell family) × 4 themes +
   dark-world duals; stage default + adjustment range.
3. Ink ramp: 4 steps + hairline, per world, APCA-verified at the type floor.
4. Type scale: exact sizes/weights/line-heights (sans + mono) in the 11–13px band.
5. Quanta: control heights, row heights per surface, panel default/min widths.
6. Radius (3), shadows (2 recipes), motion tokens (durations + easings).
7. Tag palette (~12 hues × 2 worlds), label hue values (LrC-compatible), attention
   default, error red.
8. Light-register recipe values; dot-matrix loader mapping table (domain state →
   loader).
9. Icon stroke constant; focus-ring width/offset.

Empirical gates before values freeze: the white-vs-dark well A/B on the mock; the
Windows 11px check; the active-cell threshold test (§20's redundant encoding
verified at arm's length).
