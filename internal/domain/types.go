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

type SourceKind string

const (
	SourceKindLocal         SourceKind = "local"
	SourceKindExternalDrive SourceKind = "external_drive"
	SourceKindSMB           SourceKind = "smb"
	SourceKindNFS           SourceKind = "nfs"
)

type SourceStatus string

const (
	SourceStatusActive  SourceStatus = "active"
	SourceStatusOffline SourceStatus = "offline"
	SourceStatusRemoved SourceStatus = "removed"
)

type FileStatus string

const (
	FileStatusOnline  FileStatus = "online"
	FileStatusOffline FileStatus = "offline"
	FileStatusMissing FileStatus = "missing"
)

type CollectionKind string

const (
	CollectionKindManual CollectionKind = "manual"
	CollectionKindSmart  CollectionKind = "smart"
)

type GroupRole string

const (
	GroupRoleRAW         GroupRole = "raw"
	GroupRoleJPEGSidecar GroupRole = "jpeg_sidecar"
	GroupRoleSource      GroupRole = "source"
	GroupRoleExport      GroupRole = "export"
	GroupRoleMember      GroupRole = "member"
)

type Asset struct {
	ID           string
	SourceID     string
	RelativePath string
	FileStatus   FileStatus
	LastVerifiedAt *time.Time

	Filename  string
	Extension string
	MIMEType  string
	FileType  FileType
	SizeBytes int64
	MTime     time.Time
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

type Source struct {
	ID               string
	Name             string
	Kind             SourceKind
	BasePath         string
	FilesystemUUID   *string
	DiskSerial       *string
	VolumeLabel      *string
	Host             *string
	ShareName        *string
	PollIntervalSecs *int
	ScanRecursively  bool
	Status           SourceStatus
	LastScannedAt    *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Tag struct {
	ID        string
	Name      string
	Slug      string
	ParentID  *string
	Color     *string
	CreatedAt time.Time
}

type AssetTag struct {
	AssetID   string
	TagID     string
	Source    string
	CreatedAt time.Time
}

type Collection struct {
	ID           string
	Name         string
	ParentID     *string
	Kind         CollectionKind
	Query        *string
	CoverAssetID *string
	SortField    *string
	SortDir      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Duplicate struct {
	ID               string
	OriginalAssetID  string
	DuplicateAssetID string
	PartialHash      string
	DetectedAt       time.Time
	Status           string
	ResolvedAt       *time.Time
}

type AssetGroup struct {
	ID           string
	CoverAssetID *string
	CreatedAt    time.Time
}

type AssetGroupMember struct {
	GroupID string
	AssetID string
	Role    GroupRole
}

type Settings struct {
	XMPConflictResolution string
	ThumbnailQuality      int
	ImportBatchSize        int
	CatalogBackupCount    int
	UndoStackSize         int
	UpdateCheckEnabled    bool
	DefaultSortField      string
	DefaultSortDir        string
}

// Opt is a three-state type for sparse patch updates:
// Set=false means "don't touch", Set=true with Value=nil means "clear", Set=true with non-nil Value means "set".
type Opt[T any] struct {
	Set   bool
	Value *T
}

func SetOpt[T any](v T) Opt[T] {
	return Opt[T]{Set: true, Value: &v}
}

func ClearOpt[T any]() Opt[T] {
	return Opt[T]{Set: true, Value: nil}
}

type AssetPatch struct {
	Rating           Opt[int]
	ColorLabel       Opt[ColorLabel]
	Flag             Opt[Flag]
	Note             Opt[string]
	ThumbnailPath    Opt[string]
	ThumbnailAt      Opt[time.Time]
	XMPLastReadAt    Opt[time.Time]
	XMPLastWrittenAt Opt[time.Time]
	XMPHash          Opt[string]
	IsDeleted        Opt[bool]
	DeletedAt        Opt[time.Time]
}

type AssetFilter struct {
	FileTypes      []FileType
	Rating         *int
	RatingMin      *int
	ColorLabels    []ColorLabel
	Flags          []Flag
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

// FileStat is used by ListKnownFiles for the scanner skip map.
type FileStat struct {
	MTime       time.Time
	SizeBytes   int64
	PartialHash string
}

// Keybinding action constants.
const (
	ActionRate0 = "rate_0"
	ActionRate1 = "rate_1"
	ActionRate2 = "rate_2"
	ActionRate3 = "rate_3"
	ActionRate4 = "rate_4"
	ActionRate5 = "rate_5"

	ActionFlagPick   = "flag_pick"
	ActionFlagReject = "flag_reject"
	ActionFlagClear  = "flag_clear"

	ActionLabelRed    = "label_red"
	ActionLabelOrange = "label_orange"
	ActionLabelYellow = "label_yellow"
	ActionLabelGreen  = "label_green"
	ActionLabelBlue   = "label_blue"
	ActionLabelPurple = "label_purple"
	ActionLabelClear  = "label_clear"

	ActionNavNext    = "nav_next"
	ActionNavPrev    = "nav_prev"
	ActionNavNextRow = "nav_next_row"
	ActionNavPrevRow = "nav_prev_row"

	ActionToggleFullscreen = "toggle_fullscreen"
	ActionToggleDetail     = "toggle_detail"
	ActionZoomIn           = "zoom_in"
	ActionZoomOut          = "zoom_out"

	ActionOpenInApp       = "open_in_app"
	ActionAddToCollection = "add_to_collection"
	ActionSelectAll       = "select_all"
	ActionDeselectAll     = "deselect_all"

	ActionUndo   = "undo"
	ActionRedo   = "redo"
	ActionDelete = "delete"
)
