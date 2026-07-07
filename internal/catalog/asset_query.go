package catalog

import (
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// FilePatch is the observation-only update applied on reimport: file facts
// (always written — the file changed) plus extracted metadata (overlay — a
// non-nil field overwrites, a nil field preserves the prior value, so a failed
// re-extraction never wipes good data). It deliberately carries NO judgment,
// sync, or derived columns: an observation writer physically cannot reach them.
type FilePatch struct {
	// File facts — always written.
	Filename    string
	Extension   string
	MIMEType    string
	FileType    domain.FileType
	SizeBytes   int64
	MTime       time.Time
	PartialHash string
	FileStatus  domain.FileStatus

	// Extracted metadata — overlay (nil = leave as-is).
	Width         *int
	Height        *int
	DurationSecs  *float64
	CapturedAt    *time.Time
	CameraMake    *string
	CameraModel   *string
	LensModel     *string
	FocalLengthMM *float64
	Aperture      *float64
	ShutterSpeed  *string
	ISO           *int
	GPSLat        *float64
	GPSLon        *float64
	ColorSpace    *string
	BitDepth      *int
	Creator       *string
	Copyright     *string
	Title         *string
	Caption       *string
	// Extended, when non-nil, replaces extended_metadata wholesale — the caller
	// is responsible for merging with the prior value if it wants to preserve keys.
	Extended map[string]any
}

// TriagePatch is the sparse judgment update: only fields with Set=true are
// written (Set + nil Value clears the column). Used by the judgment writer
// (bumps judgment_modified_at) and, with the same shape, by the XMP sync writer
// (which applies the values but must NOT bump judgment_modified_at).
type TriagePatch struct {
	Rating     domain.Opt[int]
	ColorLabel domain.Opt[domain.ColorLabel]
	Flag       domain.Opt[domain.Flag]
	Note       domain.Opt[string]
}

// PathStatus is the slim reconciliation projection: enough to check a file's
// existence and flip its status, without loading 40+ columns per row.
type PathStatus struct {
	ID           string
	RelativePath string
	FileStatus   domain.FileStatus
}

// AssetFilter is the query specification for AssetReader.List. SortField is a
// logical name (captured/added/rating/filename/size), not a column — the repo
// maps it through a whitelist, so an unknown value is rejected, never interpolated.
type AssetFilter struct {
	FileTypes      []domain.FileType
	Rating         *int
	RatingMin      *int
	ColorLabels    []domain.ColorLabel
	Flags          []domain.Flag
	TagIDs         []string
	SourceIDs      []string
	CapturedAfter  *time.Time
	CapturedBefore *time.Time
	SearchText     string
	IncludeDeleted bool
	SortField      string
	SortDir        string
	Limit          int
	Offset         int
}
