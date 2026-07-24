package importer

import (
	"context"
	"log/slog"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/charmbracelet/log"
)

// MATCH is the identity matrix (docs/data-model.md §6), run on a single goroutine
// so its catalog reads see a serializable view of the state the pipeline is
// mutating. It decides new/reimport/relink/duplicate, mints the UUID for the
// verdicts that need one (before THUMB, which names the thumbnail file by ID),
// and keeps an in-run hash map so a first-import duplicate PAIR is still caught.

func (pipe *pipeline) match(ctx context.Context, in <-chan *pipelineItem, out chan<- *pipelineItem) error {
	inRunHashes := map[string]string{} // hash → assetID minted this run
	for item := range in {
		if item.isSidecar || item.rejected {
			if err := pipe.emit(ctx, out, item); err != nil {
				return err
			}
			continue
		}
		_, span := pipe.importer.Tracer.Start(item.ctx, "import.match")
		verdict, existing, err := pipe.importer.classify(ctx, pipe.target, &item.scanned, item.hash, inRunHashes, item.logger)
		if err != nil {
			span.Fail(err)
			item.rejected = true
			item.addError("match", "match_failed", err.Error())
		} else {
			span.SetAttrs(slog.String("verdict", verdict.String()))
			item.verdict, item.existing = verdict, existing
			switch verdict {
			case actionNew:
				item.assetID = domain.NewID()
				inRunHashes[item.hash] = item.assetID
			case actionDuplicate:
				item.assetID = domain.NewID() // a duplicate is a new identity
			case actionReimport:
				item.assetID = existing.ID
			}
		}
		span.End()
		if err := pipe.emit(ctx, out, item); err != nil {
			return err
		}
	}
	return nil
}

// classify decides what to do with a hashed file, in the matrix's precedence
// order (docs/data-model.md §6). The returned asset is the existing/matched record
// the action refers to (nil for a brand-new file). inRunHashes maps this run's
// freshly-minted hashes → asset IDs so a first-import duplicate PAIR (a copy that
// exists before its original is committed) is still caught; pass nil for the
// single-file path.
//
// Precedence (order matters — this IS the policy). Per D20 the matrix never
// auto-changes an asset's IDENTITY: it acts on a known PATH and otherwise only
// DETECTS-and-flags. There is no relink/move verdict — a file that reappeared at a
// new path is a new asset plus a pending review row.
//  1. Reimport: path already indexed → refresh observations (and restore online if
//     it was missing and reappeared at its ORIGINAL path). Path identity wins for a
//     known address.
//  2. Duplicate: content matches another asset anywhere (present OR missing, any
//     source) → mint a NEW identity + a pending duplicates row. This is a detection
//     FLAG only — never a mutation of the matched asset. A match against a *missing*
//     asset is a probable move; against a *present* one, a plain duplicate; the
//     review queue derives which from live status (DEFERRED §5).
//  3. New: no match → mint.
func (imp *Importer) classify(ctx context.Context, target Target, scanned *scannedFile, hash string, inRunHashes map[string]string, logger *log.Logger) (action, *domain.Asset, error) {
	// (1) Reimport — something already indexed at this exact path (an in-place edit,
	// or a missing file reappearing at its ORIGINAL path → reimport restores online).
	atPath, err := imp.Reader.FindByVolumePath(ctx, target.VolumeID, scanned.relPath)
	if err != nil {
		return actionNew, nil, err
	}
	if atPath != nil {
		logger.Debug("reimporting existing asset", "path", scanned.relPath, "assetID", atPath.ID)
		return actionReimport, atPath, nil
	}

	// (2) Duplicate — content matches another asset (present or missing, any source).
	// A detection flag only: mint a new identity, log the pair for user review.
	contentMatch, err := imp.Reader.FindByHash(ctx, hash, scanned.size)
	if err != nil {
		return actionNew, nil, err
	}
	if contentMatch != nil {
		logger.Debug("content match — flagging duplicate/probable-move for review", "path", scanned.relPath, "assetID", contentMatch.ID)
		return actionDuplicate, contentMatch, nil
	}
	if inRunAssetID, ok := inRunHashes[hash]; ok {
		logger.Debug("in-run content match — flagging duplicate for review", "path", scanned.relPath, "assetID", inRunAssetID)
		return actionDuplicate, &domain.Asset{ID: inRunAssetID}, nil
	}

	// (3) New.
	logger.Debug("new asset detected", "path", scanned.relPath)
	return actionNew, nil, nil
}
