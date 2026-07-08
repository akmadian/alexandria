Requirements
- Functional
- Non Functional

Availability vs Consistency - Consistency is much more important.

Core Entities
- Catalog
- Asset
- Asset Group - A group of assets based on the assets' relation to each other. A RAW+JPG+XMP sidecar group, for example. Most groups are of an "auto" origin when auto grouping was done.
- Collection - A colletion of assets based on some set of user defined filtering rules. Filtering by many potential feilds with nested conditional definitions.
- Stack - A user-defined group of assets. Not recomputable, user judgement based. Stacks are also asset groups, but with a "manual" origin.
- Tag

System Components
- Server/ Backend
    - Storage
    - Media processing pipelines
        - File decoding - contracted out to subprocesses for isolation, memory and runaway, easy killing. We can also lean on the knowledge repos that are packages like exiftool. Subprocess cons are 1. spawn overhead (use exiftool daemon mode and batch processing to mitigate for lighter files, video is heavy enough that spawn time is noise) 2. Dependency distribution - this is tough, I'm leaning towards in app UX for user spawned fetching and downloading where possible, or doing automatically on first launch.
        - Registry instead of hierarchy for handling per filetype noise. We reduce complexity by only supporting the core, most important metadata fields that we actually care about for filtering and complex interaction, while normalizing and bundling everything else into a json blob. Fields earn a real column when UX demands it with filter or sort. Normalization happens once at extraction time, never at query time, and never in ui. Frontend also gets a registry for fanout from normalized type to presentation.
    - Query/ search
    - Watcher/ reconciler
    - XMP Sync
    - Jobs/ Scheduler
- Client/ Frontend

---
## Observability
### Development
- What pieces of information might a dev or contributor want to know about the system state? Why?
- Queue depth in pipelines - monitoring backpressure, performance stats (average vs last vs p9x time to clear an event from pipeline stage)
- Sensor visiblity on sources, pull vs push strategies for watching
- FSEvents feed
- Skip/ drop events in ingest pipeline with reason, count by file type, etc.

### User
- What pieces of information might the user want to know about the system state? Why?

---
## Scenarios to Game Out
### New Filetype
- New entry in backend registry, plus new handlers as needed.
- New entry in frontend registry, plus new react UI components as needed.

### Failure Modes
- External storage interruptions. What if an external drive accidentally disconnects during import? What if network connection drops? How do we keep state healthy and surface the issue to the user?

---

## Notes on Go
### Concurrency
- Goroutines are threads/ workers.
- Goroutine worker pools orchestrate, pure go decides safe common formats in process, everything exotic goes through subprocess fleet.
- Go channels with buffering are essentially queues for workers/jobs?
- Goroutines can be awaited with waitgroups https://gobyexample.com/waitgroups

### Generics(?)


---
## The note dump
- Downloading binaries for user is a big security risk. We need integrity checks with checksums, https, signature verification, etc. The UX of offering to download for the user is the right way to go. We should try to autodetect in path on startup (and maybe in common locations as well?)
- Interfaces are for varying BEHAVIOR behind a standardized contract. Generics are for verying DATA used in the same way - generics are for code that is parametric over DATA TYPES. Interfaces are for code that is parametric over BEHAVIOR.

---
## Next Up
Backend → seam → frontend is right (the seam is derived from what the engine can promise; designing it first inverts the dependency). Remaining backend deep dives, in dependency order:
- Ingest pipeline end-to-end — stage topology, batching/transactions, where grouping recompute and the job model attach, backpressure. (Most connected; do first.)
- Watcher/reconciler — event semantics, debounce, the converge model, volume identity.
- XMP sync — the conflict model; hardest correctness problem in the system.
- Catalog/query layer — schema finalization, FTS wiring, the query builder that smart collections will reuse.