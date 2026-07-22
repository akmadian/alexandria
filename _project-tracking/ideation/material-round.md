# The material round — amendment drafts (NOT ratified)

2026-07-22. Companion to `frontend/design/library/material-probe.html` (v2, tonal).
Diagnosis: the reference look is flat-but-dimensional via TONAL means — pole anchoring,
two-grade lines, fills-as-objects, a steeper ink ladder — not via shadows/gradients
(the round-1 neumorphic candidate was vetoed). Measured values and sources live in the
probe header and the session plan. Everything below is a DRAFT awaiting Ari's eye against
the probe; on ratification these become constitution amendments + token edits, and this
file is deleted (D27).

## Draft A — §7 addendum: pole anchoring

Each world's surface family is anchored at its pole: the panel surface sits at (or a
declared whisper off) the world's luminance pole — light worlds near L 1.0, dark worlds
near their floor — and every other chrome value is spread AWAY from the anchor: canvas
and wells below panel (light) / below panel toward the floor (dark); interaction fills
2–8% off the anchor. Content ink deepens toward the opposite pole (light-world ink1
toward L 0.15). Rationale: figure/ground needs a ground; a family compressed into the
middle (today: paper 0.938–0.975 total) reads as one gray sheet. Probe: §1/§2 frames.
Open A/B: paper anchored at pure 1.0 vs off-white 0.985 (does paper's warmth survive?).

## Draft B — §7 addendum: the chrome selected state is a light pill

Selected chrome (tabs, tree rows, list rows) renders as a rounded fill *just off the
anchor* plus an ink jump to ink1 + semibold — never a deep fill with unchanged ink.
(Reference grammar: #EBECF0 pill + #010005 ink. Today's paper selected = 0.888 slab —
a *dark* object on light chrome, the backwards direction.) Note: this refines the §7
light-theme direction claim for the *selected* step; hover keeps the small darkening.
The asset-cell family is untouched (§7's raise-to-white exception stands).

## Draft C — §3 sharpening: two line grades + the cull rule

The hairline rung splits into two registered grades:
- `line.container` — bounds true containers (panel seams, cards, wells-by-border).
- `line.separator` — one register lighter; internal splits only.
A separator may exist only where whitespace cannot carry the split (crowded data rows,
scroll-clipped heads). Rows in trees/inspectors separate by whitespace by default.
This is enforcement of the existing ladder ("cheapest sufficient rung"), not a reversal:
today one grade is applied liberally, spending line-contrast that whitespace should fund.

## Draft D — fill grammar (new §3 note or §25 note): fill XOR border, fills as objects

A gray fill marks interactivity or data (well, selected pill, chip, header band) — never
zoning decoration. An element carries fill OR border, not both (today's note input has
both). Chips/badges move outline → quiet tint fill (registry recipe change: the `outline`
recipe stays available for tag hues; the *machinery* chip default becomes tint).

## Draft E — §24 radius re-pins

control 4 → 6 · selection pill = control (6) · transient 8 → 12 · card/floating panel 10
(new rung if cards become a surface role) · docked stays 0 · round stays 999.
Probe: §7 sweep. No squircle machinery.

## Draft F — type-role remaps (role layer only, no ramp edits)

- ink.1 deepens toward L 0.15 (light worlds) / 0.95+ (dark) — the ladder's top rung must
  be near the pole for hierarchy to register (Linear's fix, verbatim).
- Section heads (`head` role): weight 600 + ink.1; probe whether a size bump is needed
  after the ink deepening (likely not).
- Selected/current items: ink.1 + 600 via the pill grammar (Draft B), not a new role.

## Draft G — canvas identity (hue-ledger/registry row if adopted)

The grid well (and other workspace canvases) may carry a dot grid: 1px dots, ~15px pitch,
α ≈ 0.05–0.08 (measured ref: #EFEFEF on white ≈ α 0.06), achromatic, never under text
panels, never on the stage. Registered as a surface treatment row, off by default outside
the grid well. Probe: §8.

## Draft H — dependencies and deferrals

- Iconography (§30.9) graduates to prerequisite: the reference rhythm leans on 16px
  outline icons in rows/heads; the seeded registry grows during the next code round.
- Spacing audit (row pitch toward ~32 in trees, card insets 16–20) rides the code round,
  quantum multiples only — no token change needed.
- Near-black dark world (probe §2 CAND B): if the eye prefers it, it lands as a THEME
  (new gray ramp), not an edit to graphite/carbon.
- §6 the occlusion axiom: UNCHANGED. Round-1's material recipe (gradients, inner
  highlights, control shadows) is withdrawn.
