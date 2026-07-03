# Deferred Features

This document tracks features that were explicitly discussed and deliberately deferred from v1. Each entry includes the rationale for deferral and notes on what would be required to implement it. The schema and architecture accommodate these features — they are not blocked by existing decisions.

---

## P1 — Deferred from v1, planned for early follow-up

### Asset grouping

**What it is:** Related assets — a RAW file and its exported JPEG, a PSD and its exported PNG, multiple crops of the same shot — are presented as a single card in the grid. Expanding the card reveals the group members. Each member has a role (raw, jpeg_sidecar, source, export, member).

**Why deferred:** The grouping logic (detecting which files should be grouped automatically) is non-trivial. Heuristics based on filename similarity and partial hash comparison are needed. The UX for managing groups (creating, breaking apart, reassigning roles) needs careful design. The schema is ready (`asset_groups`, `asset_group_members`).

**What's needed to implement:**
1. Grouping heuristics: filename-based (strip extension, compare base names; same base name with different extensions = potential group), hash-based (partial hash match with different MIME types)
2. UI: group card in grid (stacked card appearance), group detail view showing all members with their roles
3. Commands: `GroupAssetsCommand`, `UngroupAssetsCommand`, `SetGroupCoverCommand`
4. Integration with ingest pipeline: detect grouping candidates at ingest time or as a post-ingest pass

**Schema impact:** None — `asset_groups` and `asset_group_members` tables are present from migration 0001.

---

### Smart collections

**What it is:** A collection whose membership is computed dynamically from a stored query. "All 5-star RAW files from 2023" is a smart collection that always shows the current matching assets. Saved searches and smart collections are the same concept.

**Why deferred:** The query builder (translating a JSON filter definition to SQL) needs careful design to support complex combinations (AND/OR, nested conditions, date ranges). The UI for creating and editing smart collection queries is non-trivial.

**What's needed to implement:**
1. Query definition schema (JSON): `{ "and": [ { "field": "rating", "op": "gte", "value": 4 }, { "field": "file_type", "op": "in", "value": ["raw"] } ] }`
2. Query builder: translates JSON to parameterised SQL SELECT against the assets table using `AssetFilter`
3. UI: smart collection editor with field/operator/value pickers
4. `CollectionRepository.GetAssets()` already accepts `AssetFilter` — the query builder just needs to populate it from the JSON

**Schema impact:** None — `collections.query` column is present from migration 0001.

---

## P2 — Post-v1, requires research

### AI/ML tagging

**What it is:** Automatic tagging based on image content — face detection, object recognition, scene classification. Similar to what Damselfly does with ML models.

**Why deferred:** Requires either on-device ML inference (large model files, GPU/ANE access) or an external ML service (network dependency, privacy concerns). The UX for reviewing and correcting auto-tags is significant design work. Privacy is a hard constraint — user files must not be sent to external services without explicit opt-in.

**What's needed to implement:**
- Decision: on-device (ONNX runtime, Core ML on Mac) vs opt-in cloud service
- Tag source "ai" added to `asset_tags.source` CHECK constraint
- ML pipeline stage in ingest or as a separate background pass
- Face regions: `face_regions` and `persons` tables (schema left as an example in the migrations doc)
- UI: confidence scores, correction workflow, opt-in settings

**Schema impact:** New tables needed for face detection. `asset_tags.source` CHECK constraint needs "ai" value (migration required).

---

### Duplicate review queue

**What it is:** When `duplicate_handling = "review"`, import detects duplicates and queues them for the user to review rather than auto-dropping or auto-importing. The user sees a side-by-side comparison and decides: keep both, keep one, or link as variants.

**Why deferred:** The review queue UI is a distinct screen with complex interaction design. The backend detection is already implemented (dedup checker in the pipeline). The missing piece is the UI and the commands to act on queue items.

**What's needed to implement:**
1. A `duplicate_review_queue` table (or in-memory queue — duplicates are transient, only meaningful during/after an import session)
2. UI: duplicate review screen showing both assets side-by-side with metadata comparison
3. Commands: `ResolveDuplicateCommand` (options: keep_both, keep_first, keep_second, link_as_group)

---

### Integrity check service

**What it is:** A periodic or on-demand check that verifies source files still exist at their expected locations, and that their content (hash) matches what was recorded at ingest. Surfaces missing, moved, and changed files.

**Why deferred:** The UX for presenting integrity check results is non-trivial — you need to show the user what changed and what they should do about it. The background check implementation is straightforward.

**What's needed to implement:**
1. `IntegrityChecker` service: walks each online source, compares mtime/size (fast) or partial hash (slow, optional) against location records
2. Results surfaced as a report: N assets verified, N missing, N changed, N moved (detected via hash match at different path)
3. UI: integrity report screen with actions (re-link moved files, remove missing from catalog, re-ingest changed)
4. Scheduled runs: configurable background integrity check (nightly, weekly)

---

### Batch rename

**What it is:** Rename multiple selected files on disk according to a template (e.g. `{date}_{camera}_{sequence}` → `2024-07-01_Sony_A7IV_001.arw`).

**Why deferred:** Writes to source files on disk. Requires careful UX (preview before apply, confirmation, undo). The file rename is a location update in the catalog — the asset record is unchanged. The rename command must update the filesystem and the location record atomically (or handle partial failures gracefully).

---

### Export / sharing

**What it is:** Export a selection of assets as a zip, generate a web gallery, send to a cloud storage destination.

**Why deferred:** Wide scope. Each export format/destination is a significant feature. No clear v1 user need established.

---

### Telemetry / crash reporting

**What it is:** Optional, opt-in, privacy-respecting crash reporting and usage analytics.

**Why deferred:** Privacy is a hard constraint. The implementation (what is collected, how it is anonymised, where it is sent, how the user opts in) requires careful thought and transparent communication. File paths must never be included in telemetry. The framework (Sentry, self-hosted Plausible, etc.) needs to be selected.

**Requirements when implemented:**
- Explicit opt-in on first launch (not opt-out)
- Clear description of what is collected
- User can view what would be sent before enabling
- No file paths, no file names, no metadata that could identify content
- Easy to disable at any time via settings

---

### Localisation (i18n)

**What it is:** Support for non-English languages in the UI.

**Why deferred:** Significant ongoing maintenance burden (translation updates with every release). Third-party translation management tooling needed. Design must accommodate variable string lengths in UI layouts.

**Constraint:** String literals in application code must not be concatenated (e.g. `"File " + filename + " not found"` would require the translator to work around an awkward structure). Even before localisation is implemented, strings should be structured so they can be extracted to resource files. This is a low-cost discipline to establish early.

---

### Accessibility

**What it is:** Screen reader support (VoiceOver on Mac, Orca on Linux, NVDA/Narrator on Windows), keyboard-only navigation, sufficient colour contrast for WCAG AA compliance.

**Why deferred:** Foundational keyboard navigation (the keyboard-driven workflow already defined) provides some accessibility. Full screen reader support in a Wails/webview app requires ARIA attribute work throughout the UI component tree. This is a significant effort that is easier to address once the UI component structure is established.

**Constraint:** The color label system uses colour alone to convey meaning (Red label, Yellow label, etc.). A shape or pattern alternative must be provided for users who cannot distinguish colours. This should be implemented early as it affects the core labelling UI.

---

### Plugin / extension system

**What it is:** A public API for third-party extensions to add new file format support, custom actions, or integrations.

**Why this is permanently deferred (not just P2):** Explicitly decided against. A plugin system is a significant maintenance, security, and support burden. The API surface must be versioned and maintained forever once published. Contributors add features via code contributions or feature requests. This is a deliberate scope boundary.

---

### Onboarding tour

**What it is:** An in-app guided tour for new users.

**Why deferred:** Online documentation is sufficient for v1. An in-app tour requires significant UI work and becomes outdated as the app changes. The empty state on first launch (with a prominent "Add Source" call to action) provides enough guidance to get started.

---

## Implementation order recommendation

When these features are prioritised, suggested order based on user value and implementation dependency:

1. **Asset grouping** — highest user value for creative professionals with RAW+JPEG workflows
2. **Smart collections** — enables powerful filtering workflows; backend query builder unblocks this
3. **Integrity check** — important for catalog reliability as libraries grow
4. **Duplicate review queue** — backend detection is done; just needs UI
5. **AI/ML tagging** — high user value but requires significant infrastructure decisions
6. **Localisation** — lower priority until user base has clear non-English demand
7. **Accessibility** — should be addressed before any v2 release
8. **Telemetry** — if revenue/sustainability requires product analytics
9. **Batch rename / Export** — nice to have, not core to the DAM value proposition
