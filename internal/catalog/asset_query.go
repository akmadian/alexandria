package catalog

import (
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// AssetRow is the slim grid-card projection (~17 fields). Full *domain.Asset
// stays Get-only (seam/01). The json tags ARE the wire contract: the schema
// generator (cmd/generate) reflects this struct into the generated TS model,
// and the seam adapter layers presentation fields (thumbURL, the kind
// discriminator) on top — those are adapter concerns, not engine truth.
type AssetRow struct {
	ID           string             `json:"id"`
	VolumeID     string             `json:"volumeId"`
	Filename     string             `json:"filename"`
	FileType     domain.FileType    `json:"fileType"`
	FileStatus   domain.FileStatus  `json:"fileStatus"`
	Rating       *int               `json:"rating"`
	ColorLabel   *domain.ColorLabel `json:"colorLabel"`
	Flag         *domain.Flag       `json:"flag"`
	Width        *int               `json:"width"`
	Height       *int               `json:"height"`
	DurationSecs *float64           `json:"durationSecs"`
	CameraModel  *string            `json:"cameraModel"`
	CapturedAt   *time.Time         `json:"capturedAt"`
	IngestedAt   time.Time          `json:"ingestedAt"`
	ThumbnailAt  *time.Time         `json:"thumbnailAt"`
	RelativePath string             `json:"relativePath"`
	SizeBytes    int64              `json:"sizeBytes"`

	// Enriching / Failed are transient enrichment decoration (task 21), filled by
	// the seam from the engine — NOT the catalog query (both are zero off the DB).
	// Enriching lists the kinds in flight; Failed lists the terminally-failed
	// (attempt-exhausted) kinds. The frontend derives per-artifact state: data
	// present = ready, in Enriching = enriching, in Failed = failed, neither =
	// pending (D25). omitempty so an idle row carries neither.
	Enriching []domain.EnrichmentKind `json:"enriching,omitempty"`
	Failed    []domain.EnrichmentKind `json:"failed,omitempty"`
}

// TriageState is the prior-state projection undo captures: before-images for
// value writes.
type TriageState struct {
	ID         string
	Rating     *int
	ColorLabel *domain.ColorLabel
	Flag       *domain.Flag
	Note       *string
}

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
