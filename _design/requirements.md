## Functional
- Collections support manual ordering
- Explicit disk-write verbs: metadata edits write to the file/sidecar; remove-from-catalog vs delete-from-disk is an explicit user choice; delete-all-rejects exists
- External-editor round trip modeled on LrC "Edit in Photoshop": the returned file auto-imports and is linked to its original
- Video: trim a clip, save as new file or overwrite (iPhone Photos-style affordance)
- Rescan on demand: user-invoked re-examination of a folder/volume; no background watcher in v1
- Routine multi-file pairs (e.g. RAW+JPEG) collapse to one grid item; the inspector discloses the constituent files; groups can be dissolved
- Grouping automation is consent-based: explicit invocation (e.g. auto-stack by capture time) or opt-in use of camera-embedded bracket metadata; a global toggle disables all auto-grouping
- Full keybinding system, professional-grade: judgments, view switching, navigation, actions — core workflows operable keyboard-only
- NLE/creative project files are importable, trackable, and openable in their owning app; deeper understanding (e.g. enumerating referenced media) only where vendor formats permit, with honest messaging where they don't
- Universal import floor: any file can enter the catalog; unknown or undecodable files still appear (with a generic or decode-failure presentation); richer per-kind experiences layer on top
- Import accepts a directory, a single file, or multiple files
- Migration flow from Lightroom Classic (judgments, collections, keywords); migration from other popular catalog systems considered per-system
- Export presets: format, resize (long edge / short edge / pixel dimensions), quality, and templated file naming
- Keywords: hierarchical, exported to standard XMP/IPTC fields; the single labeling system — no separate "tags" concept. Color labels supported, post-v0
- Import splits into import-critical work (the minimum for a first browsable state) and deferrable enrichment (duplicate detection, face detection, etc. can land after) — design principle for the import pipeline
- Search as a user-facing verb over filenames, keywords, and metadata; full-text search post-v0; semantic/natural-language search (on-device embeddings) definitely in, not necessarily P0
- On-device AI auto-tagging that learns the user's own keyword vocabulary and habits; content never leaves the device except by explicit user action
- Loupe view for a single work, with access to its constituent files; zoom to 100%; compare view
- Lights-out mode (dim everything but the image) and full-screen mode, each a single keystroke
- Video playback handles pro formats well (4:2:2 log, RAW video)
- Batch rename; move/reorganize files between folders from within the app
- Import is in-place only for P0; copy-on-import is a later feature
- Duplicate detection, on-device (import-time "ignore suspected duplicates" per LrC precedent)
- Catalog backup as a user-visible verb: configurable schedule, configurable location (network shares included), retention/pruning
- Undo/redo for judgments
- Offline volumes: browse, search, and judge media on unplugged volumes from the cached index and previews
- Export finishing integrated, not plugins: watermarking, borders, social-format splitting (e.g. panorama → sequence of square tiles)
- Delete-from-disk moves files to the system Trash; the confirmation notes recoverability
- Import progress is deterministic and detailed: the import is sized up front; per-stage progress (cataloged / thumbnails / metadata …) is visible; no fake time estimates — an honest spinner beats a lying progress bar
- Grid sort: user-selectable sort key and direction
- Multi-select everywhere, including discontiguous selections
- Inspector displays work/file metadata
- Face detection: in scope, on-device, explicitly not P0/P1
- AI-assisted culling: on-device detection of technical defects (missed focus, motion blur, over/under-exposure incl. clipped highlights) flags candidates for the user's review; thresholds user-tunable; transparent and consistent — never an opaque "trust us" auto-cull. Photo-first; applicability per media kind
- Text notes as catalog items (an asset with no file — e.g. a note inside a collection)
- Inspector metadata display configurable per media kind (image/video/audio): show/hide and reorder fields — no preset-menu maze
- Catalog topology: multiple catalogs on disk; in-app switching without relaunch; a catalog opens in at most one window at a time (enforced lock); simultaneous catalogs only as fully siloed windows/tabs — no cross-catalog operations, no catalog merging (a cross-catalog search utility is a possible later separate tool)

## Non Functional
Rough targets, not hard specs.

Performance:
- Import: first thumbnails within seconds of pointing at a large source (~40k RAWs on a NAS); the whole set is browsable and cullable before enrichment finishes — no import wall
- Cull loop: keystroke-to-next-preview perceptually instant (~100ms budget; embedded previews)
- Grid scrolling never drops frames at library scale
- Filtering/search over the whole library feels interactive (sub-second)
- Scale target: up to ~1M cataloged items (mixed media: files + sidecars, video, audio, design files, bookmarks, …)

Durability & trust:
- A judgment is durably committed the moment its gesture completes; a crash or power loss a second later loses nothing
- The catalog survives crashes without corruption; a failed integrity check has a recovery path, never a shrug
- User actions give positive feedback: pending vs completed vs failed is always visible

Privacy:
- Content and metadata never leave the device except by explicit user action; optional analytics are anonymous and transparent (see "what even is alexandria?")

Respect:
- Derived data (previews, thumbnails, caches) is bounded and prunable — never silently balloons
- Catalog format documented and parsable; judgments exportable (sidecars, full catalog export) — no lock-in

Responsiveness & resilience:
- The UI never blocks on I/O; a hung network volume never hangs the app; when the app is busy, what it's doing is visible — no mystery freezes
- Interrupted work (import, enrichment) resumes or recovers to a known state after a crash or quit; the catalog is never ambiguous about what completed

Citizenship:
- Alexandria coexists with heavy creative software: background work throttles on battery, thermals, and system load; memory stays proportional to what's visible, not library size; frugal with RAM, storage, and compute generally
- Schema migrations never lose data; upgrades back up the catalog first
- Accessibility: keyboard-first throughout; VoiceOver and contrast basics respected
- Internationalization: built localizable from the start
- Auto-update for the packaged app


## Workflows
- As a creative professional, I want to be able to manage my library, assign judgements to my files, and use alexandria as a hub for managing all of my creative assets.

## BIGGEST OPEN QUESTIONS
- Atomic unit - files or groups? → ANSWERED in the noun round: the asset (neither — the question predated fileless assets)

## My Personal Wish List
- In app RAW photo editing with soft proofing - LrC style. Full fat export tooling, print proofing, and even, dare I say, the ability to replace Epson print layout.
- In app map module to track locations to visit and notes on them, GPX tracks, etc. This would be my current Gaia system, but in Alexandria. Maybe use MapBox
- Shoot/trip planner? Itinerary, locations with timing, etc etc.
- GPX Track Correllation to Files
- Browse video clips in alexandria, create bookmarks at timestamps, then have the bookmarks just show up in your video editor.
- Tethered photo capture
- Websites as an asset type - Eagle's drag and drop images into the DAM
- Browser extension to pull things from the internet into the DAM
- Latex docs
- A more rich and structured filtering and smart collection defining system than what LrC has. It's flat predicates only, and doesn't have much if any support for grouped, nested, or negated conditionals.
- Media integrity checking (ASC MHL?)
- Network share connection management in app

## Nouns (settled in the noun round, 2026-09-09)
- **File** — one thing on disk (a path on a volume). Belongs to at most one asset. (Whether and how files are differentiated within an asset — original vs sidecar etc. — is a schema-round question, deliberately not settled here.)
- **Asset** — THE atom: what the grid shows, judgments attach to, collections contain, search returns. Zero-to-many files (0 = note/bookmark; many = RAW+JPEG+sidecar, image sequence, package). Every imported file yields an asset. Kind-dispatched (photo, video, audio, note, bookmark, project, …).
- **Stack** — two or more assets grouped, shown collapsed with a count badge and a cover (the elected representative displayed while collapsed). Judgments stay on member assets; judging a collapsed stack applies to the cover by default, user-settable to all-members. Created by the user or by consented automation.
- **Catalog** — the database and everything in it; open in at most one window at a time.
- **Volume** — a storage location the catalog knows (drive or network share); may be online (mounted) or offline (unplugged, disconnected, away from home).
- **Folder** — a folder on a volume. The catalog tracks the folders it has imported (that's how you browse by disk location). Folder contents mirror disk truth; creating/moving/renaming folders in-app are disk operations, not catalog-only authorship.
- **Collection** — an authored, named set of assets. One user-facing concept, two kinds: manual (stored membership; manual ordering available, alongside ordinary sorts) and smart (stored predicate — sorting is a view concern, not part of the collection).
- **Import** — the user-invoked event that brings files/assets into the catalog; leaves residue (a browsable scope).
- **Judgment** — class term for rating / flag (pick/reject) / color label.
- **Keyword** — hierarchical label, exported to standard XMP/IPTC fields; the single labeling system.
- **Note** — fileless text asset.
- **Web page** — fileless asset referencing a URL.
- **Marker** — a timestamp on a video asset (NLE vocabulary — it exports to editors).
- Round-trip derivatives (e.g. the TIFF back from Photoshop): a new file yields a new asset (per the universal rule), auto-stacked with its original by default. The user can instead merge it into the original asset as one of its files (merge = the inverse of dissolve). One data model; stack-vs-merge is a user verb, not a setting.

## Small Shops - What do they need/ want?
- Multiple editors working simultaneously off shared storage
- Access control (who can touch what)
- Findability across the shop's archive

How would we do this?
- Modular local vs server based clients?
- Is there a decentralized model that can make this work? Local network p2p communication? Network protocols and decentralized mesh protocols have advanced a lot in recent years, is there something there we can integrate in the future? This would keep certain subsets of files synced across workstations with no central server.

## Backlog (maybe-someday; tracked, not core requirements)
- Aesthetic scoring (experiment-grade — deeply subjective)
- Pinned items in collections (always-first regardless of sort; likely retrofittable)
- MCP / AI-model integration as an opt-in module, off by default
- Metadata export to spreadsheet/CSV
- Slideshows (request-driven only)
- Publish-service API integrations (Flickr/Instagram/etc.)
- Cloud catalog-sync service (Obsidian model): syncs the catalog only, never media files; paid — ongoing service, ongoing cost; the same sync machinery small shops could run in-house

## Backlog (Maybes)
