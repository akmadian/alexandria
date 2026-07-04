package importer

import (
	"context"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/metadata"
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

func (a action) String() string {
	switch a {
	case actionNew:
		return "new"
	case actionReimport:
		return "reimport"
	case actionMove:
		return "move"
	case actionDuplicate:
		return "duplicate"
	default:
		return "unknown"
	}
}

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
func (imp *Importer) persist(ctx context.Context, source *domain.Source, sf scannedFile, hash string, md metadata.Metadata, act action, existing *domain.Asset) (string, error) {
	switch act {
	case actionMove:
		if err := imp.Assets.UpdatePath(ctx, existing.ID, source.ID, sf.relPath); err != nil {
			return "", err
		}
		return existing.ID, imp.Assets.UpdateFileStatus(ctx, existing.ID, domain.FileStatusOnline)

	case actionReimport:
		applyFileFields(existing, sf, hash)
		applyMetadata(existing, md)
		return existing.ID, imp.Assets.Update(ctx, existing)

	default: // actionNew, actionDuplicate
		asset := buildAsset(source, sf, hash, md)
		if err := imp.Assets.Create(ctx, asset); err != nil {
			return "", err
		}
		if act == actionDuplicate {
			return asset.ID, imp.Dups.Log(ctx, &domain.Duplicate{
				ID:               uuid.NewString(),
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

// buildAsset creates a new asset from scan + hash, then overlays extracted
// metadata. ThumbnailAt is left nil here — the thumbnail stage patches it after
// the asset is written (see thumbnail.go).
func buildAsset(source *domain.Source, sf scannedFile, hash string, md metadata.Metadata) *domain.Asset {
	now := time.Now().UTC()
	a := &domain.Asset{
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
	applyMetadata(a, md)
	return a
}

// applyMetadata overlays extracted metadata onto an asset. Only non-nil fields
// are written, so a reimport with empty extraction never clears existing values.
func applyMetadata(a *domain.Asset, md metadata.Metadata) {
	if md.Width != nil {
		a.Width = md.Width
	}
	if md.Height != nil {
		a.Height = md.Height
	}
	if md.DurationSecs != nil {
		a.DurationSecs = md.DurationSecs
	}
	if md.CapturedAt != nil {
		a.CapturedAt = md.CapturedAt
	}
	if md.CameraMake != nil {
		a.CameraMake = md.CameraMake
	}
	if md.CameraModel != nil {
		a.CameraModel = md.CameraModel
	}
	if md.LensModel != nil {
		a.LensModel = md.LensModel
	}
	if md.FocalLengthMM != nil {
		a.FocalLengthMM = md.FocalLengthMM
	}
	if md.Aperture != nil {
		a.Aperture = md.Aperture
	}
	if md.ShutterSpeed != nil {
		a.ShutterSpeed = md.ShutterSpeed
	}
	if md.ISO != nil {
		a.ISO = md.ISO
	}
	if md.GPSLat != nil {
		a.GPSLat = md.GPSLat
	}
	if md.GPSLon != nil {
		a.GPSLon = md.GPSLon
	}
	if md.ColorSpace != nil {
		a.ColorSpace = md.ColorSpace
	}
	if md.BitDepth != nil {
		a.BitDepth = md.BitDepth
	}
	if md.Creator != nil {
		a.Creator = md.Creator
	}
	if md.Copyright != nil {
		a.Copyright = md.Copyright
	}
	if len(md.Extended) > 0 {
		if a.ExtendedMetadata == nil {
			a.ExtendedMetadata = make(map[string]any, len(md.Extended))
		}
		for k, v := range md.Extended {
			a.ExtendedMetadata[k] = v
		}
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
