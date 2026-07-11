# xmp — bidirectional sidecar sync

Implements D15 (see `docs/decisions.md`). exiftool via the dependency fleet (`-stay_open`
daemon), both directions. We never hand-parse RDF/XML — multiple legal serializations, every
writer's dialect differs; exiftool absorbed all of it. exiftool missing → the feature reports
unavailable and degrades per D5.

## Scope & the documented asymmetry

READ: sidecar `.xmp` files + embedded XMP (embedded comes via normal ingest extraction).
WRITE: **sidecars only** — never asset files (reference model). Consequence to document for
users: LrC ignores JPEG sidecars (expects embedded), so RAW interop is fully bidirectional;
JPEG write-back waits for P2 metadata-editing (explicit per-user opt-in to file modification).

## Field map

| Catalog | XMP | Notes |
|---|---|---|
| rating | `xmp:Rating` | clean; range-guarded 0..5 (unrepresentable "rejected" -1 clears) |
| color_label | `xmp:Label` | STRING, locale-dependent (LrC writes "Rot" on German systems). `NormalizeLabel`: EN/DE/FR/ES/IT/JA → the canonical six; unknown strings preserved round-trip, never dropped, label left unset, logged |
| tags | `dc:subject` + `lr:hierarchicalSubject` ("Travel\|Japan\|Tokyo") | hierarchy nodes auto-created; merge-only (below) |
| caption (observation col) | `dc:description` | syncs to `caption`; `note` stays Alexandria-private. Inbound apply NOT yet wired (see the XMP-sync task) |
| title | `dc:title` | same pending state as caption |
| flag | `alexandria:Flag` (custom namespace) | not yet read/written. LrC has NO flag in XMP; best-effort survival through our flows; LrC MAY strip it on rewrite (empirical test: open question #8). NEVER auto-map flags onto ratings/labels — lossy mappings are an opt-in P3 toggle only |

## Conflict model — file-level 3-way (`conflict.go`)

Base state = the asset's sync-state columns. Per asset with a sidecar:

- sidecar changed? current sidecar hash ≠ `assets.xmp_hash`
- catalog changed? `judgment_modified_at` > max(`xmp_last_read_at`, `xmp_last_written_at`)

| Sidecar Δ | Catalog Δ | Action |
|---|---|---|
| no | no | no-op (the overwhelming case; sync passes are ~free) |
| yes | no | apply inbound |
| no | yes | write outbound (only if `xmpWriteBack` enabled) |
| yes | yes | conflict → setting `xmpConflictResolution`: `xmp_wins` (default) \| `catalog_wins` |

Inbound apply is **wholesale**: under `xmp_wins` the sidecar is authoritative including its
removals — a field it omits CLEARS the catalog value (matching LrC "Read Metadata"). Safe
because the 3-way grid already routes any user judgment newer than the last sync to a conflict,
so a plain apply only clears already-synced state, i.e. a genuine sidecar removal.

**Tags exception: always union, both directions, never delete** (absence ≠ deletion in a merge).
`asset_tags.source='xmp'` marks synced tags forever. Runs on any sidecar change regardless of
the judgment policy.

File-level granularity is deliberate (matches LrC's own wholesale behavior). Upgrade path if
coarse conflicts annoy: `xmp_base` JSON snapshot column (sync-state class) → per-field 3-way
(open question #14).

## Loop prevention — two levels, both mandatory

1. **File level** (lives in the watcher wiring): after writing a sidecar, its hash is stored in
   `xmp_hash`; an inbound sidecar hint hashing to that value is our own echo → dropped.
2. **State level**: inbound applies go through `AssetSyncWriter.ApplyXMPInbound`, which writes
   judgment VALUES but **never bumps `judgment_modified_at`**. If it bumped it, every inbound
   would look like a user edit → outbound write → new file hash → inbound → oscillator. This is
   the writer-class system's whole justification; do not "simplify" it away.

## Triggers

Inbound: at ingest (`Importer.OnAssetCommitted`) · watcher sidecar hint post echo-check
(`Watcher.SidecarChanged`) · manual "Read metadata". Outbound (only when `xmpWriteBack`):
judgment change, debounced per asset ~2s (`WriteBackDebouncer`) so a 50-asset triage session is
50 writes total, not per-keystroke · P2 bulk "Save metadata to files".

Settings (`xmpWriteBack`, `xmpConflictResolution`) are read live via a
`func() settings.Settings` accessor — hot-reload for free.

## Writes

Merge-into-existing: only our fields touched; develop settings and foreign namespaces byte-
preserved; sidecar created if absent (when write-back on). Atomic temp + rename (our own
watcher sees the rename; the echo check absorbs it).
