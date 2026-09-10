# Learnings from the first architecture (2024–2026 Go-core repo)

**Status: RATIFIED except where marked PROPOSED** (distilled from the architecture
reset discussion, 2026-09-09). Scope rule: only what was explicitly discussed and
agreed in that conversation appears here. Everything else in the old repo is
**archaeology** — it stays on disk, and a concept is pulled forward only when Ari
invokes it during a design discussion ("what we had previously was a decent
model"). Concepts cross as prose; **code never crosses** (ported code smuggles old
architectural assumptions into new discussions).

## Do not recreate

Each entry names the mistake and the mechanism of the damage — the list matters as
much as the survivors.

- **The process seam.** Go core + shell over loopback HTTP/SSE, OpenAPI contract,
  codegen on both sides. Forced every concept to exist three times (DB row, contract
  shape, client shape) plus mapping and versioning between them; most design spin in
  the old repo was this tax, not the domain. Justified only by cross-platform reach
  we no longer want. One process, one language, one struct.
- **Import/enrichment as separate subsystems.** There is one user verb: *import* —
  "these disk files become catalog files, ready to use." A thumbnail is part of
  ready-to-use. Responsiveness (grid populates in seconds, thumbnails fill in
  behind) is *phases of one pipeline* — internal, invisible in the vocabulary. The
  mistake was promoting an implementation phase into a named engine with its own
  governance.
- **Watch/reconcile as P0.** The watcher dragged in the hardest machinery in the
  repo (reconciler, identity matrix, event-hint discipline) to answer "what changed
  while I wasn't looking?" — which v1 answers with **explicit rescan on demand**.
  FSEvents arrives later as an additive trigger for the same rescan path. V1 still
  needs a graceful "missing" presentation state for gone-at-render-time files.
- **The governance apparatus.** CONSTANTS + decision log + tracking directories +
  multiple CLAUDE.mds + memory files, loaded into every session before any work.
  Grew partly to police the seam (dies with the seam), partly by habit. PROPOSED
  ceiling for this repo: one CLAUDE.md, ~one page, hard cap; invariants enforced by
  compiler/lint/tests wherever a fence is possible — prose only for what can't be a
  fence.
- **Photos-first schema.** Metadata columns designed around photos, other kinds
  bolted on. Replaced by the relational-core + per-kind JSON namespace pattern
  ([database.md](database.md)).
- **Roadmap inheritance.** The old P0–P4 roadmap encodes old-architecture
  assumptions. Requirements are redefined from scratch (next artifact), with
  performance as functional requirements ("40k RAWs on a NAS: grid interactive in
  N seconds"), not vibes.

## Concepts ratified in the reset discussion

Only what was explicitly discussed and agreed. Anything not listed is archaeology.

- **Split truth.** Disk owns bytes and existence; the catalog owns judgments and
  organization, plus a *rebuildable index* of observations (rebuild path: rescan);
  XMP bridges outward. Referenced files always — never copy into an app-owned
  bundle (disrespects disk space, forecloses "your files are just files",
  unworkable at multi-TB).
- **File rows are an index + judgment anchor, not a copy.** ~Hundreds of bytes per
  file buys library-wide queries, offline-volume browsing, and a stable identity
  for judgments to attach to (a path string is not an anchor; renames orphan it).
  Index the *queryable surface* only; disk answers the long tail on demand.
- **Directories vs collections: unify presentation only.** Both render as a grid of
  files; in the data model a folder's contents are *observed* (disk truth) and a
  collection's membership is *authored* (judgment) — those never entangle in
  storage. Smart collections store the predicate, never the membership.
- **Metadata vocabulary** *(amended 2026-09-10, Ari — the original overstated a
  blanket Spotlight ban he never made)*: Alexandria's field catalog is its own,
  with its own names; Spotlight's `kMDItem*` attribute set is one reference
  checklist among others (exiftool tag groups, IPTC Core) when populating it.
  NSMetadataItem is not the catalog's extraction path and its attribute model
  doesn't enter the database (indexing is unreliable on network volumes — NAS is
  the core scenario — and importer-dependent across machines); Spotlight APIs
  elsewhere are fine where they earn their place. Extraction via
  ImageIO/AVFoundation natively, exiftool as subprocess for the long tail.
- **Portability promise (Ari's framing).** Files stay exactly where the user put
  them; judgments are exportable — sidecars, write-to-file where the format allows,
  eventually full catalog-to-JSON export. The promise is about the user's data,
  not the catalog file format.
- **Embedded-preview-first import** (explicitly pulled forward by Ari,
  2026-09-09). Old Alexandria's import pipeline extracted the embedded JPEG every
  RAW already contains instead of decoding the RAW — the same trick behind Photo
  Mechanic's culling speed. The general import-performance thinking there was
  good; carry the *approach* (previews first, cheap before expensive), re-derive
  the pipeline itself fresh. Don't over-index on the old implementation.

## Parked (named, not designed)

- **Metadata source normalization** — many sources (EXIF/IPTC/XMP/sidecar), one
  logical field. Prior art: Metadata Working Group precedence (roughly XMP > IPTC >
  EXIF), exiftool composite tags. A post-requirements design item; adopt a
  precedence policy per field, don't invent reconciliation.
- **Watcher** (post-v1, additive trigger on the rescan path).
- **Linux/cross-platform** — closed, not parked: Alexandria is a Mac app. The data
  substrate (files in place, SQLite, sidecars) is the only hedge, and it's enough.
