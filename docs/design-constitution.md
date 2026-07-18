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
  achromatic — OKLCH C = 0 **exactly** in the gray system (ratified 2026-07-12;
  a hint of chroma at panel scale reads as a color cast — a tinted gray would be
  a doctrine amendment, not a tuning choice). Needs only *consistency*, not
  color accuracy.
- **The stage** — the surface behind assets in loupe/compare/survey. Theme-
  **independent**, neutral, dark (default **PIN** ~OKLCH L 0.30–0.38), user-adjustable
  (photographers expect it; LrC precedent) with the range capped below the dead band
  (max L 0.45) so stage ink stays legible at every setting. Carries almost no text.

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
> (the shadow rung is reachable only by transient layers — §6)

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
  orthogonal to what it says. (Hero is not a new ink: it is the fill rung with
  the fun layer injected on top, §17.)

One unary operator: **disabled = chroma drained + one ink step down.** Read-only is
NOT disabled (§25).

### §5 The hue ledger

Every use of hue in chrome is a registered row. Adding color = adding a row = a
collision review. The ledger:

| Hue | Meaning | Where | Constraint |
|---|---|---|---|
| Label hues ×5 (LrC-compatible: red/yellow/green/blue/purple) | User judgment: color label | Cell slot, filter bar, inspector | Solid swatch style only; user-toggleable off (hidden option) — rows conditional |
| Tag palette ×12 (named scales; ratified 2026-07-13) | User meaning: tags | Chips in four recipes: tint / outline (border + wash) / dot / fill | Per-hue engineered steps × two worlds (no formula); solids world-independent with per-hue on-solid ink; users pick from palette only |
| Accent (user-picked from the named scales; default blue — ratified 2026-07-13) | Functional signal: focus ring, drop-target fill, toggles-on, links | Outline-first; drop-target is its only large fill | Every offered hue must pass ring contrast on all four themes (validator-gated picker); gray excluded; hue-sharing with a label is lawful — accent and labels are style-disjoint |
| Attention (user-picked from tag palette; default **PIN**) | "Needs a human decision" (§10) | Dot/outline (advisory) → tint/fill (blocking) | One hue for ALL attention states; identity via glyph + word, never hue |
| Error red | Invalid input (forms, chips) | Field hairline + message row | Conventional; independent of attention choice |
| Fun layer (§17) | Delight: an injected overlay, not core chrome | Registered sites only (the `funSites` registry; admission by the wit rule, §17) | One per view, area-capped, never adjacent to stage/cells |

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
occlusion shadow + hairline + the roundest radius rung (§24). Transients sit on a
dedicated surface role: equal to panel on light themes, a raised step above panel on
dark themes — shadow alone does not read against dark chrome. One fixed-dark
exception: **tooltips and keyboard hints** — labels, not surfaces (genre convention;
every tooltip in the reference set).

### §7 Register shifts (interaction states)

Interaction states are luminance steps within a surface family. The invariant behind
direction: **interaction moves toward ink** — fills darken on light themes, lighten
on dark. The `direction: recess | raise` field per family × theme in the token
source is derived from this invariant plus one declared exception, never chosen
freely:

- **Chrome families follow the invariant** (hover/selected fills darken on light
  themes, lighten on dark) — the reference-set grammar: a field you can type in is a
  slightly darker well; a hovered row darkens; an open dropdown gains a border.
- **The asset-cell family raises** toward white on every theme (the declared
  exception; LrC matting): a selected cell must never put a dark surround beside a
  photograph; lighter mats are the print convention.

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
- **Row density is an intent, not a number** (ratified 2026-07-13; values closed
  same date): `row-control` 28 (controls breathe) · `row-list` 24 (interactive
  dense — the tree; sits ON the hit-target floor) · `row-text` 16 (read-only
  metadata — the instrument voice: precision measurements, not UI copy). Density
  and type step down TOGETHER: `row-text` pairs with the small roles (label-sm/
  data-sm) — body type in a 16px row is zero-slack and banned; the pairing is a
  registered grammar (registries `rowIntents`), validator-lintable. One exception
  to the 24px hit-target floor: full-width *read-only* rows may pack below it;
  anything interactive keeps 24. Density is vertical only — insets are identical
  across intents — and switches at section boundaries, never row-by-row. Where we
  cramp and where we breathe is a first-class design decision, taken deliberately
  per surface.
- **The register-step quantum:** one perceptual step of ΔL, tokenized per world
  (light 0.018, dark ~2×). State deltas are authored and adjusted in multiples of
  it — OKLCH's perceptually uniform L is what makes "one step" mean the same thing
  everywhere on the ramp.
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

Rules: hierarchy by **weight and ink, never size** within a surface; body UI sizes
live in the 10–13px band, plus a **display tier** (16/24 and 20/28, semibold —
ratified 2026-07-13) reserved for view-level chrome: the module picker, view page
headings — never body UI; tabular numerals everywhere numbers align;
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
  twice-converted pixel. (The engine-side color-space work sits in the tracking queue.)
- Color-critical *editing* is out of scope forever: "Open in external editor" is the
  escape hatch. No soft-proofing, no render intents.
- Grain, prism, dither, and the fun layer never sit adjacent to the stage or
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
- Media (extended 2026-07-13): generative dot/halftone art; **public-domain
  classical art passed through the dot screen** (dithered/halftone — the reference
  must be earned by the moment, §17's wit rule: the handoff hands at handoff time);
  **interactive dot art** (voids that respond to the cursor — alive, not animated);
  **illustrative modern skeuomorphism** (the archive's physical hardware — drives,
  discs, circuit boards — as portraits inside joy sites and About/Storage, never as
  interactive chrome); Instrument Serif italic; Geist Pixel.
  **Never the user's own photos** (custody: dithering their work is modifying it).
- Empty states are invitations to act (§18), with the art as setting, not message.
- Everything respects `prefers-reduced-motion` by construction.

### §17 The fun layer (ratified 2026-07-12; supersedes "the light register")

The prismatic thread from the reference set — moving polychrome gradient, glow, and
frosted glass (prototype: `GlassFlowButton`) — is an **add-on overlay category, not
a core part of the system**. It is injected selectively at registered sites for
delight; chrome works identically with the layer stripped. Its ledger row (§5) is
the site registry, which closes the hue-accounting question: fun is one budgeted
row, not a per-effect debate.

**The wit rule (ratified 2026-07-13) — the admission test for every fun moment:
the moment must reference its own meaning.** The art knows what is happening: the
lighthouse sweeps while the app searches; the handoff painting appears at handoff
time; scramble text runs while something indexes. Decoration that doesn't know
what's happening — a gradient because gradients are nice — fails the gate
regardless of beauty. Because moments are tied to real events, reality itself
rations them: the UI can never become a grab bag of effects.

- **Registered sites** (the `funSites` registry rows; populated 2026-07-13):
  - *boot-splash* — the flagship: the Pharos greeting whose beam sweep IS the
    progress indicator (the lighthouse searching for your files). **The art never
    costs a millisecond**: duration = actual load, a fast catalog gets a glimpse,
    instant = skipped. Designed for the 500th launch, not the first; the full
    theatrical form belongs to naturally-slow moments (first-run, cold start).
  - *hero-cta* — the primary call-to-action (the hero rung = fill + fun overlay,
    §4), **including its press payoff**: a one-shot bloom that hands off visibly
    to the marquee. One firework at the launch pad, not sparklers on every
    switch; ordinary chrome stays dry.
  - *marquee-progress* — the one long job the user is actively awaiting. Dots
    remain the *ambient* progress voice; the light-bar is the marquee only — one
    sentence, two progress languages prevented.
  - *announcement-sweep* — "something new exists here": one transient lap of
    gradient around the thing's outline, then settled and gone. The fun layer's
    answer to a notification — runs once, never loops, never nags.
  - *slide-commit* — deliberate commitment rendered physically: the glass tray +
    puck slid to start a heavyweight or irreversible operation. Safety pattern
    and fun moment in one body; reserved for actions whose weight warrants
    friction.
  - *joy-moment* — the §16 voids (empty states, first-run, completion, about).
- One per view, area-capped, never adjacent to stage or cells (§11), static under
  reduced motion. Glass is lawful only here and on transients ("glass = transient or
  fun").
- Tokens: the palette-indexed gradient, flow duration/easing, glass face recipe
  (blur/saturate/face-alpha/inset highlights), glow halo (**PIN** values — the
  fun-layer probe page is the pinning instrument).

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
3. **No component tier yet; combination names never** (amended 2026-07-12 from a
   flat ban). Tokens name axes and operators — a token naming a combination
   (`button-tinted-hover-bg`) is a defect, because the axis algebra (§4) already
   expresses it. A component tier (per-component override tokens that alias
   semantic roles) is not banned on principle: it is the override API a design
   system grows the day it ships to consumers outside this app, and adding it
   later is purely additive. Until that day, components consume semantic roles
   directly. Capability tables cap the state matrix per element.

**Pipeline (C15 applied to design):** one declarative token source (W3C DTCG-format
JSON) compiled by our generator into CSS custom properties, TypeScript types, the
docs table, and the contract validator. Hand-written parallel definitions are a
defect. The source is `frontend/design/tokens.resolver.json` + `tokens/`
(primitives / semantic / per-theme bindings, themes as resolver modifier contexts),
with `contracts.json` (validator input) and `registries.json` (dispatch tables)
beside it — never inside it.

### §23 Contracts and enforcement

The validator runs in `make check` and fails when the system contradicts its own
reasoning — internal consistency first, usage-linting second:

- Every declared ink/surface pair meets its promised APCA Lc — re-checked per
  theme, per family, and for the stage across its full adjustable range, not just
  the default. Targets are per-POLARITY since 2026-07-17 (dark worlds run lower:
  APCA scores reverse polarity down and light-on-dark blooms); `contracts.json`
  is the numeric authority.
- Register families are monotonic in their declared direction; **every adjacent
  step in every family** sits within the declared band — above the perceivability
  floor AND below the delta cap. (A step below threshold is a state users cannot
  see; spot-checking two pairs proved insufficient.)
- Every spacing/size value is a quantum multiple; radius/z/shadow values come from
  registered rungs only.
- The hue ledger is the exhaustive list of chroma in chrome; the attention hue keeps
  a minimum hue distance (**PIN**, working hypothesis ≥ 30° OKLCH) from every
  enabled label hue — inequality alone passes hues that are indistinguishable at
  swatch scale.
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
- **Selected — the promote rule** (adjudicated 2026-07-17): on the strengthened
  selected fill (+2 register steps), content ink **promotes one step** — ink.2
  content renders ink.1, ink.3 renders ink.2 — so findability keeps its fill and
  text keeps its contrast. The selected-text contract checks the promoted pairs.

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
the dot voice, the fun layer, and the two-world luminance architecture — never
on re-inventing mechanics users already know.

### §28 Platform honesty list (open empirical checks)

- Geist at 11px on Windows ClearType at 1× — verify before pinning the type floor.
- WKWebView (macOS) vs WebView2 (Windows) divergence: everything tagged sRGB behaves
  identically; exotic content is where they fork — another argument for §11's
  conservative canonical space.
- P3 display headroom for the tag palette and fun layer: optional refinement,
  OKLCH makes it expressible when wanted.

### §29 Scope ratifications (decided, dated 2026-07-12)

- Single density mode. Whole-app zoom out. px not rem. One bundled font family
  cross-platform. Panels collapsible + resizable. Soft-proofing never (until real
  users beg). Dark mode = the Graphite/Carbon themes, not a separate system.
  Labels user-toggleable. Attention hue user-picked from the palette. Tooltips
  fixed-dark (the one polarity exception). User photos never used as joy content.
- Reviewed against the Figma-ish reference set (2026-07-12) and deliberately NOT
  adopted from it: uppercase micro-labels (the references' column heads; sentence
  case stays law, §9) and fixed-dark popovers on light chrome (tooltips remain the
  only polarity exception, §6). The fun layer re-scoped as an injected add-on, not
  core chrome (§17).
- Ratified 2026-07-13: accent user-selectable from the named scales (blue default,
  ring-contract-gated picker). One gray scale in the tag palette (no shade family);
  gray excluded from accent and attention. Selected register = +2 steps (findable
  at a glance). Placeholder ink = ink.4 (two steps below its label). Row intents
  `row-control`/`row-text` (§8). Register-step quantum tokenized per world.
- Ratified 2026-07-13 (the art-and-joy round): the wit rule as §17's admission
  test. The fun-site registry populated: boot-splash (the Pharos, chosen as the
  mark; art never costs a millisecond), hero-cta press payoff, announcement-sweep,
  slide-commit, marquee light-bar. §16 media extended: dithered public-domain
  classical art, interactive dot art, illustrative modern skeuomorphism (never
  interactive chrome). Decorated/hued icons deferred to the iconography round
  carrying the hypothesis: display-scale choice moments only, never working rows.
- Ratified 2026-07-17 (the Phase C adjudication — the validator's first machine
  pass over the palette round; D31): text targets are per-polarity (the 07-13 dark
  ink retune stands over the pre-retune hypothesis; the raised transient is dark's
  binding surface); the §25 selected promote rule (linen's selected-primary at
  Lc 70 pending its probe); the hue scales gain a world-varying **ring step**
  (light world: the solid; dark world: L 0.74 C 0.12 starting symmetry, PIN —
  eye-tune in the token-gaps round) because world-independent solids measure only
  Lc 20–43 on dark panels — unsatisfiable for any focus ring. Emitted-name
  contract: strict path mirror (`--alx-` + token path, dots → hyphens), one unit
  class per type role carrying its paired ink; `type-scale` composites are not
  emitted (roles supersede them); accent/attention carry their bound hue's
  on-solid and ring pairings (`--alx-accent-on` / `--alx-accent-ring`).

---

## §30 The pinning worksheet (all that remains)

Open values, each constrained by written contracts and closed by running the
validator + the 1× eyeball test on the Library-view mock (with the
`Alexandria Photos Slice` as content):

1. *(closed 2026-07-13 — the accent is a user-selectable binding over the named
   scales, blue default; ring contract gates the picker; label hue-sharing ratified
   lawful via style-disjointness)*
2. Surface family values: 5 steps × (chrome families + cell family) × 4 themes +
   dark-world duals; stage default + adjustment range (max capped at L 0.45, §1);
   the transient surface role (raised above panel on dark themes, §6).
3. Ink ramp: 4 steps + hairline, per world, APCA-verified at the type floor.
4. Type roles: committed 2026-07-13 (eleven roles incl. label-sm, two working
   sizes + micro 10px + title 13px, placeholder = ink.4). Verified with real Geist
   same date (library/type-probe.html): matched pair holds, no mono compensation;
   head = semibold 600. Still open: the Windows ClearType 1× check (10–12px).
5. Quanta: control heights, row heights per surface, panel default/min widths.
6. Radius (3), shadows (2 recipes), motion tokens (durations + easings).
7. *(closed 2026-07-13 — the twelve named hue scales, labels-as-solids, attention =
   magenta, error = red scale all live in the token source; survives only as
   validator re-checks)*
8. Fun-layer recipe values (the scenario/site registry closed 2026-07-13 — the
   wit rule and six sites are law; the RECIPES are the open half:
   gradient palette-indexing, glass, glow halo — `library/fun-probe.html` is the
   pinning instrument); dot-matrix loader mapping table (domain state → loader).
9. Icon stroke constant; focus-ring width/offset.

Empirical gates before values freeze: the white-vs-dark well A/B on the mock; the
Windows 11px check; the active-cell threshold test (§20's redundant encoding
verified at arm's length).
