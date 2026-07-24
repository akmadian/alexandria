// The schema compiler (C15) projects the enums and shared models declared here
// into the generated TS and the data dictionary. Regenerate after any change:
//go:generate go run github.com/akmadian/alexandria/cmd/generate -out ../../frontend/src/_generated-types -docs ../../docs

package domain

import "time"

type FileType string

const (
	FileTypeImage    FileType = "image"
	FileTypeVideo    FileType = "video"
	FileTypeRaw      FileType = "raw"
	FileTypeVector   FileType = "vector"
	FileTypeDocument FileType = "document"
	FileTypeAudio    FileType = "audio"
)

type ColorLabel string

const (
	ColorLabelRed    ColorLabel = "red"
	ColorLabelOrange ColorLabel = "orange"
	ColorLabelYellow ColorLabel = "yellow"
	ColorLabelGreen  ColorLabel = "green"
	ColorLabelBlue   ColorLabel = "blue"
	ColorLabelPurple ColorLabel = "purple"
)

type Flag string

const (
	FlagPick   Flag = "pick"
	FlagReject Flag = "reject"
)

type FileStatus string

const (
	FileStatusOnline  FileStatus = "online"
	FileStatusOffline FileStatus = "offline"
	FileStatusMissing FileStatus = "missing"
)

type Asset struct {
	ID             string
	VolumeID       string
	RelativePath   string
	FileStatus     FileStatus
	LastVerifiedAt *time.Time

	Filename    string
	Extension   string
	MIMEType    string
	FileType    FileType
	SizeBytes   int64
	MTime       time.Time
	PartialHash string

	Width        *int
	Height       *int
	DurationSecs *float64

	ColorSpace *string
	BitDepth   *int

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

	Creator   *string // EXIF Artist / IPTC By-line / XMP dc:creator
	Copyright *string // EXIF Copyright / IPTC CopyrightNotice / XMP dc:rights
	Title     *string // IPTC/XMP dc:title (observation; FTS target)
	Caption   *string // IPTC/XMP dc:description (observation; distinct from the user's Note)

	ExtendedMetadata map[string]any

	Rating     *int
	ColorLabel *ColorLabel
	Flag       *Flag
	Note       *string

	// JudgmentModifiedAt is bumped ONLY when a judgment field (rating/label/flag/
	// note/deletion) changes — never by observation refreshes. XMP conflict
	// resolution reads it to answer "did the user edit since the last sync?"
	// Enforced by the writer-split repositories (impl/02).
	JudgmentModifiedAt *time.Time

	XMPLastReadAt    *time.Time
	XMPLastWrittenAt *time.Time
	XMPHash          *string

	// ThumbnailAt records when thumbnails were generated (and doubles as the
	// "has a thumbnail?" flag). The file path is derived from the asset ID, not
	// stored — see internal/thumbnailer.Thumbnailer.Path.
	ThumbnailAt *time.Time

	// Cheap culling signals (task 20), derived post-commit by the enrichment
	// engine; nil = not yet computed. Sharpness is the raw variance of Laplacian
	// (ranking is the contract, not the absolute value); clipping is the % of
	// blown/crushed pixels. phash has no struct field — it is stored but has no
	// read/query surface yet (DEFERRED §12).
	Sharpness          *float64
	ClippingHighlights *float64
	ClippingShadows    *float64

	IsDeleted  bool
	DeletedAt  *time.Time
	IngestedAt time.Time
	UpdatedAt  time.Time
}

// FileStat is used by ListKnownFiles for the scanner skip map.
type FileStat struct {
	MTime       time.Time
	SizeBytes   int64
	PartialHash string
}
