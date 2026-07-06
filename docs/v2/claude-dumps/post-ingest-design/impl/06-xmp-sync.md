# impl/06 — XMP Sync

**Status: design complete; requires impl/05 (watcher, for sidecar hints) and impl/07 (exiftool).**
**Scope:** new `internal/xmp`. **References:** D15, `03-data-model.md` §1.

## Scope & the documented asymmetry

READ: sidecar `.xmp` files + embedded XMP (embedded comes via normal ingest extraction).
WRITE: **sidecars only** — never asset files (reference model). Consequence to document for users:
LrC ignores JPEG sidecars (expects embedded), so RAW interop is fully bidirectional; JPEG
write-back waits for P2 metadata-editing (explicit per-user opt-in to file modification).

## Field map

| Catalog | XMP | Notes |
|---|---|---|
| rating | `xmp:Rating` | clean |
| color_label | `xmp:Label` | STRING, locale-dependent (LrC writes "Rot" on German systems). Normalize known vocabularies (EN/DE/FR/ES/IT/JA at minimum) → canonical six; unknown strings preserved round-trip, never dropped, label left unset, logged |
| tags | `dc:subject` + `lr:hierarchicalSubject` ("Travel\|Japan\|Tokyo") | hierarchy nodes auto-created; merge-only (below) |
| caption (observation col) | `dc:description` | syncs to `caption`; `note` stays Alexandria-private |
| title | `dc:title` | clean |
| flag | `alexandria:Flag` (custom namespace) | LrC has NO flag in XMP. Best-effort: survives our bundle/migration flows; LrC cannot display it and MAY strip it on rewrite (empirical test — open question #8). NEVER auto-map flags onto ratings/labels; lossy mappings are an opt-in P3 toggle only |

## Conflict model — file-level 3-way

Base state = sync-state columns. Per asset with a sidecar:

- sidecar changed? current sidecar hash ≠ `assets.xmp_hash`
- catalog changed? `judgment_modified_at` > max(`xmp_last_read_at`, `xmp_last_written_at`)

| Sidecar Δ | Catalog Δ | Action |
|---|---|---|
| no | no | no-op (the overwhelming case; sync passes are ~free) |
| yes | no | apply inbound |
| no | yes | write outbound (only if `xmpWriteBack` enabled) |
| yes | yes | conflict → setting `xmpConflictResolution`: `xmp_wins` (default) \| `catalog_wins` |

**Tags exception: always union, both directions, never delete** (absence ≠ deletion in a merge).
`asset_tags.source='xmp'` marks synced tags forever.

File-level granularity is deliberate (matches LrC's own "Read Metadata" wholesale behavior).
Upgrade path if coarse conflicts annoy: `xmp_base` JSON snapshot column (sync-state class) →
per-field 3-way. Named, deferred (open question #14).

## Loop prevention — two levels, both mandatory

1. **File level** (lives in watcher, impl/05): after writing a sidecar, store its hash in
   `xmp_hash`; inbound sidecar hint hashing to that value = our own echo → drop.
2. **State level**: inbound applies go through `AssetSyncWriter.ApplyXMPInbound` which writes
   judgment VALUES but **never bumps `judgment_modified_at`** (impl/02). If it bumped it, every
   inbound would look like a user edit → outbound write → new file hash → inbound → oscillator.
   This is the writer-class system's whole justification; do not "simplify" it away.

## Triggers

Inbound: at ingest (sidecar present when asset commits) · watcher sidecar hint (post echo-check) ·
manual "Read metadata". Outbound (only when `xmpWriteBack`): judgment change, **debounced per
asset** (~2s quiet) so a 50-asset triage session = 50 writes total, not per-keystroke · P2 bulk
"Save metadata to files".

## Mechanics

exiftool via the dependency fleet, `-stay_open` daemon, both directions. Do NOT hand-parse RDF/XML
(multiple legal serializations; every writer's dialect differs; exiftool absorbed all of it).
Writes are **merge-into-existing**: only our fields touched; develop settings and foreign
namespaces preserved; sidecar created if absent (when write-back on). Atomic: temp + rename (our
own watcher sees it; echo check absorbs it). exiftool missing → feature reports unavailable,
degrades per D5.

## Acceptance

- Round-trip: LrC-authored sidecar fixture → inbound applies rating/label/keywords/caption;
  no `judgment_modified_at` bump; second pass = no-op.
- Echo: write outbound → simulate watcher hint on that file → verify dropped, no re-read.
- Oscillator test: enable write-back, apply inbound, verify NO outbound fires.
- Conflict: mutate both sides → policy applied per setting; tags unioned regardless of policy.
- Locale: "Rot"/"Rouge" fixtures normalize; unknown label string survives outbound rewrite intact.
- Merge-preservation: sidecar with LrC develop settings → our write leaves them byte-preserved
  (compare foreign subtree pre/post).
