# Data models — schema round record

**Status: decision record for the schema round (opened 2026-09-09).** Entries here
are RATIFIED by Ari in discussion; everything else is marked open. Concepts, not
schema: no tables, columns, or types are settled unless stated. Nouns themselves
are defined in requirements.md; this file records how they are modeled.

## Ratified

- **Tree-structured nouns (folders, keywords, collections).** Modeled with a
  parent reference; subtree questions answered with SQLite recursive queries.
  Subtree resolution runs over the small tree tables, never the large ones.
  Upgrade paths exist in shipped prior art (precomputed ancestor table —
  digiKam; materialized path — Lightroom) — named as research, adopted only if
  measurement demands.
- **Location lives on files, never on assets.** A file has a folder
  relationship; an asset has none and cannot be tied to a single directory
  (its files may span folders). A folder-scoped view shows the assets with at
  least one file in scope, resolved by joining through files — folder subtree
  resolved first (recursion above), then files narrowed by it, then the hop to
  assets. How an in-scope asset's out-of-scope files are displayed is a
  UI-round question. Performance at target scale is unverified (open, below).
- **Scope is a filter.** "In folder F and below," "in collection C," "from
  import J" are predicate clauses, not a separate mechanism. Whether the UI
  presents scope differently from other filters is a UI-round question.
- **The predicate representation is ours; we own its serialization.** A smart
  collection stores one (the noun round defines a smart collection as a stored
  predicate). Foundation `Predicate`/`NSPredicate` rejected
  as the stored representation (macro is compile-time only; binds to static
  key paths, not a runtime field vocabulary; no public path to SQL).
  `NSPredicateEditor` as a UI front-end over our representation remains an open
  UI-round option.
- **Keywords attach to assets, not files.** Consistent with judgments
  attaching to assets; file-level keywords identified as complexity with no
  need behind it. Which files receive keywords on metadata write-back is a
  separate write-back-policy question (open, below) — whatever the answer, the
  target must be clear to the user.
- **Stacks carry no view state and no stored order.** Collapsed/expanded is
  window state — persistable as saved window/view state, never catalog state.
  Members have no inherent ordering: a collapsed stack sorts in the grid by its
  cover's sort value; expanded, members appear as ordinary assets in the grid's
  own sort, with a visible indication that they belong to a stack (the
  indication's form is a UI-round question).
- **Stacks are global.** One stack per asset (an asset belongs to at most one
  stack); a stack is the same fact about its assets in every view — never
  scoped to a folder or collection, creatable from any view. (Grounded against
  Lightroom's per-context stacks, a 15-year complaint magnet with no located
  defenders.) A stack is its own record — it carries the cover — with member
  assets referencing it; columns are schema-detail, unminted.
- **Representative and cover defaults.** Default election — an asset's
  representative file, a stack's cover — prefers the readily displayable
  rendition: a JPEG when available, else the RAW. A revisitable starting
  heuristic, not a promise. The user can override per asset and per stack;
  overrides survive everything automatic.
- **Stacking never makes an asset undiscoverable.** A filter match on a member
  of a collapsed stack surfaces it: the stack expands and non-matching members
  are deemphasized (the visual treatment is a UI-round question).
- **Identity: time-ordered UUIDs (v7-style) on every record.** Never reused,
  never bare rowids (VACUUM can renumber those). Generatable without a
  database round-trip; time-ordered so bulk-import inserts append instead of
  scattering the index. Adopted for anchor and reference integrity —
  judgments, memberships, exclusions, covers must survive renames, rescans,
  and rebuilds — not on sync grounds.
- **Timestamps are not a day-one habit — they're retrofittable.** Row-level
  created/modified appear only where a ratified feature consumes them (e.g.
  XMP write-out cursors, interrupted-work recovery). Per-field timestamps are
  refused until the sync round earns them from primary sources; the refusal is
  safe because a single-machine catalog has no conflicting history — a later
  migration can seed per-field times from row times losing nothing that
  matters.
- **Volume identity** is recorded in requirements.md (UUID if available,
  otherwise path; network shares by server + share name, never the mount
  point; fallible by design, repairable by the user).

## Deliberately unsettled — marked so nothing silently returns

- The predicate structure's shape is settled conceptually in
  [predicate-prd.md](../predicate-prd.md); its concrete serialization format
  remains open.
- Sharing breadth of the predicate structure: whether the filter system and
  typed search build/compile the same structure smart collections store.
- Metadata write-back fan-out: which of an asset's files receive written
  keywords/metadata (post-v0 with XMP write-back).
- Grouping rules (file → asset): where they live, what they can express.
  Conceptual foundation ratified in [grouping-prd.md](../grouping-prd.md).
- Sync-adjacent schema reasoning beyond the ID ruling (merge primitives,
  clock handling, per-field timestamps) — DERIVED and unverified; owned by the
  sync round's primary-source mandate.
- File modeling must account for macOS packages (a "file" may be a directory
  the system presents as one thing); how is unsettled.
- **Access-pattern walkthrough residue** (walked 2026-09-09; a throwaway
  measurement spike was rejected — constants get measured against the real
  implementation). Two decisions owed when the schema lands: (1) sorting the
  grid by worn metadata collides with the computed-at-read wearing rule — an
  indexed sort source (e.g. the promotion pattern) needs a deliberate
  carve-out; (2) the crash-safe-judgment requirement forces the stricter WAL
  durability setting for judgment commits (bulk imports amortize it). Two
  watch-items to measure on the real build: large-scope sorts (~1M entries,
  once per scope change) and sidebar folder counts (must be one aggregate pass
  or lazy, never per-folder queries). One implementation rule: grid pagination
  is keyset, never OFFSET.
