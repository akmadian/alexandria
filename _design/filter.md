# Filter: round record

**Status: RATIFIED** (filter round, 2026-09-16). The round's decision
record — rulings, alternatives considered and why they lost, the wire
format, and the seam into the working set. Register: every ruling below is
Ari's; research citations ground them. Built, adversarially reviewed
(design-time and post-build), and fix-passed the same day.

## The noun

A **filter** is a user-authored condition that narrows the working set's
membership. It compiles to SQL beside the source's clause (scope ⊂ filter,
UI data-layer round); it never orders, never changes what a member is. A
**token** is the leaf condition — {field, operator, value, negated} — and
a **group** joins children (tokens and groups alike) with AND or OR. A
future **smart collection** is a saved filter: the same structure,
serialized into the collection's predicate column (collections round,
ruling 2).

## Rulings, with alternatives considered

1. **Tree-capable from the start, flat P0 authoring.** The format and
   compiler handle arbitrary nesting (`FilterGroup` combine and/or,
   children mixing tokens and groups; negation on leaves ONLY — leaf-only
   negation is fully expressive, De Morgan holds under SQL's three-valued
   logic). P0 authoring is one AND root of tokens. Prior art: LrC stores a
   nested tree (`combine = intersect/union` in .lrsmcol's Lua) under a
   flat default UI; Photos' flat-only and Capture One's one-group-level
   are documented user pain. Root positions are TYPED as groups
   (hub posture, stored envelope), so a bare-token root is
   unrepresentable — enforcement by type, zero validation code.
   Rejected: minting only a flat list now (a format migration later, for
   ~60 lines saved); strict group-only children (synthetic singleton
   groups around every lone token — encoding a UI limit into the format).

2. **Storage is versioned token structure, never SQL.** Wire form:
   versioned JSON envelope, tagged values (`{"int":3}`), single-key node
   encoding, sorted keys (encode→decode→encode is byte-stable, pinned).
   `version` is the VOCABULARY generation — any new field or operator
   bumps it — and a reader refuses any generation it doesn't know, whole:
   skipping unknown tokens would WIDEN the matched set, the one failure a
   filter must never have. Public face: `FilterGroup.serialized()` /
   `init(serialized:)`; the envelope is a private wire detail. Rejected:
   raw SQL storage (round-trips into pills only through a SQL parser —
   Finder's `.savedSearch` RawQuery is the shipping write-only cautionary
   tale — and bakes column names into user data); NSPredicate as the
   representation (a third representation whose only consumer would be a
   deferred UI component; GRDB has no NSPredicate lane, groue/GRDB#789);
   Foundation #Predicate (compile-time key paths vs a runtime registry).
   NSPredicateEditor/NSRuleEditor stay candidates for the smart-collection
   EDITOR only; a tokens→NSPredicate bridge is written then if ever.

3. **P0 vocabulary: rating, flag, kind** — the three asset columns that
   exist today; no schema change. Operator sets are FIELD-owned, seeded
   from the value type but corrected per domain: rating
   {eq,lt,lte,gt,gte,between,isUnset}, flag {eq,isUnset}, kind {eq} (NOT
   NULL — isUnset would be a dead affordance). No `neq`: it is exactly
   the NOT toggle on eq, and two spellings would let equal filters render
   unequal. `between` is inclusive both ends (LrC's stance and SQL
   BETWEEN's own), bounds ride in the value as `.range(Value, Value)` —
   recursive, not welded to Int, so date/size fields reuse it. Inverted
   ranges and out-of-domain values are refused at the fences (validation
   at decode; debug assertion at the intent), never compiled to
   legal-but-always-empty SQL.

4. **NULL semantics.** Positive comparisons are literal three-valued
   logic: an unrated asset matches NO rating comparison. LrC coalesces
   unrated to 0; ruled a wart, not wisdom — our schema's stance is NULL =
   no judgment, 0 is not a rating. `negated` compiles to
   `(expr) IS NOT TRUE`: the true complement, so unrated lands on the NOT
   side (naive NOT() is NULL for NULL operands and silently drops unrated
   from BOTH sides). `isUnset` is the deliberate route to unrated /
   unflagged. The complement and positive sides partition the catalog
   exactly (pinned by test).

5. **The seam.** The filter's clause composes INTO each membership
   subquery, BEFORE ordering — never around the ordering wrapper, where
   the ORDER BY would sink into a subquery whose order the outer statement
   doesn't contract to keep. With filter nil every statement is
   byte-identical to its pre-round text (pinned across sources).
   `WorkingSetQuery` composition converted end-to-end to GRDB SQL
   literals: every bound value rides WITH its placeholder, so a spliced
   clause can never shift another clause's bindings (the prior String +
   StatementArguments form was positional bookkeeping, correct only by
   accident of placeholder positions). Rejected: rewriting the walked SQL
   into the query-interface DSL (re-litigates review-blessed statements
   for ceremony).

6. **Manual-sorted collection + filter INTERSECTS.** The clause runs once
   as its own statement; the sectioned walk drops non-members; authored
   order is preserved for survivors. Without this the manual path —
   which never touches the spliced SQL — silently showed unfiltered
   results. Rejected: refusing the combination (more machinery than the
   intersect, and it takes away "my hand-picked sequence, just the
   picks").

7. **Files lens: a file matches if its ASSET matches** (asset-unit tokens
   cross through the relation, one IN-subquery). Formation-pending files
   (asset_id NULL) match nothing and drop out of any filtered files view
   — intentional, pinned. The reverse crossing (file-unit tokens under
   the asset lens) is ruled-not-built: any-file-matches, with the
   quantifier scope deliberately unsettled (see markers).

8. **Engine is UI-free.** Tokens carry no identity — two equal tokens ARE
   the same token; a pill UI owns its own identified wrapper (an id here
   would poison the envelope and every equality). Every gesture is a pure
   (group) → (group) transform committed atomically through the hub's
   `setFilter`; `normalized()` prunes empty groups (empty root = nil, ONE
   representation of "no filter") at the intent and the persistence fence
   alike.

## The wire format

```json
{ "version": 1,
  "root": { "combine": "and", "children": [
      { "token": { "field": "rating", "op": "gte", "value": { "int": 3 }, "negated": false } },
      { "group": { "combine": "or", "children": [ … ] } }
  ] } }
```

## Placement

`Alexandria/Core/Filter/` — four files, one concept each: `FilterNode`
(shape), `FilterVocabulary` (fields/operators/values, validation, typed
errors), `Filter+SQL` (compilation), `Filter+Persistence` (Codable +
version fence). Hub: `filter` posture field + `setFilter` intent beside
source/arrangement.

## Deliberately unsettled (markers live in code)

- File-unit fields — their round's MANDATORY first question is the
  quantifier scope of file-unit tokens under the asset lens (one file
  satisfies ALL criteria vs any file per criterion); the compiler gains
  the lens in its signature then. No per-field unit property exists until
  then (one-valued property = speculative field).
- Smart-collection storage: predicate column lands as `String?`, decoded
  lazily per use — one corrupt predicate disables one row, never the
  sidebar's fetch.
- Vocabulary growth (filename/text, capture date, missing, "not in any
  collection") — each bumps the version.
- Tree-authoring UI and its path helpers; the pill bar; whether the
  active filter survives relaunch (viewpoint-persistence round).
- Perf recorded as rough: fine at 40k by reasoning, UNTESTED at 1M;
  `// PERF:` comments in WorkingSetQuery name each trigger.
