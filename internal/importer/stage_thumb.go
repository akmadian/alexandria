package importer

import (
	"context"
	"io/fs"
	"time"
)

// THUMB generates the thumbnail file (moves and sidecars skip it; a type with no
// generator is a no-op). It runs BEFORE write by design: an asset commits only
// once fully processed — no placeholder cards, ever. A failure records a DLQ row
// and leaves thumbnailedAt nil; the asset still commits.

func (pipe *pipeline) thumb(ctx context.Context, in <-chan *pipelineItem, out chan<- *pipelineItem) error {
	for item := range in {
		if !item.isSidecar && !item.rejected && item.verdict != actionMove && pipe.importer.Thumbnail != nil {
			pipe.thumbnailOne(item)
		}
		if err := pipe.emit(ctx, out, item); err != nil {
			return err
		}
	}
	return nil
}

func (pipe *pipeline) thumbnailOne(item *pipelineItem) {
	reader, closeReader, err := openSeeker(pipe.fsys, item.scanned.relPath)
	if err != nil {
		item.addError("thumb", "read_failed", err.Error())
		return
	}
	defer closeReader()
	generated, err := pipe.importer.Thumbnail.Generate(item.scanned.handler.Thumb, reader, item.assetID)
	if err != nil {
		item.addError("thumb", "decode_failed", err.Error())
		return
	}
	if generated {
		now := time.Now().UTC()
		item.thumbnailedAt = &now
	}
}

// thumbnail generates the thumbnail for a freshly written asset and records
// thumbnail_at, best-effort — the single-file (watcher) path. Unlike the THUMB
// stage, it writes thumbnail_at directly (there is no batching txn to fold it
// into). Skipped for moves and when the type has no generator.
func (imp *Importer) thumbnail(ctx context.Context, fsys fs.FS, scanned scannedFile, assetID string, verdict action) {
	if imp.Thumbnail == nil || verdict == actionMove {
		return
	}
	reader, closeReader, err := openSeeker(fsys, scanned.relPath)
	if err != nil {
		imp.Log.Warn("thumbnail: open failed", "path", scanned.relPath, "err", err)
		return
	}
	defer closeReader()

	generated, err := imp.Thumbnail.Generate(scanned.handler.Thumb, reader, assetID)
	if err != nil {
		imp.Log.Warn("thumbnail generation failed", "path", scanned.relPath, "err", err)
		return
	}
	if !generated {
		return // no generator for this type; nothing written, nothing to record
	}

	now := time.Now().UTC()
	if err := imp.Derived.SetThumbnailAt(ctx, assetID, now); err != nil {
		imp.Log.Warn("thumbnail: recording failed", "path", scanned.relPath, "err", err)
		return
	}
	imp.Log.Debug("thumbnailed", "path", scanned.relPath, "asset", assetID)
}
