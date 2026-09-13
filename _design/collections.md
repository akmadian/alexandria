# Collections: round record

**Status: RATIFIED** (collections design round, 2026-09-12). This is the
round's decision record — rulings, alternatives considered and why they
lost, the approved schema, and the query shapes walked. Register: every
ruling below is Ari's; research citations ground them. The build lands in
chunks, each on its own explicit approval.

## The noun (extends requirements.md's entry)

A **collection** is an authored, named set of assets — and it may contain
collections. ONE noun: the same thing holds member assets and child
collections alike; there is no separate "set"/"group"/"folder-of-albums"
type. Clicking a collection shows everything beneath it (union of its own
members and its descendants'), the same display rule folders ratified.
Multiple roots are allowed. Membership is the user's work product
(judgment-class: the durability promise applies).

## Rulings, with alternatives considered

1. **One noun, not the field's two** (collection + container). LrC
   (collection vs collection set) and Apple Photos (album vs folder) both
   split the noun — and both walls are documented user pain: you can't
   drag photos to a set ([workaround culture around the target
   collection](https://www.lightroomqueen.com/community/threads/adding-to-collections.41054/)),
   and folders-vs-albums is a standing confusion genre
   ([Apple community](https://discussions.apple.com/thread/255487777)).
   The shape users already carry is the disk folder: holds items AND
   sub-things. A members-less collection *behaves* as a set with zero
   extra type. Rejected: the two-noun shape (more machinery, a wall to
   explain); polymorphic membership (children are NOT members — the tree
   hangs off `parent_id`, membership is asset-only, the two never share a
   table).

2. **Smart collections deferred to the filter round, retrofit seam
   settled.** A smart collection is not a different kind — it's a
   collection whose membership comes from a predicate. It arrives as a
   nullable `predicate` column (NULL = manual); it lives in the same
   tree; it takes NO manual adds (want smart + extras: compose a smart
   and a manual collection side by side under a parent). Membership rows
   therefore always mean *manual* membership. Prior art: LrC stores all
   three kinds in ONE table (`AgLibraryCollection`) discriminated by
   `creationId`, predicate serialized, membership computed
   ([lrcat format](https://github.com/hfiguiere/lrcat-extractor/blob/main/doc/lrcat_format.md)) —
   and learnings.md already ratified "smart collections store the
   predicate, never the membership." Rejected: designing the predicate
   now (the filter round's job; a speculative column is a smuggled
   field).

3. **Manual order = fractional indexing, Figma as the reference
   implementation.** One TEXT `order_key` per membership row, base-62
   `[0-9A-Za-z]`, BINARY-compared; insert-between mints a key between the
   neighbors; a reorder touches only the dragged rows. Minted
   append-at-end on add, so "order added" IS the manual order until the
   first drag — no null/uninitialized-order state. Rejected, each with a
   shipped failure attached:
   - *Float positions* — LrC pre-v10: 53-bit mantissa exhaustion after
     ~52 insert-betweens, silent identical positions, orderings corrupted
     ([the 52-reorderings bug](https://community.adobe.com/t5/lightroom-classic-bugs/p-limit-of-52-reorderings-in-custom-ordered-collections-and-folders/idi-p/12250073)).
   - *Integer positions with renumbering* — every insert rewrites the
     tail of the collection; a reorder stops being one durable gesture.
   - *LrC v10's string variant* — right idea, but a 1023-char cap and a
     `-` boundary character with nothing before it (renumber storms).
     Our alphabet has no boundary hazard and no cap; the reference is
     [Figma's arbitrary-precision scheme](https://www.figma.com/blog/realtime-editing-of-ordered-sequences/).
   - *An index→id position table* — struck as smell (bulk state for a
     per-row fact).
   Known ceiling, accepted: repeated same-gap insertion grows key length;
   the answer is a one-collection rebalance rewrite, `// PERF:`-marked at
   build, not pre-built.

4. **Manual order is an `Arrangement.Key`**, offerable only for
   collection sources. Switching to a regular sort is an explicit,
   non-destructive act — the keys sit untouched on the membership rows,
   so returning to manual restores it exactly. Drag-to-reorder is live
   only while arrangement = manual; in any other sort the gesture offers
   the switch, never a silent no-op and never a silent mode flip (LrC's
   ["does not support custom order" mystery](https://asktimgrey.com/2017/09/12/custom-sort-unavailable/)).

5. **Union view order = the sectioned heuristic, single and
   deterministic**: depth-first walk of the subtree; a collection's own
   members first, in `order_key` order; children in sidebar (Finder)
   order; a duplicate asset keeps its first appearance. Sidebar and grid
   share ONE Swift-side walk (Finder ordering is
   `localizedStandardCompare`, which SQL cannot compute — so the subtree
   is resolved and ranked in Swift, then joined; the two surfaces cannot
   disagree). Rejected: LrC parity (no manual order on parents — a
   wall); showing only own members in manual (hides data); byte-order
   sections in SQL (would diverge from the sidebar).

6. **No reorder in a union view.** A child's member has no key in the
   parent, so the drag has no honest meaning — the alternatives were
   minting a membership as a drag side effect (surprise adds) or a
   shadow per-parent order (a second ordering concept). You reorder a
   collection's own members in that collection. The explicit escape, if
   ever wanted: add the union's assets to the parent as its own members,
   then order them.

7. **Move verb, with cycle refusal.** Collections re-parent (users
   reorganize; Photos users complain they can't move albums between
   folders). Re-parenting a collection under its own descendant is
   refused inside the verb's transaction. (Folders never needed this —
   disk truth doesn't re-parent — so the guard is new here.)

8. **Delete = designed verb + confirm** (subtree summary shown first),
   hard delete; memberships cascade; assets untouched. Deleting the
   currently-viewed subtree retargets the source to `.library`.
   Soft delete: not now, retrofittable.

9. **No name uniqueness anywhere.** Authored names are the user's
   business — Apple Photos allows identical sibling album names with no
   complaint genre attached. Identity is the id; nothing ever looks a
   collection up by name. This also deleted `name_key` (no consumer
   left) and both partial unique indexes, and with them the lurking
   costs: locale-sensitive case folding, rename-to-own-name edge,
   LrC-migration rename policy. Floor: empty/whitespace names rejected
   at the verb. Rejected: the folders-style unique `name_key` — folders'
   uniqueness is disk truth and their key is find-or-create identity;
   collections share neither property.

10. **Membership unit = assets** (the ratified atom). Files lens inside
    a collection = the member assets' files, an asset's files adjacent
    in its manual slot. This dodges LrC's stacks/pairs-in-collections
    confusion class structurally: the pair is one asset everywhere.

11. **Out of this round, marked**: per-source arrangement memory (a
    TODO lands in the hub; prior art says per-source stickiness is
    expected — future round); manual order for folders (different
    mechanism if ever); undo/redo (undo round; the delete verb will
    want a subtree snapshot then); pinned members (verified
    retrofittable: one column + a sort prefix); sidebar tree manual
    ordering; target-collection/keyboard-add gesture; collection counts
    in the sidebar; stacks-in-collections display; import-time
    add-to-collection; covers; export/publish of collections;
    multi-writer key merging (sync's problem; TEXT keys take
    writer-jitter without schema change).

## The approved schema

```sql
CREATE TABLE collections (
    id        TEXT PRIMARY KEY,
    parent_id TEXT REFERENCES collections(id) ON DELETE RESTRICT,  -- NULL = a root
    name      TEXT NOT NULL  -- [jdg] authored, as typed; never an identity
);

CREATE INDEX idx_collections_parent ON collections(parent_id);

CREATE TABLE collection_members (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    asset_id      TEXT NOT NULL REFERENCES assets(id)      ON DELETE CASCADE,
    order_key     TEXT NOT NULL,  -- [jdg] fractional index, base-62
    PRIMARY KEY (collection_id, asset_id)
);

CREATE UNIQUE INDEX idx_collection_members_order ON collection_members(collection_id, order_key);
CREATE INDEX        idx_collection_members_asset ON collection_members(asset_id);
```

Both tables are pure judgment-class — the first in the schema where
every column is the user's work product. Constraint rationale:

- Composite PK: duplicate membership structurally impossible; bulk add
  is idempotent (`INSERT OR IGNORE`, report the new count).
- `UNIQUE (collection_id, order_key)`: LrC's silent identical-position
  rot made a loud transaction failure. Under the single writer a
  collision can only be a minting bug. Same index carries the ordered
  read.
- `idx_collection_members_asset`: the reverse verb ("which collections
  hold this asset"), remove-asset cascades, and the future
  "not in any collection" predicate.
- CASCADE on both membership FKs (meaningless without either parent);
  RESTRICT on `parent_id` (subtree delete is a designed verb, never an
  accidental cascade).

## The records

`Collection` (Identifiable, CatalogRecord) and `CollectionMember`
(CatalogRecord, composite key — not Identifiable) in Core/Models/,
explicit CodingKeys + Columns per the record convention. Note:
`Collection` shadows `Swift.Collection` for unqualified lookup inside
the module; no current code is affected (checked), and stdlib-generic
code can qualify. Accepted for vocabulary fidelity.

## Query shapes (walked in the round; hot paths traced at the 1M target)

- **Source.collection**: subtree resolved Swift-side (collections table
  is small; Finder-ranked walk shared with the sidebar), then one
  statement joining ranked collection ids against `collection_members`
  — O(members), not O(catalog). Same CTE-family shape as the
  review-blessed folder subtree.
- **Manual read**: `ORDER BY order_key` rides `idx_collection_members_order`;
  the ordered scan IS the index scan.
- **Files lens**: `JOIN files ON files.asset_id`, ordered by member key
  then file id.
- **Reverse lookup**: one indexed read.
- **Bulk add 10k**: one tail-key read, keys minted in memory, one
  transaction. **Reorder of M**: M row updates. Every verb = one
  transaction = one durable gesture = one observation delivery.
- **Observation hygiene**: GRDB region-tracks per query — library and
  folder sources never read `collection_members`, so collection edits
  cannot re-deliver their working sets.
- Named assumption with its ceiling: "collections number in the
  hundreds/low thousands" — the Swift-side assembly shares the browser
  round's recorded ~10k-row sidebar ceiling.

## Build plan (chunked, each chunk on Ari's explicit approval)

1. Schema tables + Swift records. ← approved 2026-09-12
2. `OrderKey` (pure) + catalog verb files + tests.
3. `Source.collection` + `Arrangement.Key.manual` + hub guard + TODO.
4. Sidebar section + verbs UI + drag-to-add.
