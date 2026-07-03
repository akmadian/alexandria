package importer

import (
	"context"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/google/uuid"
)

// action is the ingest decision for one hashed file.
type action int

const (
	actionNew action = iota
	actionReimport
	actionMove
	actionDuplicate
)

// classifyContent decides what to do with a hashed file. The returned asset is
// the existing/matched record the action refers to (nil for a brand-new file).
func (imp *Importer) classifyContent(ctx context.Context, source *domain.Source, sf scannedFile, hash string) (action, *domain.Asset, error) {
	// Something already indexed at this exact path → reimport (content changed;
	// unchanged files were skipped before hashing).
	existing, err := imp.Assets.FindBySourcePath(ctx, source.ID, sf.relPath)
	if err != nil {
		return actionNew, nil, err
	}
	if existing != nil {
		return actionReimport, existing, nil
	}

	// New path: is this content already known elsewhere?
	match, err := imp.Assets.FindByHash(ctx, hash, sf.size)
	if err != nil {
		return actionNew, nil, err
	}
	if match == nil {
		return actionNew, nil, nil
	}
	// Auto-relink only when the match is missing AND the filename agrees — that
	// combination is near-certain to be the same file moved. Anything else is a
	// genuine duplicate (logged, never dropped).
	if match.FileStatus == domain.FileStatusMissing && match.Filename == sf.filename {
		return actionMove, match, nil
	}
	return actionDuplicate, match, nil
}

// persist applies the decided action. New/duplicate insert a fresh asset;
// reimport updates in place (preserving user metadata); move relinks the
// existing record. Duplicates also log the pair.
func (imp *Importer) persist(ctx context.Context, source *domain.Source, sf scannedFile, hash string, act action, existing *domain.Asset) error {
	switch act {
	case actionMove:
		if err := imp.Assets.UpdatePath(ctx, existing.ID, source.ID, sf.relPath); err != nil {
			return err
		}
		return imp.Assets.UpdateFileStatus(ctx, existing.ID, domain.FileStatusOnline)

	case actionReimport:
		applyFileFields(existing, sf, hash)
		return imp.Assets.Update(ctx, existing)

	default: // actionNew, actionDuplicate
		asset := buildAsset(source, sf, hash)
		if err := imp.Assets.Create(ctx, asset); err != nil {
			return err
		}
		if act == actionDuplicate {
			return imp.Dups.Log(ctx, &domain.Duplicate{
				ID:               uuid.NewString(),
				OriginalAssetID:  existing.ID,
				DuplicateAssetID: asset.ID,
				PartialHash:      hash,
				DetectedAt:       time.Now().UTC(),
				Status:           "pending",
			})
		}
		return nil
	}
}

// buildAsset creates a new asset from scan + hash. Metadata and thumbnail fields
// are left nil — those stages are deferred.
func buildAsset(source *domain.Source, sf scannedFile, hash string) *domain.Asset {
	now := time.Now().UTC()
	return &domain.Asset{
		ID:           uuid.NewString(),
		SourceID:     source.ID,
		RelativePath: sf.relPath,
		FileStatus:   domain.FileStatusOnline,
		Filename:     sf.filename,
		Extension:    sf.ext,
		MIMEType:     sf.mime,
		FileType:     sf.fileType,
		SizeBytes:    sf.size,
		MTime:        sf.mtime,
		PartialHash:  hash,
		IngestedAt:   now,
		UpdatedAt:    now,
	}
}

// applyFileFields updates the file-level fields on a reimport, leaving user
// metadata (rating, labels, tags, XMP) untouched.
func applyFileFields(a *domain.Asset, sf scannedFile, hash string) {
	a.Filename = sf.filename
	a.Extension = sf.ext
	a.MIMEType = sf.mime
	a.FileType = sf.fileType
	a.SizeBytes = sf.size
	a.MTime = sf.mtime
	a.PartialHash = hash
	a.FileStatus = domain.FileStatusOnline
}
