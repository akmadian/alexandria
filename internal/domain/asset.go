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
	SourceID       string
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

	ExtendedMetadata map[string]any

	Rating     *int
	ColorLabel *ColorLabel
	Flag       *Flag
	Note       *string

	XMPLastReadAt    *time.Time
	XMPLastWrittenAt *time.Time
	XMPHash          *string

	ThumbnailPath *string
	ThumbnailAt   *time.Time

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
