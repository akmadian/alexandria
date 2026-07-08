# Query System

**Status:** design locked 2026-07-07 (C6/C7/C12). **Joint frontend/backend doc** — the AST is the
seam spine; the backend query-layer design round implements the compile side. Supersedes the
smart-collections-only framing of the query builder: filter bar, search, NL, and smart
collections all speak this one grammar.

## The AST

The query as **typed structs**, never stringly key-value maps (KV can't express nesting, OR/NOT,
or two conditions on one field, and forfeits the compiler). A tree of boolean nodes over
predicate leaves:

```jsonc
// persisted form (smart collection); version field from day one (C6)
{
  "version": 1,
  "scope": { "kind": "collection", "id": "…" },        // extensional root; optional
  "where": { "op": "and", "children": [
    { "field": "type",   "cmp": "eq",  "value": "raw" },
    { "field": "rating", "cmp": "gte", "value": 3 },
    { "op": "not", "children": [ { "field": "tag", "cmp": "under", "value": "wip" } ] }
  ]}
}
```

Defined in Go (`internal/…`), generated to TS (C13). Backend compiles it to SQL — the same
compiler serves the filter bar, smart collections (stored dynamically-evaluated queries), system
smart collections (Untagged, Unrated, Suggested Rejects), and Review projections where predicates
apply.

**Pattern lineage:** interpreter pattern (GoF). Living relatives: Prisma/Mongo filter objects,
Elasticsearch query DSL, JIRA JQL, Notion filters — and LrC smart collections internally. The
GraphQL comparison, settled: GraphQL solves client-chosen *result shape* and drags in transport/
middleware; we take only the filter-DSL half — client-chosen *predicate*, fixed result shape,
plain structs over Wails bindings.

## The token registry

A **token type** is one registry entry:

```
{ name, field, operators, value parser/validator, pill renderer, SQL compiler }
```

Frontend owns parse/render; backend owns compile. Registry rules and completeness enforcement per
C10 (TS `satisfies`, Go `MustValidate`).

**Extension flow** (the whole point): new capability = new column at ingest/enrichment → backend
registers the compiler → frontend registers the token → it appears in the filter bar, the smart
collection editor, and the NL vocabulary *automatically*. Zero new views, zero new seam methods.
Example: `sharpness` (`06`).

v1 vocabulary: filename, file type, tag (`under` for hierarchy), rating, color label, flag,
capture/ingest date ranges, source, dimensions, camera make/model, plus LrC-style metadata text
fields (contains / starts-with / is-empty). FTS free-text is itself a token (`text:`).

## Filter bar UX

- **A pill is the rendered form of one AST leaf** (macOS Finder search tokens, Gmail chips):
  click to edit operator/value, × to remove. The row of pills *is* the query, visibly — one thing
  on screen = one node in the tree.
- The flat pill row covers the common case (implicit AND); the recursive AND/OR/NOT group editor
  is the advanced path, shared with the smart collection editor.
- **Save as Smart Collection = one click from any query state** ("bookmark what I'm looking
  at") — the on-ramp; the editor is for refinement.
- Plain-language narration of the query in the status bar (`02`), round-trippable with parsing:
  render any AST as a readable sentence; parse sentence-like input back. One system serving
  display, saving, and NL.

## Search tiers

Global entry: Cmd+F (or `/`) opens the palette in search mode (`05`) scoped to everything.

1. **Deterministic parser** (always on, zero latency): lexer + recursive-descent over a **closed
   vocabulary** — token names/aliases ("photos"→`type:photo`), date grammar ("2025", "last
   summer"), a small phrase table ("grouped by day"→arrangement, "before/after X"), and the
   user's own tag/collection/place names. "japan photos 2025, grouped by day" parses fully
   deterministically if Japan is in the user's vocabulary. This is how Gmail/GitHub/Finder chips
   work, and it covers most real input because people search in their own tag vocabulary.
2. **Local LLM fallback** (optional, off by default until built): only for unresolved remainder —
   paraphrase/synonyms ("shots from my trip to the alps"), fuzzy time ("a couple summers ago").
   Schema-constrained to emit only valid AST against the token vocabulary; user's candidate
   tag/place names injected into the prompt (a shortlist step, *not* RAG). Latency budget 0.5–2s
   is fine — NL is a deliberate act (type sentence, press Enter), never the hot path; typed
   pills are always instant and model-free.
3. **FTS fallback** (always): words neither tier resolves become full-text terms. **This is why
   NL-off changes nothing structurally** — the feature degrades to baseline search, not to a
   different system.

Semantic content ("photos with red umbrellas") is categorically different — not parsing — and
waits for CLIP embeddings (P4), arriving then as new token types (`similar-to:`, `looks-like:`).

Output of every tier is visible pills (C12): a misparse is a correctable annoyance, never a
mystery. "How the hell did it think that's what I wanted" is the failure mode this kills.

## The seam

Workhorses (few, broad, typed — C7):

| Method | Shape |
|---|---|
| `QueryAssets(query, arrangement, page)` | → page of assets. Absorbs every predicate: browse, filter, search, smart collections, missing-files, sharpness thresholds. |
| `UpdateAssets(ids, patch)` | Closed optional-field patch (rating, label, flag, tags±, note) — the seam face of TriagePatch. Presets, copy/paste metadata, mixed-state batch edits all ride it. |
| Events | One envelope over named topics, cataloged per C8. |

Plus boring structural methods: sidebar trees with counts, collections/sources/tags CRUD, jobs
control, Review actions, settings. Honest sizing: **30–50 typed methods** through P2 is healthy;
the number that matters is shape stability, not count.

**The rule (C7):** new method ⇢ new *result shape*, never new *predicate*. `GetSharpAssets` is
the smell; sharpness is a token. This rule is the railing on the "just add a bespoke binding"
slope.
