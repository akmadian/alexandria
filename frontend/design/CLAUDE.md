# Alexandria design source — agent orientation

This directory is **the design system's source of truth**. The tokens are the product;
all code is downstream of them. If you are here to do design work, this file is your map.
If you are here to build components, read `frontend/CLAUDE.md` first — it tells you how
code *consumes* this system.

## Read in this order

1. **`docs/design-constitution.md`** (§1–§30) — the design law. §29 is the dated
   ratification record; §30 is the open-pins worksheet (closed items carry tombstones).
   Everything visual traces to a rule there; anything that can't is drift.
2. **`tokens.resolver.json`** — the entry point. Themes are resolver modifier contexts
   over theme-invariant token names (DTCG 2025.10 + Resolver Module).
3. **`tokens/`** — the source tree:
   - `primitives.tokens.json` — option tokens: named hue scales (12 + gray; per-hue
     engineered solids with declared on-solid ink), space, type composites, weights,
     durations, easings, strokes.
   - `semantic.tokens.json` — decision tokens: stage, accent/attention/error/labels
     (aliases into the scales), fun layer, radius/z/shadows/focus, sizes (control 24 /
     control-lg 28, row-control 28 / row-list 24 / row-text 16), **type roles**
     (display → micro; each role = typography composite + paired ink).
   - `worlds/hues-{light,dark}.tokens.json` — world-varying hue steps (tint/tint-ink/
     line) + the **register-step quantum** (0.018 light / 0.035 dark).
   - `themes/{paper,linen,graphite,carbon}.tokens.json` — the gray ramps, role-shaped
     by deliberate decision (see paper's `$description`).
4. **`contracts.json`** — the validator's input (APCA pairs, ΔL bands, hue distance,
   structure rules). **`registries.json`** — dispatch tables (tag recipes, row intents,
   hue eligibility, family direction, fun sites, machinery map).
5. **`library/`** — the probe pages. `type-probe.html` renders the committed type
   system in real Geist (fonts in `library/fonts/`); add sibling probe pages the same
   way as new rounds need them. Serve with launch config `design-library` (port 8123).

## The compiler is live — no frozen legacy, by decision

The interim runtime token generator, its frozen `tokens.json` snapshot, and the old
swatch-library trio were **deleted** (2026-07-13, D29) once they went stale: a tool
rendering outdated values authoritatively is poison for future sessions. The Phase C
compiler landed 2026-07-17 (D31): **`compiler/`** (bun + TS; culori + apca-w3)
resolves this directory, executes `contracts.json` — **a failing contract blocks
emission** — and emits `../src/styles/tokens.{css,ts}` + `tokens-reference.json`,
freshness-gated in `bun run check`. Design sessions edit `tokens/` and re-run
`bun run generate:tokens`; a change that fights a contract will not emit — that is
§23 enforcement working, not an obstacle to route around. Emitted names: the strict
path mirror (`--alx-` + token path, dots → hyphens) + one `.alx-type-<role>` unit
class per role (`:where()`-wrapped: role classes are zero-specificity DEFAULTS a
component declaration always beats). Never edit emitted files; never resurrect a
runtime generator. The **in-app design library** (`#/design-library` under
`bun run dev`) renders the emitted system live — primitive matrices, type-role
specimens, and the reference table across all four themes; every new primitive
lands its matrix there (frontend/CLAUDE.md §6).

## How design sessions run here (the method that works)

- **No implementation code.** The deliverables are token values, constitution
  amendments, and probe pages. Code rounds are separate.
- **Every value change is rendered before it is ratified**: probe HTML in `library/`
  (the only way to judge real Geist) or conversation widgets with live APCA readouts.
  Ari's eye is the gate; the render must show the exact values being committed.
- **Decision altitude** — where a change belongs, in order of preference:
  1. **Role remap** before ramp move (e.g. label-sm → ink.3, never "darken ink.2
     globally"). The role layer is the firewall between one case's taste and the world.
  2. **Register-step arithmetic** for state-fill nudges ("one step quieter" is math,
     not archaeology; multiples of the world's register-step).
  3. **A new registry row** for a new capability — never a one-off value.
  4. If a change fights the contracts, you are at the wrong tier — stop and reframe.
- **Record everything**: dated `$extensions.alx` notes on changed tokens, §29
  ratification lines, §30 tombstones for closed pins, agent-memory update.

## Inspiration (the taste anchors)

The curated reference set lives outside the repo on Ari's machine
(`~/Desktop/design round 2/` — ask him if it moved). `Figma-ish/` is the core UI
language (especially the db-browser shots: left tree, hairline construction, mono data
chips); the OpenPurpose Badge Grid is the tag-palette shape; Vercel's Geist palette is
the color temperature ("bold and saturated"). **`_archive/` inside that set is
off-limits by explicit instruction.** Radix Colors' 12-step methodology is the
engineering reference for scales.

## Open board (as of 2026-07-17)

Fun-layer recipe + gradient palette-indexing (§30.8 — Ari has usage scenarios to talk
through first) · iconography round (§14/§30.9; stroke 1.5-vs-2 needs a real 1× display)
· dark-world cell duals (decided in the loupe-view round) · Windows ClearType 1× checks
(§28) · the token-gaps round: scrim/dim treatment, chrome dimensions, icon registry
seed, and the ring-step eye-tune — both worlds (§29 2026-07-17: dark ring is a
computed starting symmetry; the light ring aliases the solid, which leaves seven hues
failing ring contrast on paper/linen and only 5 of 12 accent-eligible today).
