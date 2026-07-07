package importer

import (
	"context"

	"github.com/akmadian/alexandria/internal/domain"
)

// MATCH is the identity matrix (03-data-model.md §6), run on a single goroutine
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
		verdict, existing, err := pipe.importer.classify(ctx, pipe.source, item.scanned, item.hash, inRunHashes)
		if err != nil {
			item.rejected = true
			item.addError("match", "match_failed", err.Error())
		} else {
			item.verdict, item.existing = verdict, existing
			switch verdict {
			case actionNew:
				item.assetID = domain.NewID()
				inRunHashes[item.hash] = item.assetID
			case actionDuplicate:
				item.assetID = domain.NewID() // a duplicate is a new identity
			case actionReimport, actionMove:
				item.assetID = existing.ID
			}
		}
		if err := pipe.emit(ctx, out, item); err != nil {
			return err
		}
	}
	return nil
}

// classify decides what to do with a hashed file, in the matrix's precedence
// order (03-data-model.md §6). The returned asset is the existing/matched record
// the action refers to (nil for a brand-new file). inRunHashes maps this run's
// freshly-minted hashes → asset IDs so a first-import duplicate PAIR (a copy that
// exists before its original is committed) is still caught; pass nil for the
// single-file path.
//
// Precedence (order matters — this IS the policy):
//  1. Relink: content+name match vs a MISSING asset → adopt the new path. This
//     OUTRANKS a path-based reimport: an in-place edit changes content, so its
//     hash cannot match a missing asset; a hash that DOES match one means a lost
//     file reappeared at an occupied address (delete-and-copy), not an edit. The
//     name must ALSO match: a same-content match to a missing asset with a
//     DIFFERENT name is only a PROBABLE move (a rename), not a certain one — we do
//     not auto-relink it. It falls through to (3) duplicate, which records the pair
//     for user review; a missing "original" is how the review queue tells a move
//     apart from a plain duplicate. (partial_hash is a 64KB+size fingerprint, not a
//     full hash — the name guard is the cheap confidence backstop against a false
//     merge of two differently-named files.)
//  2. Reimport: path match, content differs → refresh observations only.
//  3. Duplicate: content matches another asset (present, or a missing one under a
//     different name → a probable move) → mint a new identity + a duplicates row.
//  4. New: no match → mint.
func (imp *Importer) classify(ctx context.Context, source *domain.Source, scanned scannedFile, hash string, inRunHashes map[string]string) (action, *domain.Asset, error) {
	contentMatch, err := imp.Reader.FindByHash(ctx, hash, scanned.size)
	if err != nil {
		return actionNew, nil, err
	}
	// (1) Relink — checked before the path, per the precedence above. Requires the
	// name to match too; a name change is only a probable move (handled at (3)).
	if contentMatch != nil && contentMatch.FileStatus == domain.FileStatusMissing &&
		contentMatch.Filename == scanned.filename {
		imp.Log.Debug("relinking missing asset", "path", scanned.relPath, "assetID", contentMatch.ID)
		return actionMove, contentMatch, nil
	}

	// (2) Reimport — something already indexed at this exact path.
	atPath, err := imp.Reader.FindBySourcePath(ctx, source.ID, scanned.relPath)
	if err != nil {
		return actionNew, nil, err
	}
	if atPath != nil {
		imp.Log.Debug("reimporting existing asset", "path", scanned.relPath, "assetID", atPath.ID)
		return actionReimport, atPath, nil
	}

	// (3) Duplicate — content matches a present asset (catalog or this run).
	if contentMatch != nil {
		imp.Log.Debug("duplicate detected", "path", scanned.relPath, "assetID", contentMatch.ID)
		return actionDuplicate, contentMatch, nil
	}
	if inRunAssetID, ok := inRunHashes[hash]; ok {
		// The original was minted this run and isn't committed yet, so FindByHash
		// can't see it; the in-run map can. Only the ID is needed to log the pair.
		imp.Log.Debug("duplicate detected", "path", scanned.relPath, "assetID", inRunAssetID)
		return actionDuplicate, &domain.Asset{ID: inRunAssetID}, nil
	}

	// (4) New.
	imp.Log.Debug("new asset detected", "path", scanned.relPath)
	return actionNew, nil, nil
}
