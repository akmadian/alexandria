---
name: frontend-primitive
description: The Alexandria method for building a design-system primitive in `frontend/src/components/` — a leaf like Badge/Kbd/Button or a container+item like Menu/Tree/Select. Use it whenever you're about to build, add, restyle, or meaningfully reshape a frontend primitive in this repo, even when the ask just names the thing ("add a Tooltip", "build the Kbd keycap", "make the Switch feel right", "this dropdown looks off") and never says "skill". It grounds the work in industry convention and our design system, keeps you out of pixel-tweaking, and routes token gaps through the right governance. Feature compositions in `features/` are a different animal (they add the store/seam/query layer) — this is for primitives.
---

# Building a frontend primitive

You're building a primitive in `frontend/src/components/` — a leaf (Badge, Kbd, Button) or a
container+item (Menu, Tree, Select). This is the *method*. The law lives in the docs and the
tokens, which are alive — so read the real files and cite them; don't paraphrase them from memory (a
copy rots and starts lying).

**One idea sits under everything here: don't invent.** Be lazy about code — YAGNI the machinery, no
speculative size rung, no prop for a consumer that doesn't exist yet — and relentless about
grounding — never ship a value you can't trace to something above it. Shorthand:

> **Maximally grounded, minimally built.**

## Ground in two directions

You are never designing from scratch. Every choice cites a convention above it, pointing *outward*
(the world) or *inward* (our system):

| | Ground outward | Ground inward |
|---|---|---|
| **Interface** (props, composition, a11y, ergonomics) | the shape libraries have converged on (shadcn, Radix, React Aria, MUI, Ark, Spectrum…) + RAC for behavior | our sibling primitives' prop vocabulary — `size`/`style`/`icon` mean the same across Badge, Menu, Kbd |
| **Implementation** (sizing, values, styling) | the *documented technique* for a hard problem (e.g. `text-box-trim` for optical centering) | our tokens and grammar — never the world's pixels |

The rule that keeps you from cargo-culting: **converge on the outside, translate on the inside.**
Adopt the interface the world agreed on so the thing feels familiar; take every value from our
tokens.

## The flow

**1 · Ground.** Read, in order, only what you'll touch — the real files, not your memory of them:
`docs/CONSTANTS.md` → the neighboring `docs/decisions.md` entries (how we solved the last few
primitives — your templates) → `frontend/CLAUDE.md` §3, §6, §8 → the relevant
`docs/design-constitution.md` §§ → **the closest existing primitive's source** (the highest-leverage
read) → `tokens-reference.json` for exact names. Then do a **prior-art pass, every time**: a couple
of web searches on how mature libraries expose this component (API, composition, a11y, ergonomics)
plus RAC via its MCP — a few queries against the cost of rebuilding a wrong interface is never a bad
trade. Note in a line or two what you're adopting and what you're diverging from, and why. Finally,
if the relevant design *law* is in tension with what you're building, surface it and let the user
resolve it — don't quietly decide it yourself.

**2 · Decide by render.** For a visual fork, build the decision instead of arguing it. Make a
throwaway exhibit — a `show_widget` mockup or a temporary `#/design-library` specimen — with the
*real* token values, render the options across the themes, let the eye pick, then delete it. It
stops you from building the wrong thing and defending it.

**3 · Build by mirroring.** Before writing sized or token CSS, open the closest primitive and copy
its structure. Badge is the interface template (props, the `satisfies Record` completeness trick,
`composes` for the type unit); `button.module.css` is the **control-size bundle** — a size class is
a whole tier of *{type role + height + pad + icon}* moving together, derived from one target.
Hand-picking any piece is the classic way this breaks. Two more: check what the ramps actually
*have* before you design a ladder (mono is only `data`/`data-sm`), and match the sibling API rather
than inventing a prop name.

**4 · Verify by measuring.** Measure for facts, render for taste. After writing token CSS, read the
*computed* values across the matrix — don't assume the CSS did what you meant (a self-referential
var is invisible in review, obvious in a probe). And when something looks wrong, prove the cause
before you fix it — a measured diagnosis points at the right fix; a guess doesn't.

**5 · Gate by eye.** The human ratifies anything aesthetic — render it and let them gate. Give them
*rendered options with a recommendation*, not a prose menu. And know when the honest answer is
"accept": distinguish "I haven't found the fix" from "there is no idiomatic fix in this stack," and
when it's the latter, prove it rather than churn (a sub-pixel nudge that blurs the glyph is worse
than the flaw).

**6 · Close.** `make check-frontend` green · the Storybook matrix + a `#/design-library` specimen ·
a `decisions.md` D-number and §29 line if anything was decided · memory updated · then run
`pre-commit-review` and resolve it before presenting for commit.

## Stay top-down — the speed bump

Once code exists, editing values to satisfy an eye *feels* like the fast path. It's a trap: the goal
isn't "make it look right," it's coherent design in a shared language, which flows top-down (tokens
→ roles → components). Pushing values up from the component fights that.

So **any visual feedback — or any urge to nudge a number by feel — stops you at a checkpoint. Before
editing, name the mechanism in one line.** Three exits:

- **"I'm missing a derivation: `<token/role>`"** → go consume it. *(usually this)*
- **"An existing role fits; I was value-quibbling"** → use it, even if you'd have picked a different value.
- **"No role *means* this — a real gap"** → stop; that's a design-source proposal, below.

If you can't name the mechanism, you've found the symptom, not the fix. A visual complaint is almost
always a system mismatch, not a pixel.

## Token gaps: fit is semantic, not numeric

The default is **consume** — the system has ~everything. When you think it doesn't, judge fit by
*meaning*, not value: `data-sm` isn't "11px," it's "the mono metadata voice." If the closest token's
meaning matches, use it even if you'd have chosen a different number. If none matches, it's a real
gap — and filling one is routine, not a failure. The discipline is in *how* (see
`frontend/design/CLAUDE.md`):

- Climb the altitude ladder first — role remap → register-step → new registry row → only then a new
  token. Most gaps resolve high.
- A new token is a **design-source round**, not a code edit: it goes through the compiler and must
  pass the contracts (if it fights one, you're at the wrong tier).
- **Always pause and propose; never mint unilaterally.** Hand the user the gap, the minimal fill,
  and a render — the pause is non-negotiable, but the ceremony scales to the size of the fill.
- Defer unless a consumer needs it now.

Forcing an ill-fitting token to avoid "bothering the system" is the same class of error as minting
carelessly — both dodge the correct altitude.

## When a bug smells systemic

If a bug feels like *how you approached it* rather than a local typo, don't patch the instance —
audit the whole component against the exemplar. One wrong icon size was the symptom; a hand-wired
size bundle was the disease, and the audit caught seven issues at once.

## Traps that cost real time

- Fresh worktree → `bun install` in `frontend/`; backend in a worktree → `GOWORK=off`.
- A worktree runs its own Storybook — the MCP may point at a different instance.
- The browser pane's screenshots go stale/blank and its ResizeObserver never fires — trust DOM
  probes; same-hash navigation doesn't reload (use `location.reload()`).
- Pane clicks can't drive RAC checkbox-family toggles — verify with tests + an in-page `input.click()`.
- Vertical optical centering → `text-box-trim`; horizontal glyph centering has no idiomatic fix
  (accept geometric).
- Generated `tokens.{css,ts}` + `tokens-reference.json` must be **staged** for the freshness gate.

## Done

Checks green · every value traceable to a token or a documented technique · Storybook matrix +
design-library specimen · decisions.md/§29 if anything was decided · memory updated ·
`pre-commit-review` resolved. If a value in the diff was chosen by feel, it isn't done.
