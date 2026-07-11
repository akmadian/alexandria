# The Vocabulary — concepts and representation rules

Hand-written companion to the generated [data dictionary](data-dictionary.md).
That file enumerates (and cannot lie — it regenerates from the Go declarations);
this file explains. **Rule: no list that exists in code appears here.** If you
want members, read the dictionary; if you want *why*, read on. Doctrine:
CONSTANTS C15; decisions: backend decision log D24.

## Parts of speech

The system's vocabulary maps onto a grammar (DDD's ubiquitous language +
CQRS conventions):

| Part of speech | What | Where declared |
|---|---|---|
| **Nouns** (entities) | Things with identity and rows: Asset, Volume/Source, Tag, Collection, … | `internal/domain` structs |
| **Adjectives I** (closed value sets) | Enum facts about nouns | `internal/domain` const blocks |
| **Adjectives II** (composable predicates) | The token grammar: field × operator × value | `internal/ast` vocabulary |
| **Verbs** (commands, imperative) | State changes; absolute values, never deltas (C7) | `internal/seam` services; writer classes gate *who may speak which verbs* |
| **Questions** (queries) | Side-effect-free reads; one workhorse absorbs every predicate (C7) | `internal/seam` + `internal/ast` |
| **Events** (past-tense facts) | What happened; hints, not facts (C8) | `internal/seam` event catalog |
| **Adverbs** (where/how/which slice) | Scope, Arrangement, Page — modify a question, never membership (C4) | `internal/ast` |

## The three-level representation rule

Where a concept lives is decided by what it *is*:

1. **Identity → domain entity.** It has a row and an ID.
2. **Composable description → `ast` value object referencing entities by ID.**
   Scope, the filter tree, Arrangement. No identity, no rows. A smart collection
   is the one place a Query reifies into an entity (the "saveable thing", C6).
3. **Computable from the above → derived, never stored as TRUTH** (C2). The
   working set has no representation anywhere, on purpose. Selection and cursor
   are view state referencing IDs. If a "working set table" ever appears,
   something has gone wrong.

   **"Never stored" bans authorities, not caches.** Materialized derivations
   are welcome — and common: FTS, thumbnails, `tags.path`, `aspect_ratio`,
   memoized selectors, the TanStack cache — provided staleness is
   machine-managed (trigger, event invalidation, memo dependency, or a
   registered rebuild function) and deleting the copy loses nothing. The
   litmus is who owns invalidation: a stored derivation whose freshness
   depends on a human remembering to update it is forbidden; one a machine
   keeps honest is just a good cache. Never cite this rule against a
   materialization that pays for itself — cite it against a second source of
   truth.

## Naming doctrine — Type vs Kind

**"Type" is reserved for a file's format category; "Kind" for entity variants** (Ari's call,
impl/03 close-out 2026-07-06). `domain.FileType` and the `assettype` package resolve format
categories, so they are "type"; variant discriminators on entities (`GroupKind`, scope kinds,
`ValueKind`) are "kind". A new discriminator picks its word by this rule, not by taste.

## Translation verbs — they encode round-trip-ability

- **serialize / deserialize** (AST ↔ JSON): same structure, different encoding.
  Lossless, bidirectional — a stored smart collection must come back identical.
- **compile** (AST → SQL): translation into a different language. Lossy,
  one-way; nobody recovers the tree from the WHERE clause.
- **parse** (text → AST): looser to stricter, one-way.
- **evaluate** (AST → values): interpretation — the mock engine's job.

If you're naming a new translation, pick the verb whose round-trip contract
matches.

## Direction of truth — the exceptions worth labeling

- **XMP is a crosswalk, not a projection.** The xmp package maps *its*
  properties (an external standard, spec-owned names) onto domain values, with
  direction-dependent policy (tags union on read; labels normalize through a
  locale table with raw-string preservation). It is deliberately NOT keyed by
  `ast.Field` and NOT covered by a completeness test — restating the semantic
  mapping in a test would itself be the drift risk. Its contract is pinned by
  the xmp package's own behavior tests.
- **Command IDs flow frontend → Go.** `keybindings.json` values name frontend
  command-registry entries; Go stores them as opaque strings and must never
  validate them. The one vocabulary whose source of truth is TS. Key names use
  the W3C UIEvents `KeyboardEvent.key`/`code` vocabulary.
- **`ExtendedMetadata` is never load-bearing.** Nothing users can filter on may
  live only in the blob (LrC's opaque-blob rot is the cautionary tale). Keys are
  canonical exiftool tag names so promotion (blob → column is ALTER + backfill
  from blob, never a file re-read — D11) stays deterministic.
- **Paths: compare keys, open bytes.** `domain.PathKey` (Unicode NFC) exists for
  equality/matching/dedup only; on-disk bytes are the truth for I/O. There is no
  "denormalize" — the mapping is one-way by design (D24).

## Known forks in the road (recorded, not taken)

- **Logical/physical asset split** (LrC's `Adobe_images` vs `AgLibraryFile`):
  our Asset fuses them; the writer-class interfaces provide the code-level
  decoupling. Copies are REAL files with a `derived_from` lineage edge (D24),
  which removes the main driver for the table split. Re-evaluated inside the
  volume/folder round.
- **`Source` → `Volume` + `Folder`** is decided (D24) and owned by the
  source-management round. Until it lands, don't propagate "source" into new
  surfaces; the ast token `source` keeps its name until that round renames the
  noun.

## The black-box ledger — what fires when you forget something

A vocabulary is done when you can forget it (C15). What pages you, per surface:

| If this drifts… | …this fires |
|---|---|
| Generated TS vs Go declarations | CI freshness gate (`make check-generated`) |
| A registry missing a generated member (mock accessors, editors, enum pickers, sort accessors) | `tsc` — `satisfies Record<…>` / completeness trick |
| A switch missing a union member | `tsc` (return-type) + `switch-exhaustiveness-check` (TS) / `exhaustive` (Go) |
| A hand-redeclared generated union | ESLint `no-restricted-syntax` tripwire |
| ast fields vs `domain.Asset`; compiled SQL vs the real schema; extraction/patch shapes vs domain | the crosswalk suite (`cmd/generate/crosswalks_test.go`) |
| The derived operator grammar | `TestDerivedGrammar_Golden` (deliberate changes update the golden in the same commit) |
| Untested query-authority code | the 100% coverage gate (`.testcoverage.yml`) |
| Writer-class violations, domain purity, hand-written SQL outside ast | Go interfaces + depguard/forbidigo |

If nothing is red, there is nothing to hold in your head.
