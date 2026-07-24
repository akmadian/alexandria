package catalog

import (
	"context"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/domain"
)

// VolumeRepository is the identity/portability-anchor store (D24 split). Volumes
// are found-or-created by filesystem UUID (the path resolver's job) — the mount
// point is never stored.
type VolumeRepository interface {
	List(ctx context.Context) ([]*domain.Volume, error)
	Get(ctx context.Context, id string) (*domain.Volume, error)
	Create(ctx context.Context, volume *domain.Volume) error
	Update(ctx context.Context, volume *domain.Volume) error
	SetConnectivity(ctx context.Context, id string, c domain.VolumeConnectivity) error
	FindByFilesystemUUID(ctx context.Context, uuid string) (*domain.Volume, error)
}

// FolderRepository is the tracked-root store (D24 split): the directories the
// catalog walks/watches, with their sync scope. Roots on one volume are disjoint
// by invariant (D41) — the folder-add engine (internal/volume) enforces it.
//
// Update is the user-action path's whole-row write (it spans [jdg] columns like
// sync_mode); a scanner recording completion must hold FolderScanRecorder
// instead — never the fat Update (per-column writer-class enforcement, D41).
type FolderRepository interface {
	List(ctx context.Context) ([]*domain.Folder, error)
	Get(ctx context.Context, id string) (*domain.Folder, error)
	ListByVolume(ctx context.Context, volumeID string) ([]*domain.Folder, error)
	Create(ctx context.Context, folder *domain.Folder) error
	Update(ctx context.Context, folder *domain.Folder) error
	Delete(ctx context.Context, id string) error
	FolderScanRecorder
}

// FolderScanRecorder is the sync-state writer slice of the folder table: the
// [syn] last_scanned_at cursor and nothing else. Inject THIS into the scanner /
// reconciler so a scan completion structurally cannot touch a judgment column.
type FolderScanRecorder interface {
	SetLastScannedAt(ctx context.Context, id string, scannedAt time.Time) error
}

// The asset repository is split by writer CLASS (see docs/data-model.md
// §1). Each consumer is injected only the interface for the columns it is allowed
// to touch, so a cross-class write is a compile error, not a code-review catch.
// One concrete type (sqlite.AssetRepo) satisfies them all; scoping happens at
// injection.

// AssetReader — read-only. Anyone may hold it.
type AssetReader interface {
	Get(ctx context.Context, id string) (*domain.Asset, error)
	FindByHash(ctx context.Context, hash string, sizeBytes int64) (*domain.Asset, error)
	// FindByVolumePath matches on (volume_id, path_key): the NFC key form, so an
	// NFD-stored name matches its NFC query form (D24). relativePath is the raw
	// (byte) path; the adapter derives the key.
	FindByVolumePath(ctx context.Context, volumeID, relativePath string) (*domain.Asset, error)
	// ListKnownFiles returns the skip map for one volume subtree — every ONLINE
	// asset whose path is under pathPrefix (volume-relative; "" = the whole
	// volume). Keyed by path_key (the NFC comparison form).
	ListKnownFiles(ctx context.Context, volumeID, pathPrefix string) (map[string]domain.FileStat, error)
	// ListPathsStatus is the reconciliation projection for one volume subtree
	// (volume-relative pathPrefix; "" = the whole volume).
	ListPathsStatus(ctx context.Context, volumeID, pathPrefix string) ([]PathStatus, error)

	// Query-layer methods (impl/13). Each is a new result SHAPE (C7).
	QueryAssets(ctx context.Context, query ast.Query, arrangement ast.Arrangement, page ast.Page) ([]AssetRow, int, error)
	AssetIDSlice(ctx context.Context, query ast.Query, arrangement ast.Arrangement, fromIndex, toIndex int) ([]string, error)
	IndexOfAsset(ctx context.Context, query ast.Query, arrangement ast.Arrangement, id string) (*int, error)
	DistinctValues(ctx context.Context, field ast.Field) ([]string, error)
	ReadTriageStates(ctx context.Context, ids []string) ([]TriageState, error)
}

// AssetObservationWriter — ingest / watcher / reconciler ONLY. No judgment,
// sync, or derived column is reachable through it.
type AssetObservationWriter interface {
	Create(ctx context.Context, asset *domain.Asset) error // minting; judgment fields must be zero
	ApplyFilePatch(ctx context.Context, id string, p *FilePatch) error
	UpdatePath(ctx context.Context, assetID, volumeID, relativePath string) error
	SetFileStatus(ctx context.Context, assetID string, status domain.FileStatus) error
	MarkConnectivityByVolume(ctx context.Context, volumeID string, online bool) error
}

// AssetJudgmentWriter — the user-action service ONLY. Every write bumps
// judgment_modified_at (this is the single code path that does).
type AssetJudgmentWriter interface {
	ApplyTriagePatch(ctx context.Context, ids []string, p TriagePatch) error
	ApplyTriagePatchByQuery(ctx context.Context, query ast.Query, exceptIDs []string, p TriagePatch) ([]string, error)
	SoftDelete(ctx context.Context, ids []string) error
}

// AssetSyncWriter — XMP sync ONLY. Applies inbound judgment VALUES under the
// conflict policy but must NEVER bump judgment_modified_at; owns the xmp_* cursor.
type AssetSyncWriter interface {
	ApplyXMPInbound(ctx context.Context, id string, p TriagePatch, readAt time.Time, xmpHash string) error
	RecordXMPWritten(ctx context.Context, id string, writtenAt time.Time, xmpHash string) error
}

// AssetDerivedWriter — the enrichment writer goroutine, plus ingest's reimport
// path for ClearDerived ONLY (the D28 staleness transition: a reimport's
// transaction clears derived columns so the enrichment scan re-derives them —
// the one sanctioned derived write outside the engine).
type AssetDerivedWriter interface {
	SetThumbnailAt(ctx context.Context, id string, t time.Time) error
	SetSharpness(ctx context.Context, id string, value float64) error
	SetClipping(ctx context.Context, id string, highlights, shadows float64) error
	SetPhash(ctx context.Context, id string, hash string) error
	ClearDerived(ctx context.Context, id string) error
}

// TagRepository is the tag surface. Today it exposes only the keyword-import
// path (the seam XMP sync and LrC migration depend on); the tag-management UI
// methods (Tree/Get/Update/Delete/SetAssetTags) will land when that UI is the
// caller.
type TagRepository interface {
	ImportKeywords(ctx context.Context, assetID string, flat []string, hierarchical [][]string, source string) error
	// AssetTagNames returns the active (non-tombstoned) tag names for an asset,
	// split into flat names and pipe-delimited hierarchical paths for XMP write-back.
	AssetTagNames(ctx context.Context, assetID string) (flat []string, hierarchical []string, err error)
}

type CollectionRepository interface {
	List(ctx context.Context) ([]*domain.Collection, error)
	Get(ctx context.Context, id string) (*domain.Collection, error)
	Create(ctx context.Context, collection *domain.Collection) error
	Update(ctx context.Context, collection *domain.Collection) error
	Delete(ctx context.Context, id string) error
	AddAsset(ctx context.Context, collectionID, assetID string) error
	RemoveAsset(ctx context.Context, collectionID, assetID string) error
}

type DuplicateRepository interface {
	Log(ctx context.Context, dup *domain.Duplicate) error
	ListPending(ctx context.Context) ([]*domain.Duplicate, error)
}
