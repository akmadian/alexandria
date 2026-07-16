package importer

import (
	"context"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/metadata"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/gospan"
	"github.com/charmbracelet/log"
)

// WRITE is the single-writer stage: SQLite takes one writer, so this goroutine IS
// the batching point. It accumulates up to pipe.batchSize items (or flushes on a
// lull, or at stream end) and commits each batch in one transaction via
// Store.InTx. Cancellation commits the current batch and exits — completed work
// is never rolled back.

func (pipe *pipeline) write(ctx context.Context, incoming <-chan *pipelineItem) error {
	batch := make([]*pipelineItem, 0, pipe.batchSize)
	timer := time.NewTimer(writeLull)
	timer.Stop()
	defer timer.Stop()

	flush := func() error {
		err := pipe.commit(ctx, batch)
		batch = batch[:0]
		return err
	}
	for {
		select {
		case <-ctx.Done():
			// Commit the current batch then exit — never roll back completed work.
			// WithoutCancel so the commit itself isn't aborted by the same cancel.
			_ = pipe.commit(context.WithoutCancel(ctx), batch)
			return ctx.Err()
		case item, ok := <-incoming:
			if !ok {
				return flush()
			}
			// The write-wait span: arrival → commit. Its duration is the batching
			// latency (fill time + lull + tx), ended in endItemTrace.
			_, item.awaitCommitSpan = pipe.importer.Tracer.Start(item.ctx, "import.await-commit")
			if len(batch) == 0 {
				timer.Reset(writeLull)
			}
			batch = append(batch, item)
			if len(batch) >= pipe.batchSize {
				timer.Stop()
				if err := flush(); err != nil {
					return err
				}
			}
		case <-timer.C:
			if err := flush(); err != nil {
				return err
			}
		}
	}
}

// commit writes one batch in a single transaction, then runs post-commit hooks
// (progress; grouping recompute is a stub until that engine lands). Tallies
// happen only after a successful commit.
func (pipe *pipeline) commit(ctx context.Context, batch []*pipelineItem) error {
	if len(batch) == 0 {
		return nil
	}
	// The batch is its own tiny trace (fan-in recipe): a commit serving N item
	// traces belongs to none of them, so the many-to-one lives in the shared
	// batch_seq attribute, never in span structure.
	pipe.batchSeq++
	_, batchSpan := pipe.importer.Tracer.Start(context.Background(), "import.write-batch",
		slog.Int("items", len(batch)), slog.Int("batch_seq", pipe.batchSeq))
	err := pipe.importer.Store.InTx(ctx, func(repos sqlite.Repos) error {
		for _, item := range batch {
			if err := pipe.writeItem(ctx, repos, item); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		batchSpan.Fail(err)
	}
	batchSpan.End()
	if err != nil {
		// The tx is poisoned by the first failing statement, so per-item isolation
		// isn't free here. Log loudly, count the batch as errored, continue — the
		// work is skip-gated / re-driven next run (idempotency is the recovery).
		// ponytail: no per-item re-commit on tx failure. Add it if a real workload
		// starts losing whole batches to one bad row.
		pipe.importer.Log.Error("batch commit failed", "items", len(batch), "err", err)
		for _, item := range batch {
			pipe.addItemError(item, "write", err)
			pipe.endItemTrace(item, err)
		}
		pipe.errorCount += len(batch)
		return nil
	}

	var committed int
	for _, item := range batch {
		pipe.tally(item)
		pipe.endItemTrace(item, nil)
		if !item.isSidecar && !item.rejected {
			committed++
			if pipe.importer.OnAssetCommitted != nil {
				pipe.importer.OnAssetCommitted(ctx, pipe.source, item.assetID, item.scanned.relPath)
			}
		}
	}
	pipe.done.Add(int64(committed))
	pipe.postCommit(ctx)
	return nil
}

// endItemTrace closes the item's await-commit span and its root span, both
// tagged with the batch that carried them. The root records the verdict (for
// per-verdict SQL) and whether the item was rejected upstream; a failed commit
// fails the root, and a rejected item's error lives on the failed STAGE span +
// the DLQ row — the root's status stays the commit outcome.
func (pipe *pipeline) endItemTrace(item *pipelineItem, commitErr error) {
	batchAttr := slog.Int("batch_seq", pipe.batchSeq)
	item.awaitCommitSpan.SetAttrs(batchAttr)
	item.awaitCommitSpan.End()

	root := gospan.FromContext(item.ctx)
	if !item.isSidecar && !item.rejected {
		root.SetAttrs(batchAttr, slog.String("verdict", item.verdict.String()))
	} else {
		root.SetAttrs(batchAttr, slog.Bool("rejected", item.rejected))
	}
	if commitErr != nil {
		root.Fail(commitErr)
	}
	root.End()
}

// writeItem applies one item's mutations via the tx-bound repos. Uses only the
// observation/derived/dup/sidecar/import repos on Repos — the "one cook" that
// owns every catalog mutation (judgment columns are never touched here).
func (pipe *pipeline) writeItem(ctx context.Context, repos sqlite.Repos, item *pipelineItem) error {
	for _, stageError := range item.stageErrors {
		if err := repos.Imports.LogError(ctx, pipe.sessionID, item.scanned.relPath, stageError.stage, stageError.reasonCode, stageError.message); err != nil {
			return err
		}
	}
	if item.isSidecar {
		return repos.Sidecars.UpsertObservation(ctx, buildSidecar(pipe.source, &item.scanned, item.hash))
	}
	if item.rejected {
		return nil // error row written; no identity minted
	}

	mergeMarker(&item.extractedMetadata, item.mismatchMarker)
	switch item.verdict {
	case actionReimport:
		if err := repos.Assets.ApplyFilePatch(ctx, item.assetID, reimportFilePatch(&item.scanned, item.hash, &item.extractedMetadata, item.existing)); err != nil {
			return err
		}
		item.logger.Debug("reimport: derived state + enrichment DLQ cleared", "path", item.scanned.relPath, "assetID", item.assetID)
		return clearStaleDerived(ctx, repos, item.assetID)
	default: // actionNew, actionDuplicate
		asset := buildAsset(item.assetID, pipe.source, &item.scanned, item.hash, &item.extractedMetadata)
		if err := repos.Assets.Create(ctx, asset); err != nil {
			return err
		}
		if item.verdict == actionDuplicate {
			return repos.Dups.Log(ctx, &domain.Duplicate{
				ID:               domain.NewID(),
				OriginalAssetID:  item.existing.ID,
				DuplicateAssetID: asset.ID,
				PartialHash:      item.hash,
				DetectedAt:       time.Now().UTC(),
				Status:           "pending",
			})
		}
		return nil
	}
}

// clearStaleDerived is the D28 staleness transition, run inside the reimport's
// own transaction: new bytes mean every derived artifact describes the OLD
// bytes, so derived columns flip to NULL (instantly "missing" — the next
// enrichment scan re-derives them) and the asset's enrichment DLQ rows are
// deleted (exhaustion described the old bytes; new bytes get fresh attempts).
// Thumbnail FILES deliberately survive on disk: the grid shows the
// outdated-but-real thumb until regeneration overwrites it, and the
// content-addressed URL cache-busts on completion.
func clearStaleDerived(ctx context.Context, repos sqlite.Repos, assetID string) error {
	if err := repos.Assets.ClearDerived(ctx, assetID); err != nil {
		return err
	}
	return repos.Enrichment.ClearFailures(ctx, assetID)
}

// persist applies the decided verdict on the single-file (watcher) path. New and
// duplicate mint a fresh asset; reimport refreshes observation columns ONLY
// (judgments untouched — the writer split makes touching them impossible, and a
// missing file reappearing at its original path is restored online here) and
// then runs the D28 staleness clear (see clearStaleDerived) through the same
// store transaction the batch path uses. This is the unbatched sibling of
// writeItem.
func (imp *Importer) persist(ctx context.Context, source *domain.Source, scanned *scannedFile, hash string, extractedMetadata *metadata.Metadata, verdict action, existing *domain.Asset, logger *log.Logger) (string, error) {
	switch verdict {
	case actionReimport:
		logger.Debug("write.persist: reimporting existing asset", "path", scanned.relPath, "assetID", existing.ID)
		return existing.ID, imp.Store.InTx(ctx, func(repos sqlite.Repos) error {
			if err := repos.Assets.ApplyFilePatch(ctx, existing.ID, reimportFilePatch(scanned, hash, extractedMetadata, existing)); err != nil {
				return err
			}
			return clearStaleDerived(ctx, repos, existing.ID)
		})

	default: // actionNew, actionDuplicate
		logger.Debug("write.persist: new asset detected - minting!", "path", scanned.relPath)
		asset := buildAsset(domain.NewID(), source, scanned, hash, extractedMetadata)
		if err := imp.Obs.Create(ctx, asset); err != nil {
			return "", err
		}
		if verdict == actionDuplicate {
			logger.Debug("write.persist: duplicate detected", "path", scanned.relPath, "assetID", asset.ID)
			return asset.ID, imp.Dups.Log(ctx, &domain.Duplicate{
				ID:               domain.NewID(),
				OriginalAssetID:  existing.ID,
				DuplicateAssetID: asset.ID,
				PartialHash:      hash,
				DetectedAt:       time.Now().UTC(),
				Status:           "pending",
			})
		}
		return asset.ID, nil
	}
}

// buildSidecar derives the (dir, stem) filesystem key and observation columns
// for a companion file. The grouping engine later matches assets to sidecars on
// this key.
func buildSidecar(source *domain.Source, scanned *scannedFile, hash string) *domain.SidecarFile {
	directory := path.Dir(scanned.relPath)
	if directory == "." {
		directory = ""
	}
	stem := strings.ToLower(strings.TrimSuffix(scanned.filename, "."+scanned.ext))
	now := time.Now().UTC()
	return &domain.SidecarFile{
		ID:           domain.NewID(),
		SourceID:     source.ID,
		Dir:          directory,
		Stem:         stem,
		Ext:          scanned.ext,
		RelativePath: scanned.relPath,
		SizeBytes:    scanned.size,
		MTime:        scanned.mtime,
		PartialHash:  hash,
		FirstSeenAt:  now,
		UpdatedAt:    now,
	}
}

// reimportFilePatch maps the scanned file + extracted metadata onto an
// observation-only FilePatch. Metadata fields ride straight from extractedMetadata
// (same overlay semantics: nil preserves the prior value). extended_metadata is
// merged with the existing map here — the caller has the loaded asset, the patch
// writer does not.
func reimportFilePatch(scanned *scannedFile, hash string, extractedMetadata *metadata.Metadata, existing *domain.Asset) *catalog.FilePatch {
	patch := catalog.FilePatch{
		Filename:    scanned.filename,
		Extension:   scanned.ext,
		MIMEType:    scanned.mime,
		FileType:    scanned.fileType,
		SizeBytes:   scanned.size,
		MTime:       scanned.mtime,
		PartialHash: hash,
		FileStatus:  domain.FileStatusOnline,

		Width:         extractedMetadata.Width,
		Height:        extractedMetadata.Height,
		DurationSecs:  extractedMetadata.DurationSecs,
		CapturedAt:    extractedMetadata.CapturedAt,
		CameraMake:    extractedMetadata.CameraMake,
		CameraModel:   extractedMetadata.CameraModel,
		LensModel:     extractedMetadata.LensModel,
		FocalLengthMM: extractedMetadata.FocalLengthMM,
		Aperture:      extractedMetadata.Aperture,
		ShutterSpeed:  extractedMetadata.ShutterSpeed,
		ISO:           extractedMetadata.ISO,
		GPSLat:        extractedMetadata.GPSLat,
		GPSLon:        extractedMetadata.GPSLon,
		ColorSpace:    extractedMetadata.ColorSpace,
		BitDepth:      extractedMetadata.BitDepth,
		Creator:       extractedMetadata.Creator,
		Copyright:     extractedMetadata.Copyright,
	}
	if len(extractedMetadata.Extended) > 0 || len(existing.ExtendedMetadata) > 0 {
		merged := make(map[string]any, len(existing.ExtendedMetadata)+len(extractedMetadata.Extended))
		for key, value := range existing.ExtendedMetadata {
			merged[key] = value
		}
		for key, value := range extractedMetadata.Extended {
			merged[key] = value
		}
		patch.Extended = merged
	}
	return &patch
}

// buildAsset creates a new asset from scan + hash, then overlays extracted
// metadata. The ID is minted by the caller (MATCH). ThumbnailAt is nil at
// commit by design (D25): the thumbnail is an enrichment artifact, produced
// post-commit by the engine — the missing column IS the queue.
func buildAsset(id string, source *domain.Source, scanned *scannedFile, hash string, extractedMetadata *metadata.Metadata) *domain.Asset {
	now := time.Now().UTC()
	asset := &domain.Asset{
		ID:           id,
		SourceID:     source.ID,
		RelativePath: scanned.relPath,
		FileStatus:   domain.FileStatusOnline,
		Filename:     scanned.filename,
		Extension:    scanned.ext,
		MIMEType:     scanned.mime,
		FileType:     scanned.fileType,
		SizeBytes:    scanned.size,
		MTime:        scanned.mtime,
		PartialHash:  hash,
		IngestedAt:   now,
		UpdatedAt:    now,
	}
	applyMetadata(asset, extractedMetadata)
	return asset
}

// applyMetadata overlays extracted metadata onto an asset. Only non-nil fields
// are written, so a reimport with empty extraction never clears existing values.
func applyMetadata(asset *domain.Asset, extractedMetadata *metadata.Metadata) {
	if extractedMetadata.Width != nil {
		asset.Width = extractedMetadata.Width
	}
	if extractedMetadata.Height != nil {
		asset.Height = extractedMetadata.Height
	}
	if extractedMetadata.DurationSecs != nil {
		asset.DurationSecs = extractedMetadata.DurationSecs
	}
	if extractedMetadata.CapturedAt != nil {
		asset.CapturedAt = extractedMetadata.CapturedAt
	}
	if extractedMetadata.CameraMake != nil {
		asset.CameraMake = extractedMetadata.CameraMake
	}
	if extractedMetadata.CameraModel != nil {
		asset.CameraModel = extractedMetadata.CameraModel
	}
	if extractedMetadata.LensModel != nil {
		asset.LensModel = extractedMetadata.LensModel
	}
	if extractedMetadata.FocalLengthMM != nil {
		asset.FocalLengthMM = extractedMetadata.FocalLengthMM
	}
	if extractedMetadata.Aperture != nil {
		asset.Aperture = extractedMetadata.Aperture
	}
	if extractedMetadata.ShutterSpeed != nil {
		asset.ShutterSpeed = extractedMetadata.ShutterSpeed
	}
	if extractedMetadata.ISO != nil {
		asset.ISO = extractedMetadata.ISO
	}
	if extractedMetadata.GPSLat != nil {
		asset.GPSLat = extractedMetadata.GPSLat
	}
	if extractedMetadata.GPSLon != nil {
		asset.GPSLon = extractedMetadata.GPSLon
	}
	if extractedMetadata.ColorSpace != nil {
		asset.ColorSpace = extractedMetadata.ColorSpace
	}
	if extractedMetadata.BitDepth != nil {
		asset.BitDepth = extractedMetadata.BitDepth
	}
	if extractedMetadata.Creator != nil {
		asset.Creator = extractedMetadata.Creator
	}
	if extractedMetadata.Copyright != nil {
		asset.Copyright = extractedMetadata.Copyright
	}
	if len(extractedMetadata.Extended) > 0 {
		if asset.ExtendedMetadata == nil {
			asset.ExtendedMetadata = make(map[string]any, len(extractedMetadata.Extended))
		}
		for key, value := range extractedMetadata.Extended {
			asset.ExtendedMetadata[key] = value
		}
	}
}

// mergeMarker overlays the extension_mismatch marker onto extracted metadata so
// it persists in extended_metadata alongside real tags.
func mergeMarker(extractedMetadata *metadata.Metadata, marker map[string]any) {
	if len(marker) == 0 {
		return
	}
	if extractedMetadata.Extended == nil {
		extractedMetadata.Extended = make(map[string]any, len(marker))
	}
	for key, value := range marker {
		extractedMetadata.Extended[key] = value
	}
}
