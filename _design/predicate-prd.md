# Predicate language PRD — one structure for "show me things where…"

**Status: RATIFIED** (adopted 2026-09-09; distilled from the old repo's query
AST, invoked by Ari as archaeology and re-expressed in the current noun model.
Concepts crossed as prose; no code, serialization format, or field inventory
carried. Open questions at the end are exactly that.)

One serializable predicate structure of our own: built by the filter UI, stored
by smart collections, compiled to database queries
([data-models.md](technical/data-models.md) records the own-it decision and the
rejection of Foundation's predicate types). Its reason to exist beyond the
minimum: arbitrarily nested, grouped, negated conditions — the filtering
richness LrC lacks, already a requirement.

## The shape

- **A tree.** Boolean group nodes — and / or / not, holding children — over
  leaves of field × operator × value. Groups nest without depth limit in the
  model (the UI may cap presentation depth if it ever needs to).
- **Scope is a filter.** "In folder F and below," "in collection C," "from
  import J" are ordinary nodes in the tree, not a separate frame around it.
  (The old design kept scope outside the tree and was forced to invent a
  scope-as-node escape hatch to express collection roll-ups — the fossil that
  confirms the ruling.)
- **Sort lives outside.** Arrangement decides order, never membership; a
  stored predicate carries no sort (sorting is a view concern — noun round).
- **Root-agnostic.** A query is a population plus a tree. The default
  population is assets (the grid's atom): asset-grain leaves (judgments,
  keywords) test the asset directly; file-grain leaves (filename, dimensions,
  location) reach through membership with any-member-matches semantics. The
  compiler must not hardwire assets as the only population (file-grain search
  is benched in the requirements backlog).

## The kind registry

- Every field belongs to a **value kind** — text, numeric, enum, date-range,
  keyword-reference, entity-reference, free-text (inventory expected to evolve;
  the mechanism is the ratified part). The kind, not the field, decides
  everything: which operators are legal, how values validate, which editor the
  filter UI renders, which strategy produces the SQL.
- **Fields are registry rows.** A field never enumerates its own operators; it
  inherits its kind's family. Adding a filterable capability = adding a row —
  the registry idiom applied to the query grammar. A field's row may carry
  small capability flags (e.g. its distinct values feed autocomplete); fields
  with no backing column (keyword membership, free text) compile through
  dedicated strategies.
- **Nullable fields get a presence pair** ("is empty" / "is not empty")
  appended to their operator family automatically.
- **Negation includes absent** (ratified: it is the literally correct
  reading). "Rating ≠ 5" matches unrated photos; a field a thing lacks is not
  equal to any value.
- **Subtree operators are first-class** for hierarchical references:
  "under / not under" a keyword (and its descendants), compiled via the
  ratified tree recursion. (Prior art: digiKam's InTree relation.)

## Dates

- A date value is an **anchor plus a calendar-aware duration**, forming a
  half-open interval. The anchor may be **symbolic "now," resolved when the
  query runs, never when it is saved** — this is what makes a stored
  "last 30 days" smart collection roll forward daily instead of freezing.
- Calendar-aware means "last 3 months" is 3 calendar months, not 90 days.
  Serialized as standard ISO 8601 durations.

## Serialization

- Ours, and **versioned**: every stored predicate carries a format version, so
  an older app detects a newer query instead of misreading it. The concrete
  format (JSON shape etc.) is unsettled — a schema-round/build decision.

## Deliberately not carried from the old design

- **The scope frame** and its scope-as-node escape hatch — absorbed into the
  tree (above).
- **The file-rooted query grain.** The old tree filtered files and reached up
  to group-owned judgments through a special-case fragment; the new default
  population is assets, inverting that join. The special case died with the
  premise.
- **The concrete field inventory** (and old vocabulary like "tag") — fields
  are added per feature as capabilities land; only the registry shape crosses.
- **Contract/codegen machinery** (API twin tests, TS generation, column-name
  derivation) — seam-shaped, dead.

## Open questions

- The concrete serialization format.
- Whether typed search-box input compiles into this same language
  ([data-models.md](technical/data-models.md), sharing breadth).
- The filter-building UI — including `NSPredicateEditor` as a front-end over
  this structure — is a UI-round question.
