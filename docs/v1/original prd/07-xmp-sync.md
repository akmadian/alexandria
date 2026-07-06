# XMP Sync

## Overview

XMP (Extensible Metadata Platform) is Adobe's standard for embedding metadata in files. It is the interchange format between Alexandria and Lightroom Classic (and any other XMP-aware tool). Rather than trying to read Lightroom's proprietary catalog format, Alexandria uses XMP sidecar files as the handshake layer.

This document covers: what XMP contains, where it lives, how Alexandria reads and writes it, and how conflicts are resolved.

---

## What Alexandria syncs via XMP

XMP contains many fields. Alexandria only cares about the portable subset relevant to creative asset management:

| XMP Field | Alexandria Field | Notes |
|---|---|---|
| `xmp:Rating` | `assets.rating` | Integer 0–5 |
| `xmp:Label` | `assets.color_label` | String: "Red", "Yellow", "Green", "Blue", "Purple" |
| `dc:subject` | `asset_tags` (keywords) | Array of strings |
| `dc:description` | `assets.note` | Per-asset note. Maps to Lightroom's "Caption" field. |
| `xmp:CreateDate` | `assets.captured_at` | **Fallback only.** EXIF `DateTimeOriginal` is authoritative for capture date; `xmp:CreateDate` is used only when the file has no EXIF capture date. It is a creation date, not strictly a capture date, and tools disagree on its semantics. |

**Color label caveat:** Lightroom Classic has five labels (Red, Yellow, Green, Blue, Purple) — no Orange. Alexandria's Orange label is catalog-only: it is never written to XMP, and an outbound sync for an orange-labelled asset writes no `xmp:Label` (leaving any existing value untouched). This is documented in the UI where labels are configured.

**Not synced:** Lightroom develop settings, crop data, local adjustments, collections, virtual copies, history. These are stored in Lightroom's own catalog format (`Lightroom Catalog.lrcat`) and are not part of the XMP spec. Alexandria neither reads nor writes these.

---

## Where XMP lives

XMP metadata can live in different places depending on the file type:

| File Type | XMP Location | Notes |
|---|---|---|
| RAW (.arw, .cr3, .nef, .dng, etc.) | Sidecar `.xmp` file | Alongside the RAW file. Standard for RAW. |
| JPEG | Embedded in file (APP1 segment) OR sidecar | LrC embeds by default; sidecars are preferred if present |
| TIFF | Embedded | |
| PSD | Embedded | |
| Video | Sidecar `.xmp` only | Video containers have no standard XMP embedding location |
| Affinity (.afphoto, .afdesign, .afpub) | Sidecar only | Never write into Affinity files |
| InDesign (.indd) | Sidecar only | Never write into InDesign files |
| SVG | Embedded or sidecar | |

**Sidecar preference rule:** If a sidecar `.xmp` file exists alongside the source file, it takes precedence over any embedded XMP. This is what Lightroom writes and what other tools expect.

**Never write into proprietary files:** Alexandria will never attempt to write embedded XMP into Affinity, InDesign, or other proprietary format files. For these, sidecar-only is the policy.

---

## Label normalisation

Lightroom's color label strings are locale-dependent. In an English install, they are "Red", "Yellow", "Green", "Blue", "Purple". In other locales they may differ. Alexandria normalises on read by lowercasing and trimming:

```
"Red"   → ColorLabelRed
"YELLOW" → ColorLabelYellow
"Rojo"  → ColorLabelRed (if Spanish locale normalisation is implemented)
```

The reverse mapping (domain label → XMP string) always writes the English string, which is the universal default. This means a catalog built with a non-English Lightroom install and synced to Alexandria will normalise to English XMP strings on write-back. This is an acceptable limitation for v1.

---

## XMP sync triggers

### Inbound (XMP → Alexandria catalog)

XMP is read in the following situations:

1. **At ingest time:** When a file is imported, Alexandria reads any available XMP (sidecar or embedded) and populates rating, label, and keywords in the catalog immediately.

2. **When a `.xmp` file changes:** The file watcher detects changes to `.xmp` sidecar files. When a `.xmp` file changes:
   - Find the asset whose source file corresponds to this sidecar (strip `.xmp` extension, look up by path)
   - Read the sidecar
   - Compare its hash to `assets.xmp_hash`
   - If changed: run conflict resolution, apply if appropriate, update `xmp_hash`

3. **On manual sync:** The user can trigger "Sync XMP" from the settings or from a source's context menu. This re-reads all XMP for all assets in a source.

### Outbound (Alexandria catalog → XMP)

XMP is written in the following situations:

1. **When a catalog field changes (if `catalog_wins` mode):** After any command that modifies rating, label, or tags, Alexandria writes an updated XMP sidecar.

2. **When a catalog field changes (if `xmp_wins` mode):** XMP is NOT automatically written back. The user's XMP-native tools (Lightroom) remain authoritative. The user can still trigger a manual write-back.

3. **On manual sync:** The user can trigger "Write XMP" to push catalog values to XMP sidecars for a source.

---

## Conflict resolution

A conflict occurs when:
- `assets.xmp_hash` exists (we've read XMP before)
- The current XMP sidecar hash differs from `assets.xmp_hash` (XMP changed externally — Lightroom or another tool edited it)
- The catalog value for the changed field also differs from what the XMP says

In other words: both sides have changed since the last sync.

### Resolution modes

Configured per-user in settings as `xmp_conflict_resolution`:

**`xmp_wins` (default):**
XMP / Lightroom values always overwrite catalog values on conflict. This is the right default for users who primarily work in Lightroom and treat Alexandria as a supplementary view. Lightroom is the source of truth.

**`catalog_wins`:**
Alexandria catalog values are preserved on conflict. XMP is not applied for conflicting fields. The `xmp_hash` is updated so the conflict is not re-detected on the next sync. This mode is appropriate for users who primarily work in Alexandria and want to push their changes back to Lightroom.

### Non-conflicting changes

If XMP changes a field that the catalog hasn't changed since last sync (or if the catalog has no value for the field), the XMP value is applied regardless of the conflict resolution mode. Conflict resolution only matters when both sides have a different opinion about the same field.

### Tags / keywords: merge, not replace

Tag sync always merges, regardless of conflict mode:
- Tags present in XMP but not in catalog → added to catalog with `source: "xmp"`
- Tags present in catalog (with `source: "user"`) but not in XMP → retained in catalog, not removed
- Tags present in both → no change

This prevents a Lightroom keyword removal from silently deleting tags the user added in Alexandria, and vice versa.

---

## XMP hash tracking

Three fields on each asset track XMP sync state:

- `xmp_last_read_at`: timestamp of the last time Alexandria read XMP for this asset. Used to identify assets that have never been XMP-synced, or haven't been synced recently.
- `xmp_last_written_at`: timestamp of the last time Alexandria wrote XMP for this asset.
- `xmp_hash`: hash of the XMP content at the last sync. Used to detect external changes. On each inbound sync check, the current sidecar is hashed and compared to this value. If they differ, an external tool has edited the XMP since Alexandria last synced.

The hash is computed over the relevant XMP fields only (rating, label, keywords, description), not the entire XMP document. Full XMP documents contain timestamps and tool-specific metadata that changes on every write, which would cause false "conflicts" on every sync.

**Preventing self-triggered sync loops:** Alexandria's own sidecar writes generate file-watcher events on the `.xmp` file, which would otherwise trigger an inbound sync of our own write (and, via undo or `catalog_wins` mode, potentially a ping-pong of writes). The rule: the XMP writer computes and stores the new `xmp_hash` on the asset **synchronously, in the same operation as the sidecar write, before returning**. The inbound handler's first check is hash-vs-stored-hash; a match means "this is our own write (or nothing changed)" and the event is dropped. The debounced watcher event always arrives after the hash is already stored, so the loop cannot start.

---

## Implementation notes

### Reading XMP

The XMP reader checks for a sidecar first, falls back to embedded XMP:

```
func bestXMPSource(filePath, mimeType):
  sidecarPath = filePath + ".xmp"
  if file_exists(sidecarPath):
    return sidecarPath
  if mimeType supports embedded XMP:
    return filePath  (reader handles embedded extraction)
  return ""  (no XMP available)
```

### Writing XMP

The XMP writer always writes sidecars (never modifies embedded XMP in source files, to avoid corrupting them):

```
func WriteSidecar(sidecarPath, data XMPData):
  if file_exists(sidecarPath):
    read existing sidecar
    merge new values into existing (preserve fields we don't manage)
    write back
  else:
    create new minimal XMP sidecar with our values
```

**Merging on write is important.** A sidecar may contain develop settings or other fields that Lightroom manages. Alexandria must not overwrite the entire sidecar with only the fields it knows about — it must merge its fields into whatever is already there.

### Write-protected formats

The following MIME types must never be written to (sidecar or embedded):

- `application/x-affinity-photo`
- `application/x-affinity-designer`
- `application/x-affinity-publisher`
- `application/x-indesign`
- Any format where we only have an embedded preview (not the real format data)

If an outbound sync is triggered for an asset with one of these MIME types, it is silently skipped.

### Non-fatal errors

XMP sync errors are non-fatal:

- Source file offline: skip, log at debug (expected when drive not mounted)
- Permission denied: log at warn (user may need to check permissions on NAS share)
- Corrupt sidecar: log at warn, leave `xmp_hash` unchanged so it retries next time
- Write failed (disk full): log at error, surface to user

A failed XMP sync does not affect the catalog record. The catalog retains its current state. The sync will be retried on the next trigger.

---

## Lightroom Classic setup requirements

For XMP sync to work with Lightroom Classic, the user must enable "Automatically write changes into XMP" in Lightroom's catalog settings (Edit → Catalog Settings → Metadata → Automatically write changes into XMP).

Without this setting, Lightroom stores all metadata in its own catalog and does not write XMP sidecars. Alexandria will not see Lightroom's changes.

This requirement should be documented prominently in the Alexandria setup guide.
