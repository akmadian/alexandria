# Ingest Pipeline

## Overview

The ingest pipeline is the core of Alexandria's import system. It takes a source (a folder, drive, or network share) and produces catalog records, thumbnails, and metadata for every asset it finds.

The pipeline is built as a series of stages connected by buffered Go channels. Each stage is a pool of goroutines consuming from an input channel and producing to an output channel. Stages are decoupled — a slow thumbnailer does not block the hasher. The whole pipeline is cancellable, idempotent, and fault-tolerant (individual file failures do not abort the pipeline).

---

## Pipeline stages

```
Scanner → Hasher → Dedup Checker → Metadata Extractor → Thumbnailer → Catalog Writer
```

Each arrow is a buffered channel. The buffer size is tuned to decouple stage speeds without holding excessive data in memory.

### Stage 1: Scanner

**Goroutines:** 1 (single goroutine walking the directory tree)

**Input:** ImportJob (source ID, batch size, options)

**Output:** `ScannedFile` channel

**What it does:**
1. Walks the source directory recursively (or non-recursively, per source config)
2. For each file, reads filename, extension, size, mtime
3. Infers MIME type from extension (fast path) or file header sniffing (fallback)
4. Checks the catalog for an existing location record matching this source + relative path
5. If a location record exists AND mtime + size match the stored values → skip (emit nothing, increment skipped count)
6. If a location record exists BUT mtime or size differ → emit with `Reimport: true` flag
7. If no location record → emit as new file
8. Hidden files and system files are skipped
9. Files with unsupported extensions are skipped (logged at debug level)

**The skip check is the idempotency gate.** Re-running the import on the same unchanged source is essentially free — the scanner skips everything it already knows about.

```
ScannedFile
  SourceID      string
  AbsPath       string
  RelativePath  string          -- relative to source.BasePath
  Filename      string
  Extension     string
  MIMEType      string
  SizeBytes     int64
  MTime         time.Time
  Reimport      bool            -- true if updating an existing asset
```

### Stage 2: Hasher

**Goroutines:** N (configurable, default 4, I/O bound)

**Input:** `ScannedFile` channel

**Output:** `HashedFile` channel

**What it does:**
1. Reads the first 64KB of the file
2. Computes xxHash of those bytes concatenated with the file size as a string
3. If the file is smaller than 64KB, hashes the entire file
4. Emits HashedFile with the computed hash

**Why xxHash:** It is approximately 10–20x faster than MD5 and does not need cryptographic properties. The goal is a fast change detector and dedup fingerprint, not collision resistance.

**Why first 64KB + size:** Reading the entire file at ingest time is prohibitively slow for large files (a 2GB video) over a NAS connection. First 64KB is sufficient to distinguish files in a creative library. The size is appended to prevent false matches between a small file and a prefix of a larger file with identical content.

```
HashedFile
  ScannedFile           -- embeds all ScannedFile fields
  PartialHash  string
```

### Stage 3: Dedup Checker

**Goroutines:** 1 (single goroutine — needs serialised DB access to avoid race conditions on detection)

**Input:** `HashedFile` channel

**Output:** `HashedFile` channel (pass-through) + `DuplicateFile` channel (for review queue)

**What it does:**
1. Queries the catalog for an existing asset with the same partial hash AND same size
2. **No match:** pass through to extraction
3. **Match, same source + path:** this is a reimport of the same file (shouldn't reach here if scanner skip worked, but defensive check). Skip.
4. **Match, different path:** duplicate detected. Route based on user setting:
   - `auto_drop`: log at info level, emit nothing (file is skipped)
   - `review`: add to duplicate review queue, emit nothing to main pipeline for now

**On the duplicate review queue:** When `duplicate_handling = "review"`, duplicates are held in a transient in-memory queue (not persisted). At the end of import, the summary shows "N duplicates need review" with a link to the review UI where the user can decide: keep both, keep one, or ignore. This is a UX detail — the key point is that duplicates do not flow into the catalog automatically.

**What counts as a duplicate:** Same `partial_hash` AND same `size_bytes`. This is a probabilistic match — not guaranteed to catch every duplicate (different files could theoretically share a 64KB prefix and size), but reliable enough for a creative asset library. False positives are possible but extremely unlikely.

### Stage 4: Metadata Extractor

**Goroutines:** N (configurable, default 2, CPU bound — be conservative here)

**Input:** `HashedFile` channel

**Output:** `ExtractedFile` channel

**What it does:**
1. Routes to the appropriate `MetadataExtractor` implementation based on MIME type
2. Extracts all available metadata: EXIF, IPTC, XMP, dimensions, duration, camera data, GPS, etc.
3. Populates an `AssetMetadata` struct
4. On extraction error: logs the error, emits the file with partial metadata (filename, size, type), continues

Extraction failure is not fatal. A corrupt EXIF block should not prevent the file from being indexed.

```
ExtractedFile
  HashedFile               -- embeds all HashedFile fields
  Asset    *domain.Asset   -- populated with extracted metadata
```

### Stage 5: Thumbnailer

**Goroutines:** N (configurable, default 2, CPU bound)

**Input:** `ExtractedFile` channel

**Output:** `ThumbedFile` channel

**What it does:**
1. Determines the output path: `{app_data_dir}/thumbnails/{uuid_prefix}/{asset_uuid}.jpg`
2. Routes to the appropriate `Thumbnailer` implementation based on MIME type
3. Generates thumbnail at a consistent max dimension (e.g. 512×512 or 1024×1024, configurable)
4. Writes thumbnail file to app data directory
5. Sets `ThumbnailPath` and `ThumbnailAt` on the asset
6. On thumbnail error: logs the error, sets a placeholder thumbnail path, continues

**Thumbnail storage:** `thumbnails/{ab}/{ab1234cd-...}.jpg` — a two-character prefix subdirectory derived from the UUID avoids filesystem limits on files per directory at large library sizes.

**For reimports:** If a thumbnail already exists for this asset and the file's hash has not changed, skip regeneration. If the hash changed (file was edited), regenerate.

```
ThumbedFile
  ExtractedFile            -- embeds all ExtractedFile fields
  ThumbnailPath  string
```

### Stage 6: Catalog Writer

**Goroutines:** 1 (single goroutine — one write path to SQLite)

**Input:** `ThumbedFile` channel

**Output:** none (terminal stage)

**What it does:**
1. Accumulates incoming files into a batch (default batch size: 50)
2. When batch is full OR input channel is closed, writes the batch in a single SQLite transaction
3. For new assets: INSERT into `assets`, INSERT into `locations`
4. For reimports: UPDATE `assets` (metadata, hash, mtime), UPDATE `locations`
5. Updates `assets_fts` full-text search index
6. After each batch commit: emits a progress event to the frontend via Wails events
7. Collects any errors for the final summary

**Why batched writes:** Each SQLite transaction involves a disk fsync. Writing 50 records in one transaction is vastly faster than 50 individual transactions. On a large import (2,000 files), this is the difference between seconds and minutes.

**Why single writer:** SQLite's WAL mode allows concurrent reads alongside one writer. Using a single write goroutine keeps the write path simple and avoids lock contention. The catalog writer is the only place in the application that INSERTs or UPDATEs.

---

## Orchestration

The `Importer` struct owns the pipeline. It creates the channels, starts the goroutine pools, and coordinates shutdown.

```
ImportJob
  SourceID     string
  BatchSize    int             -- catalog write batch size
  Priority     ImportPriority  -- Normal or Low (for background catch-up scans)
  OnProgress   func(ImportProgress)

ImportProgress
  Total        int
  Processed    int
  Errors       int
  Stage        string          -- "scanning", "hashing", "extracting", "thumbnailing", "writing", "done"

ImportResult
  Added        int
  Updated      int
  Skipped      int
  Errors       []ImportError

ImportError
  Path   string
  Stage  string
  Err    error
```

**Cancellation:** The `ImportJob` receives a `context.Context`. Every worker's inner loop checks `ctx.Done()` before processing each file. Cancellation propagates through the pipeline by closing channels in sequence after the context is cancelled.

**Graceful shutdown:** When the scanner goroutine finishes (or the context is cancelled), it closes the scanned channel. The hasher goroutines drain the scanned channel, then close the hashed channel. And so on through each stage. The catalog writer sees its input channel close, flushes any remaining partial batch, and signals completion. This ensures no files are dropped mid-pipeline on cancellation — the pipeline drains cleanly.

**Priority:** Low-priority imports (background catch-up scans) use a smaller worker pool (e.g. 1 hash worker, 1 thumb worker) and include deliberate small sleeps between batches to yield I/O to other processes. Normal-priority imports (user-triggered) use the full configured worker pool.

---

## Single-file ingest (watcher path)

When the file watcher detects a change to a single file, it calls `Importer.IngestFile()` which enters the pipeline at the hasher stage, bypassing the scanner. The same stages run; only the entry point differs.

```
Importer.IngestFile(ctx, source *Source, absPath string)
  → infers MIMEType from extension/header
  → creates a ScannedFile
  → feeds it directly into the hasher
  → pipeline continues normally
  → catalog writer commits a batch of 1
```

This means the watcher and the manual importer share identical pipeline logic. There is no separate "watch update" code path.

---

## Idempotency

The pipeline is safe to re-run on the same source at any time:

1. **Scanner skip:** Files with unchanged mtime + size are skipped before entering the pipeline. No hashing, no extraction, no thumbnailing, no DB write.
2. **Reimport flag:** Files with changed mtime or size get `Reimport: true`, which causes the catalog writer to UPDATE rather than INSERT.
3. **Dedup checker:** Prevents the same file content from being indexed twice even if it appears at two different paths.
4. **Thumbnail skip:** Thumbnails are not regenerated if the asset hash hasn't changed.

Re-running import on a 10,000-file source where nothing has changed should take a few seconds (just the scanner walk + mtime/size checks), not minutes.

---

## Error handling in the pipeline

Errors in the pipeline do not abort processing. Each worker:

1. Catches errors per file
2. Logs them at the appropriate level (warn for expected failures, error for unexpected)
3. Sends them to a shared error channel
4. Continues to the next file

The orchestrator drains the error channel into `ImportResult.Errors`. At the end of import, the UI shows the error count with a "view errors" option that lists file paths and failure reasons.

**Expected failures (warn level):**
- EXIF extraction failed (corrupt EXIF block)
- Thumbnail generation failed (unsupported format variant, corrupt file)
- File disappeared between scan and hash (race condition, fine)

**Unexpected failures (error level):**
- Catalog write failed (disk full, SQLite error)
- Cannot open file (permissions issue)

Catalog write failures are the most serious — if the writer can't write, the import is failing silently. These should surface prominently, not just in the error list.
