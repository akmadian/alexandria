# Search and Filter UX

**Status:** design locked 2026-07-07 (C6/C12). This is the *presentation* of the query system;
the AST, token registry contract, and seam methods live in `../../docs/seam-contract.md`.

> **Amended 2026-07-08 (redesign round):** the frontend token/AST contract is now concrete in
> `09-ground-up-redesign-notes.md` §Token & AST drill-in — the token/leaf/pill triad, value
> kinds (7 shared editors, ONE date grammar), negated operators + assembler normalization,
> anchor+duration date values with rolling `"now"`, wire typing (generated
> `TokenField`/`TokenOperator` unions, strict constructors, persistence-boundary validation),
> and the AST versioning policy.

## The filter bar

- **A pill is the rendered form of one AST leaf** (macOS Finder search tokens, Gmail chips):
  click to edit operator/value, × to remove. The row of pills *is* the query, visibly — one thing
  on screen = one node in the tree.
- The flat pill row covers the common case (implicit AND); the recursive AND/OR/NOT group editor
  is the advanced path, shared with the smart collection editor.
- **Save as Smart Collection = one click from any query state** ("bookmark what I'm looking
  at") — the on-ramp; the editor is for refinement.
- The status bar narrates the query in plain words (`01-flows-and-views.md`). Invest in the
  **round-trip property**: render any AST as a readable sentence, parse sentence-like input back
  into tokens — then display, saving, and NL search are one system, not three.
- Filtering on a still-computing signal annotates the pill honestly: "sharpness > 0.5 · **214 not
  yet scored**", results streaming in (`05-culling-and-signals.md`).

## Search tiers

Global entry: **Cmd+F** (or `/`) opens the command palette in search mode (`04`), scoped to
everything by default. Time-to-file is measured in keystrokes (flow #2).

1. **Deterministic parser** (always on, zero latency): lexer + recursive descent over a **closed
   vocabulary** — token names/aliases ("photos"→`type:photo`), date grammar ("2025", "last
   summer"), a small phrase table ("grouped by day"→arrangement, "before/after X"), and the
   user's own tag/collection/place names. "japan photos 2025, grouped by day" parses fully
   deterministically if Japan is in the user's vocabulary. This is how Gmail/GitHub/Finder chips
   work, and it covers most real input because people search in their own tag vocabulary.
2. **Local LLM fallback** (optional, off by default until built): only for unresolved remainder —
   paraphrase/synonyms ("shots from my trip to the alps"), fuzzy time ("a couple summers ago").
   Schema-constrained to emit only valid AST against the token vocabulary; the user's candidate
   tag/place names injected into the prompt (a shortlist step, *not* RAG). Latency budget 0.5–2s
   is fine — NL is a deliberate act (type sentence, press Enter), never the hot path; typed pills
   are always instant and model-free.
3. **FTS fallback** (always): words neither tier resolves become full-text terms. **This is why
   NL-off changes nothing structurally** — the feature degrades to baseline search, not to a
   different system.

Semantic content ("photos with red umbrellas") is categorically different — not parsing — and
waits for CLIP embeddings (P4), arriving then as new token types (`similar-to:`, `looks-like:`).

Output of every tier is visible pills (C12): a misparse is a correctable annoyance, never a
mystery. "How the hell did it think that's what I wanted" is the failure mode this kills.
