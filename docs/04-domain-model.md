# Domain Model & Interfaces

## Overview

The domain model lives in `internal/domain/`. It is pure Go — no external dependencies, no database imports, no Wails imports. Every other package in `internal/` imports `domain`. Nothing in `domain/` imports anything outside the Go standard library.

This isolation is intentional. The domain types are the lingua franca of the application. They can be used in tests without spinning up a database, passed across the IPC boundary to the frontend (via JSON serialisation), and reasoned about independently of infrastructure concerns.

---

## Domain types

### Asset

```
Asset
  ID              string          -- UUID
  Filename        string
  Extension       string          -- lowercase, no dot: "jpg", "psd", "mp4"
  MIMEType        string
  FileType        FileType        -- coarse category enum
  SizeBytes       int64
  MTime           time.Time       -- last modified, from filesystem
  PartialHash     string          -- xxHash of first 64KB + size

  Width           *int            -- nil for non-visual
  Height          *int
  DurationSecs    *float64        -- nil for non-temporal

  ColorSpace      *string
  BitDepth        *int

  CapturedAt      *time.Time      -- from EXIF DateTimeOriginal
  CameraMake      *string
  CameraModel     *string
  LensModel       *string
  FocalLengthMM   *float64
  Aperture        *float64        -- f-number
  ShutterSpeed    *string         -- "1/250", "2", etc.
  ISO             *int
  GPSLat          *float64
  GPSLon          *float64

  ExtendedMetadata map[string]any  -- format-specific fields, not queried

  Rating          *int            -- 0–5, nil = unrated
  ColorLabel      *ColorLabel
  Flag            *Flag

  XMPLastReadAt    *time.Time
  XMPLastWrittenAt *time.Time
  XMPHash          *string

  ThumbnailPath   *string         -- relative to app data dir
  ThumbnailAt     *time.Time

  IsDeleted       bool
  DeletedAt       *time.Time
  IngestedAt      time.Time
  UpdatedAt       time.Time
```

### FileType (enum)

```
FileType string

FileTypeImage    = "image"
FileTypeVideo    = "video"
FileTypeRaw      = "raw"
FileTypeVector   = "vector"
FileTypeDocument = "document"
FileTypeAudio    = "audio"
```

### ColorLabel (enum)

```
ColorLabel string

ColorLabelRed    = "red"
ColorLabelOrange = "orange"
ColorLabelYellow = "yellow"
ColorLabelGreen  = "green"
ColorLabelBlue   = "blue"
ColorLabelPurple = "purple"
```

### Flag (enum)

```
Flag string

FlagPick   = "pick"
FlagReject = "reject"
```

### Source

```
Source
  ID               string
  Name             string
  Kind             SourceKind
  BasePath         string          -- absolute path or UNC path
  FilesystemUUID   *string         -- primary drive identity
  DiskSerial       *string         -- secondary drive identity (fallback)
  VolumeLabel      *string         -- display only, never used for matching
  Host             *string         -- network shares only
  ShareName        *string         -- network shares only
  PollIntervalSecs *int            -- nil = use filesystem events
  ScanRecursively  bool
  Status           SourceStatus
  LastScannedAt    *time.Time
  CreatedAt        time.Time
  UpdatedAt        time.Time
```

### SourceKind (enum)

```
SourceKind string

SourceKindLocal         = "local"
SourceKindExternalDrive = "external_drive"
SourceKindSMB           = "smb"
SourceKindNFS           = "nfs"
```

### SourceStatus (enum)

```
SourceStatus string

SourceStatusActive  = "active"
SourceStatusOffline = "offline"
SourceStatusRemoved = "removed"
```

### Location

```
Location
  ID              string
  AssetID         string
  SourceID        string
  RelativePath    string          -- relative to source.BasePath
  Filename        string          -- denormalized from RelativePath
  SizeBytes       int64
  MTime           time.Time
  PartialHash     *string
  Status          LocationStatus
  LastVerifiedAt  *time.Time
  CreatedAt       time.Time
  UpdatedAt       time.Time
```

### LocationStatus (enum)

```
LocationStatus string

LocationStatusOnline  = "online"
LocationStatusOffline = "offline"
LocationStatusMissing = "missing"
LocationStatusMoved   = "moved"
```

### Tag

```
Tag
  ID        string
  Name      string
  Slug      string
  ParentID  *string
  Color     *string
  CreatedAt time.Time
```

### AssetTag

```
AssetTag
  AssetID   string
  TagID     string
  Source    string    -- "user", "xmp", "lr"
  CreatedAt time.Time
```

### Collection

```
Collection
  ID           string
  Name         string
  ParentID     *string
  Kind         CollectionKind
  Query        *string         -- JSON query definition for smart collections
  CoverAssetID *string
  SortField    *string
  SortDir      string
  CreatedAt    time.Time
  UpdatedAt    time.Time
```

### CollectionKind (enum)

```
CollectionKind string

CollectionKindManual = "manual"
CollectionKindSmart  = "smart"
```

### AssetGroup

```
AssetGroup
  ID           string
  CoverAssetID *string
  CreatedAt    time.Time
```

### AssetGroupMember

```
AssetGroupMember
  GroupID  string
  AssetID  string
  Role     GroupRole
```

### GroupRole (enum)

```
GroupRole string

GroupRoleRAW         = "raw"
GroupRoleJPEGSidecar = "jpeg_sidecar"
GroupRoleSource      = "source"
GroupRoleExport      = "export"
GroupRoleMember      = "member"
```

### AssetMetadata

A transient type used during ingest to carry extracted metadata before it is written to the catalog. Not persisted directly.

```
AssetMetadata
  -- All fields from Asset that are extracted from the file
  -- (dimensions, EXIF, IPTC, XMP, duration, etc.)
  -- Populated by metadata extractors, merged into an Asset record by the catalog writer.
```

### Settings

A typed struct that maps to the settings table. Loaded at startup and held in memory. Updated via the settings service.

```
Settings
  XMPConflictResolution  string    -- "xmp_wins" | "catalog_wins"
  DuplicateHandling      string    -- "auto_drop" | "review"
  ThumbnailQuality       int
  HashWorkerCount        int
  ExtractWorkerCount     int
  ThumbWorkerCount       int
  ImportBatchSize        int
  MemoryLimitMB          int
  CatalogBackupCount     int
  UndoStackSize          int
  UpdateCheckEnabled     bool
  DefaultSortField       string
  DefaultSortDir         string
```

---

## Error types

Typed errors allow callers to make decisions based on error type, not string matching. Defined in `internal/domain/errors.go`.

```
NotFoundError
  Resource  string    -- "asset", "source", "collection", etc.
  ID        string

ConflictError
  Resource  string
  Field     string
  Message   string

SourceOfflineError
  SourceID  string
  Path      string

CatalogLockedError
  Path      string    -- path to the catalog file

ValidationError
  Field     string
  Message   string

ErrKeybindingConflict
  Combo          string
  ConflictAction string

ErrSchemaTooOld
  Current   int
  Required  int

ErrSchemaTooNew
  Current   int
  Known     int
```

---

## Keybinding action constants

All keyboard action identifiers are string constants defined in `internal/domain/keybindings.go`. These are the stable names stored in the database and referenced by the frontend.

```
-- Rating
ActionRate0 = "rate_0"    -- clear rating
ActionRate1 = "rate_1"
ActionRate2 = "rate_2"
ActionRate3 = "rate_3"
ActionRate4 = "rate_4"
ActionRate5 = "rate_5"

-- Flags
ActionFlagPick        = "flag_pick"
ActionFlagReject      = "flag_reject"
ActionFlagClear       = "flag_clear"

-- Color labels
ActionLabelRed        = "label_red"
ActionLabelOrange     = "label_orange"
ActionLabelYellow     = "label_yellow"
ActionLabelGreen      = "label_green"
ActionLabelBlue       = "label_blue"
ActionLabelPurple     = "label_purple"
ActionLabelClear      = "label_clear"

-- Navigation
ActionNavNext         = "nav_next"
ActionNavPrev         = "nav_prev"
ActionNavNextRow      = "nav_next_row"
ActionNavPrevRow      = "nav_prev_row"

-- View
ActionToggleFullscreen = "toggle_fullscreen"
ActionToggleDetail    = "toggle_detail"
ActionZoomIn          = "zoom_in"
ActionZoomOut         = "zoom_out"

-- Operations
ActionOpenInApp       = "open_in_app"
ActionAddToCollection = "add_to_collection"
ActionSelectAll       = "select_all"
ActionDeselectAll     = "deselect_all"

-- Catalog
ActionUndo            = "undo"
ActionRedo            = "redo"
ActionDelete          = "delete"           -- triggers soft delete confirmation modal
```

---

## Repository interfaces

Defined in `internal/catalog/interfaces.go`. Concrete SQLite implementations live alongside. Test implementations (backed by in-memory SQLite) are provided via `testutil`.

### AssetRepository

```
AssetRepository
  Get(ctx, id) → (*Asset, error)
  List(ctx, filter AssetFilter) → ([]*Asset, error)
  Create(ctx, asset *Asset) → error
  Update(ctx, asset *Asset) → error
  Patch(ctx, id, patch AssetPatch) → error
  BulkPatch(ctx, ids []string, patch AssetPatch) → error
  BulkPatchIndividual(ctx, patches map[string]AssetPatch) → error
  SoftDelete(ctx, id) → error
  FindByHash(ctx, hash string, sizeBytes int64) → (*Asset, error)
```

### AssetFilter

Used by `AssetRepository.List()` and by the smart collection query evaluator. Covers all fields that can be filtered, sorted, or paginated in the grid.

```
AssetFilter
  FileTypes       []FileType
  Rating          *int          -- exact match
  RatingMin       *int          -- minimum rating (inclusive)
  ColorLabels     []ColorLabel
  Flags           []Flag
  TagIDs          []string
  SourceIDs       []string
  CapturedAfter   *time.Time
  CapturedBefore  *time.Time
  SearchText      string        -- full-text search via FTS5
  IncludeDeleted  bool
  SortField       string
  SortDir         string        -- "asc" | "desc"
  Limit           int
  Offset          int
```

### AssetPatch

A sparse update struct. Only non-nil fields are written. Used for partial updates (rating, label, etc.) without requiring a full asset read-modify-write cycle.

```
AssetPatch
  Rating          *int
  ColorLabel      **ColorLabel  -- pointer to pointer so nil = "don't update", &nil = "clear"
  Flag            **Flag
  ThumbnailPath   *string
  ThumbnailAt     *time.Time
  XMPLastReadAt   *time.Time
  XMPLastWrittenAt *time.Time
  XMPHash         *string
  IsDeleted       *bool
  DeletedAt       *time.Time
  XMPKeywords     []string      -- tag names from XMP, merged into asset_tags
```

### LocationRepository

```
LocationRepository
  GetByAsset(ctx, assetID) → ([]*Location, error)
  Create(ctx, location *Location) → error
  Update(ctx, location *Location) → error
  FindBySourcePath(ctx, sourceID, relativePath) → (*Location, error)
  FindByAbsPath(ctx, absPath) → (*Location, error)
  MarkOfflineBySource(ctx, sourceID) → error
  UpdateStatus(ctx, id, status LocationStatus) → error
```

### SourceRepository

```
SourceRepository
  List(ctx) → ([]*Source, error)
  Get(ctx, id) → (*Source, error)
  Create(ctx, source *Source) → error
  Update(ctx, source *Source) → error
  UpdateStatus(ctx, id, status SourceStatus) → error
  FindByFilesystemUUID(ctx, uuid) → (*Source, error)
  FindBySharePath(ctx, host, shareName) → (*Source, error)
```

### TagRepository

```
TagRepository
  Tree(ctx) → ([]*Tag, error)      -- returns full tag tree
  Get(ctx, id) → (*Tag, error)
  Create(ctx, tag *Tag) → error
  Update(ctx, tag *Tag) → error
  Delete(ctx, id) → error
  GetByAsset(ctx, assetID) → ([]*AssetTag, error)
  SetAssetTags(ctx, assetID, tagIDs []string, source string) → error
```

### CollectionRepository

```
CollectionRepository
  List(ctx) → ([]*Collection, error)
  Get(ctx, id) → (*Collection, error)
  Create(ctx, collection *Collection) → error
  Update(ctx, collection *Collection) → error
  Delete(ctx, id) → error
  GetAssets(ctx, collectionID, filter AssetFilter) → ([]*Asset, error)
  AddAsset(ctx, collectionID, assetID) → error
  RemoveAsset(ctx, collectionID, assetID) → error
```

---

## Platform interfaces

Defined in `internal/platform/interfaces.go`. Implementations live in `internal/platform/darwin/`, `internal/platform/linux/`, `internal/platform/windows/`. The correct implementation is selected at startup and injected into services. Tests use stub implementations.

### FileWatcher

Watches a filesystem path and emits events when files are created, modified, deleted, or renamed.

```
FileWatcher
  Watch(path string) → (<-chan FileEvent, error)
  Unwatch(path string) → error
  Close()

FileEvent
  Kind    FileEventKind     -- "created", "modified", "deleted", "renamed"
  Path    string
  OldPath string            -- populated for renamed events only
```

**Implementation notes:**
- macOS: FSEvents via `fsnotify`
- Linux: inotify via `fsnotify`
- Windows: ReadDirectoryChangesW via `fsnotify`
- Network shares: FileWatcher is not used. Network sources use polling instead.
- The watcher consumer is responsible for debouncing — events for the same path within 500ms should be collapsed before acting on them.

### VolumeMonitor

Monitors the system for volumes being mounted or unmounted. Used to detect when external drives are connected or disconnected.

```
VolumeMonitor
  Events() → <-chan VolumeEvent

VolumeEvent
  Kind       VolumeEventKind    -- "mounted", "unmounted"
  MountPath  string
```

**Implementation notes:**
- macOS: DiskArbitration framework or `diskutil` polling
- Linux: udev events or `/proc/mounts` polling
- Windows: WM_DEVICECHANGE messages

### DriveIdentifier

Reads identity information from a mounted volume. Used during source reconnection.

```
DriveIdentifier
  FilesystemUUID(mountPath string) → (string, error)
  DiskSerial(mountPath string) → (string, error)
  VolumeLabel(mountPath string) → (string, error)
```

### Opener

Opens a file in its default application, or in a specified application.

```
Opener
  Open(path string) → error
  OpenWith(path string, appName string) → error
```

**Implementation notes:**
- macOS: `open` command / NSWorkspace
- Linux: `xdg-open`
- Windows: `ShellExecute`

### Thumbnailer

Generates a thumbnail for a file. Multiple implementations exist, dispatched by MIME type.

```
Thumbnailer
  Generate(ctx, req ThumbRequest) → (*ThumbResult, error)
  Supports(mimeType string) → bool

ThumbRequest
  SourcePath  string
  OutputPath  string
  MaxWidth    int
  MaxHeight   int
  MIMEType    string

ThumbResult
  Width   int
  Height  int
```

**Implementations:**
- `ImageThumbnailer`: handles JPEG, PNG, TIFF, WebP, GIF, BMP via Go image libraries
- `RawThumbnailer`: handles RAW formats via libraw bindings
- `VideoThumbnailer`: extracts a frame via FFmpeg bindings
- `PSDThumbnailer`: extracts embedded composite via ImageMagick or psd library
- `PDFThumbnailer`: first page via Ghostscript
- `VectorThumbnailer`: SVG rasterisation
- `EmbeddedPreviewThumbnailer`: reads embedded preview from file header (Affinity, InDesign)
- `DispatchThumbnailer`: wraps all of the above, routes by MIME type

### MetadataExtractor

Extracts metadata from a file into an AssetMetadata struct. Multiple implementations dispatched by MIME type.

```
MetadataExtractor
  Extract(ctx, path string) → (*AssetMetadata, error)
  Supports(mimeType string) → bool
```

**Implementations:**
- `EXIFExtractor`: handles images with EXIF data (JPEG, TIFF, RAW)
- `VideoExtractor`: extracts stream info via FFmpeg
- `XMPExtractor`: reads embedded or sidecar XMP
- `DispatchMetadataExtractor`: routes by MIME type

### Hasher

Computes the partial hash for a file.

```
Hasher
  Hash(path string) → (string, error)
```

**Implementation:** reads first 64KB of file, computes xxHash, appends file size. Uses `cespare/xxhash`.

---

## Package dependency graph

```
cmd/alexandria
    └── app/
            ├── internal/commands/
            │       └── internal/catalog/
            │               └── internal/domain/
            ├── internal/ingest/
            │       ├── internal/catalog/
            │       ├── internal/thumbnailer/
            │       ├── internal/metadata/
            │       └── internal/domain/
            ├── internal/watcher/
            │       ├── internal/ingest/
            │       ├── internal/platform/ (interfaces)
            │       └── internal/domain/
            ├── internal/xmp/
            │       ├── internal/catalog/
            │       └── internal/domain/
            └── internal/platform/
                    ├── darwin/
                    ├── linux/
                    └── windows/
```

`internal/domain` has no internal dependencies. Everything else flows toward it. `app/` is the only package that imports Wails. `cmd/alexandria` is the only entry point.
