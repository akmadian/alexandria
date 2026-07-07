# NOTICE TO AGENTS WORKING IN THIS REPO

**The design documents in this folder — everything EXCEPT `v2/` — are a previous revision,
superseded on 2026-07-06 by the v2 design handoff:**

→ **Start at `docs/v2/post-ingest-design/00-START-HERE.md`**

The v2 decision log (`02-decision-log.md`) is authoritative. Where these older documents conflict
with it, **the decision log wins, always**. Known conflicts include (non-exhaustive): the
keybindings DB table (dropped in v2), the standalone app-maintained FTS design (v2: trigger-
maintained), `sources.status` (v2: split into `enabled` + `connectivity`), localStorage routing for
UI persistence (v2: catalog-scoped settings KV), and the sequential importer design (v2: concurrent
pipeline, spec in `v2/.../impl/04-ingest-pipeline.md`).

These older docs remain useful for: the functional requirements (`functional-requirements.md` is
still the feature source of truth), background rationale, coding guidelines, and frontend
architecture (frontend work is deferred; its docs have not been revised). Read them for context;
do not implement from them where v2 has spoken.
