# Token-inventory gaps: scrim, chrome dimensions, icon registry seed

**Parked as a pick-list, 2026-07-17 (Ari):** components-first — each item below lands as a
mini design decision inside the component round that first needs it (a dialog forces the
scrim call; the first icon-bearing primitive forces the registry seed; shell polish forces
the chrome dimensions), rather than as one batch round. The D29 method still applies
per-pull: values render before they ratify, Ari's eye is the gate.
**References:** `docs/design-constitution.md` §6, §12, §14, §18, §19, §26, §28, §30.5;
`frontend/design/CLAUDE.md` (method, decision altitude); `frontend/design/registries.json`.

The 2026-07-17 system review diffed the token inventory against the industry taxonomies
(DTCG tiers; Spectrum's global/alias/component inventory). Coverage is strong and the
contract layer is ahead of the field; these are the holes found. All are additive — new
roles, new rows, new §-lines — nothing existing changes shape. Every value renders before
it ratifies (probe page or the task-24 library); Ari's eye is the gate; the task-23
validator must pass on the result.

## Items

1. **Overlay scrim + the dim treatment.** No backdrop token exists. Decide: is a
   dialog/sheet scrim lawful under §6 (occlusion doctrine), and what is §26.1's
   "chrome dims and recedes" during grid→loupe, concretely? Deliverable: a §6 amendment
   naming the treatment + role token(s) with per-theme values (a scrim that reads on
   Carbon differs from Paper's).
2. **Chrome dimension tokens** (§30.5 finishing its own list): header bar, filter bar,
   status bar, and filmstrip heights; grid-cell geometry — cell aspect, inter-cell gutter,
   thumbnail size steps (§19 specs the anatomy; only `cell-pad` is tokenized today). The
   shell CSS currently hardcodes 44px/28px; after this round it may not.
3. **Icon registry seed.** A new `icons` section in `registries.json`: concept → Lucide
   glyph, one glyph per concept, per §14/§18 (the registry extends "UI nouns come from the
   vocabulary" to pictures). Seed the v1 concept set (judgment verbs, tree/chrome
   affordances, attention states); the stroke constant stays a §30.9 hypothesis — the
   iconography round owns empirics.
4. **Histogram ink** (§12 inspector). One written rule for what the histogram draws in —
   presumption: machinery-achromatic (§10), since hue there would collide with the ledger.
   A registry/constitution line, not a new hue row, unless the round decides otherwise.
5. **§28 honesty-list additions:** forced-colors / Windows High Contrast named as a
   deliberate defer (currently silently absent). `::selection` and scrollbar thumb/track
   values: pin if trivially decidable from existing roles, else add to §30 as PIN.
6. **Ring-step eye-tune, both worlds** (from the 2026-07-17 Phase C adjudication —
   §29/D31). Dark world: the ring is a computed starting symmetry (L 0.74 C 0.12, PIN)
   — tune per hue by eye. Light world: the ring aliases the solid, which leaves seven
   hues (peach/orange/amber/lime/green/teal/cyan) failing ring contrast on paper/linen
   and only 5 of 12 hues accent-eligible — decide whether that attrition is an accepted
   design fact or the light ring detaches from the solid and tunes darker. The
   validator's eligibility output is the instrument. Also from the same adjudication:
   linen's selected-primary target sits at Lc 70 pending its probe (contracts.json
   selected-text note).

## Non-goals

Fun-layer recipe values (§30.8 stays its own round with `fun-probe.html` as instrument).
Dark-world cell duals (loupe round). Windows ClearType checks. No component code.
