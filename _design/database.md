# Database: SQLite via GRDB

**Status: RATIFIED** (Ari, 2026-09-09, architecture reset discussion)

Two decisions, made separately and each earned on its own:

1. **Engine: SQLite.**
2. **Access layer: GRDB** (`groue/GRDB.swift`), not SwiftData, not Core Data.

## Requirements the choice was tested against

Stated during the reset discussion, in Ari's words or close to them:

- **Embedded, single-process.** The app is one native Mac process; the database is a
  library inside it, never a server.
- **Strong consistency / ACID.** The catalog holds hours of the user's judgment work
  (ratings, flags, groups, collections). Durability is the product promise, not a
  checkbox — a catalog that loses judgments destroys trust in the product.
- **SQL-grade querying.** Complex filter predicates over the whole library (smart
  collections), full-text search, sorting at grid scale.
- **Transactional (OLTP) workload shape.** Small inserts and updates, single-row
  lookups, bulk insert during import, paged reads for the grid. Not analytics.
- **No scale-out.** One user, one machine, one catalog. Distributed concerns are
  irrelevant and any machinery for them is dead weight.
- **Performance at library scale.** Benchmark scenario: point the app at ~40k RAW
  files on a NAS; the grid stays interactive during import and scroll never chugs.
  Concurrent reads while the import writer commits are required (WAL).

## Engine survey

| Candidate | Verdict | Why |
|---|---|---|
| **SQLite** | **Chosen** | Meets every requirement; see below for what it uniquely adds. |
| DuckDB | Rejected | Embedded OLAP: columnar, built for scans/aggregation over huge datasets. Explicitly not for transactional CRUD workloads, which is exactly what a catalog is. Would be the answer for an analytics tool; Alexandria isn't one. |
| libSQL | Rejected | SQLite fork whose additions are server mode and cloud sync. We need neither; it's SQLite with someone else's roadmap. |
| Realm | Rejected | Deprecated by MongoDB (2024), SDKs end-of-life 2025. Dead. |
| Couchbase Lite / ObjectBox | Rejected | Commercial, niche, document/object stores — give up the SQL filtering that's on the requirements list. |
| Key-value stores (LMDB, RocksDB) | Rejected | No query layer at all. |
| Document databases as a category | Rejected | "A database of documents" is a pun, not an argument — "document" means different things in the two contexts. The real problem a doc DB would solve here (sparse per-kind metadata) has a standard answer inside SQLite (below). |

What SQLite brings beyond surviving the eliminations:

- Among the most rigorously tested codebases in existence; billions of deployments.
  For the durability promise this is substance, not trivia.
- Single-file catalog: backup is copying one file.
- WAL mode: concurrent reads during write transactions — the grid stays live while
  import commits.
- FTS5 in-engine for full-text search.
- JSON1 + generated columns (the sparse-metadata answer, below).
- Public domain, readable from every language, forever. The catalog is never a
  proprietary artifact.

## Access-layer survey (the fork with live alternatives)

Core Data and SwiftData are also SQLite underneath, so on Apple platforms the real
decision is how Swift talks to the engine:

| Candidate | Verdict | Why |
|---|---|---|
| **GRDB** | **Chosen** | Talks to SQLite almost directly: fastest for reads and bulk writes, full SQL reachable (FTS5, JSON1, generated columns, WAL tuning), typed record structs, and `ValueObservation` — live queries driving the UI, which replaces an entire eventing layer with a library feature. `DatabaseQueue`/`DatabasePool` gives the single-writer discipline as infrastructure. |
| SwiftData | Rejected | ~20x slower than GRDB on inserts in published comparisons; not the safe choice past ~50–70k records — our benchmark scenario imports 40k in one go. Hides the SQL: no FTS5 control, no generated-column promotion, no hand-tuned grid queries. |
| Core Data | Rejected | Mature, but an object graph with faulting and its own ideas; same SQL-is-hidden problem as SwiftData. |

## Patterns adopted with the choice

- **Relational core + JSON metadata column.** Universal fields every file has
  (path, size, kind, timestamps, judgment anchors) are real columns. Per-kind
  metadata (ISO, focal length, duration, sample rate, codec, …) lives in one JSON
  blob per file. Nulls in SQLite cost ~one byte and SELECT lists are controlled, so
  sparse columns were never the problem they appeared to be — but the JSON column
  keeps the schema honest across images/video/audio without per-kind tables.
- **Promotion via virtual generated columns.** Any JSON field that becomes
  filterable or sortable gets a virtual generated column (`json_extract`) with an
  index: B-tree speed, no migration, added exactly when the need appears. A
  filterable capability = a promoted column. This keeps the queryable vocabulary
  explicit and deliberate.
- **One `DatabasePool` per catalog, alive for the app's lifetime.** Single writer,
  concurrent readers (WAL).
- **The database is the single source of truth for UI state**: views observe
  queries (`ValueObservation`); no parallel in-memory model layer.

## Lightroom's disease (research — inputs to future design sessions, not decisions)

Lightroom Classic also uses SQLite, so the engine choice alone immunizes
nothing. Failure vectors observed in the survey:

- **Corruption** clusters around cloud-sync software syncing a live database
  file it doesn't understand, mid-write crashes, and [catalogs placed on
  network shares — which Adobe flatly does not support, while users routinely
  complain about that restriction and resort to iSCSI/manual-sync
  workarounds](https://www.lightroomqueen.com/community/threads/share-catalog-between-pcs-local-and-nas-location.46234/).
  Catalog-on-NAS is simultaneously a corruption vector and a wanted
  capability — a design tension, not a settled call.
- **Performance rot** comes from unbounded accretion (preview caches, edit
  history), queries that degrade with library size, and an app layer whose
  memory grows with the library.

Countermeasures belong to future design sessions. Discussed 2026-09-09 and
agreed as good directions: WAL + single writer (already part of the GRDB
pattern above); catalog defaulting to a local path.



- DuckDB vs SQLite workload guidance: [DataCamp comparison](https://www.datacamp.com/blog/duckdb-vs-sqlite-complete-database-comparison), [MotherDuck comparison](https://motherduck.com/learn/duckdb-vs-sqlite-databases/)
- Realm deprecation: [realm-swift discussion #8680](https://github.com/realm/realm-swift/discussions/8680), [Atlas Device Sync EOL notice](https://www.mongodb.com/community/forums/t/atlas-device-sync-end-of-life-and-deprecation/296687)
- Swift persistence layer comparison: [GRDB vs SwiftData vs Core Data (2026)](https://www.pistack.xyz/posts/2026-08-11-grdb-swiftdata-core-data-swift-persistence-comparison/), [iOS databases guide](https://fractal-dev.com/blog/ios-databases)
- JSON promotion pattern: [generated columns + indexes](https://reinketechnology.com/fast-json-queries-in-sqlite-using-generated-columns-and-indexes/), [virtual columns + indexing](https://www.dbpro.app/blog/sqlite-json-virtual-columns-indexing)
- GRDB: [GRDB.swift](https://groue.github.io/GRDB.swift/)
