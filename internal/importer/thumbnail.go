package importer

import (
	"context"
	"io/fs"
	"time"
)

// thumbnail generates thumbnails for a freshly written asset and records
// thumbnail_at, best-effort: any failure logs a warning and leaves the flag
// nil (the asset still indexes; thumbnails regenerate later). Skipped for moves
// (content unchanged, existing thumbnail still applies) and when the type has no
// generator — Generate reports ok=false, so we never flag a phantom thumbnail.
func (imp *Importer) thumbnail(ctx context.Context, fsys fs.FS, sf scannedFile, assetID string, act action) {
	if imp.Thumbnail == nil || act == actionMove {
		return
	}
	rs, closeFn, err := openSeeker(fsys, sf.relPath)
	if err != nil {
		imp.Log.Warn("thumbnail: open failed", "path", sf.relPath, "err", err)
		return
	}
	defer closeFn()

	ok, err := imp.Thumbnail.Generate(rs, sf.mime, assetID)
	if err != nil {
		imp.Log.Warn("thumbnail generation failed", "path", sf.relPath, "err", err)
		return
	}
	if !ok {
		return // no generator for this type; nothing written, nothing to record
	}

	now := time.Now().UTC()
	if err := imp.Derived.SetThumbnailAt(ctx, assetID, now); err != nil {
		imp.Log.Warn("thumbnail: recording failed", "path", sf.relPath, "err", err)
		return
	}
	imp.Log.Debug("thumbnailed", "path", sf.relPath, "asset", assetID)
}
