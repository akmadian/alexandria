package catalog

import (
	"context"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

type SourceRepository interface {
	List(ctx context.Context) ([]*domain.Source, error)
	Get(ctx context.Context, id string) (*domain.Source, error)
	Create(ctx context.Context, source *domain.Source) error
	Update(ctx context.Context, source *domain.Source) error
	SetConnectivity(ctx context.Context, id string, c domain.SourceConnectivity) error
	FindByFilesystemUUID(ctx context.Context, uuid string) (*domain.Source, error)
	FindBySharePath(ctx context.Context, host, shareName string) (*domain.Source, error)
}

// The asset repository is split by writer CLASS (see docs/v2/.../03-data-model.md
// §1). Each consumer is injected only the interface for the columns it is allowed
// to touch, so a cross-class write is a compile error, not a code-review catch.
// One concrete type (sqlite.AssetRepo) satisfies them all; scoping happens at
// injection.

// AssetReader — read-only. Anyone may hold it.
type AssetReader interface {
	Get(ctx context.Context, id string) (*domain.Asset, error)
	List(ctx context.Context, filter AssetFilter) ([]*domain.Asset, error)
	FindByHash(ctx context.Context, hash string, sizeBytes int64) (*domain.Asset, error) // is_deleted=0 only
	FindBySourcePath(ctx context.Context, sourceID, relativePath string) (*domain.Asset, error)
	ListKnownFiles(ctx context.Context, sourceID string) (map[string]domain.FileStat, error)
	ListPathsStatus(ctx context.Context, sourceID string) ([]PathStatus, error)
}

// AssetObservationWriter — ingest / watcher / reconciler ONLY. No judgment,
// sync, or derived column is reachable through it.
type AssetObservationWriter interface {
	Create(ctx context.Context, asset *domain.Asset) error // minting; judgment fields must be zero
	ApplyFilePatch(ctx context.Context, id string, p FilePatch) error
	UpdatePath(ctx context.Context, assetID, sourceID, relativePath string) error
	SetFileStatus(ctx context.Context, assetID string, status domain.FileStatus) error
	MarkConnectivityBySource(ctx context.Context, sourceID string, online bool) error
}

// AssetJudgmentWriter — the user-action service ONLY. Every write bumps
// judgment_modified_at (this is the single code path that does).
type AssetJudgmentWriter interface {
	ApplyTriagePatch(ctx context.Context, ids []string, p TriagePatch) error
	SoftDelete(ctx context.Context, ids []string) error
}

// AssetSyncWriter — XMP sync ONLY. Applies inbound judgment VALUES under the
// conflict policy but must NEVER bump judgment_modified_at; owns the xmp_* cursor.
type AssetSyncWriter interface {
	ApplyXMPInbound(ctx context.Context, id string, p TriagePatch, readAt time.Time, xmpHash string) error
	RecordXMPWritten(ctx context.Context, id string, writtenAt time.Time, xmpHash string) error
}

// AssetDerivedWriter — jobs / ingest thumbnail stage ONLY.
type AssetDerivedWriter interface {
	SetThumbnailAt(ctx context.Context, id string, t time.Time) error
}

type TagRepository interface {
	Tree(ctx context.Context) ([]*domain.Tag, error)
	Get(ctx context.Context, id string) (*domain.Tag, error)
	Create(ctx context.Context, tag *domain.Tag) error
	Update(ctx context.Context, tag *domain.Tag) error
	Delete(ctx context.Context, id string) error
	GetByAsset(ctx context.Context, assetID string) ([]*domain.AssetTag, error)
	SetAssetTags(ctx context.Context, assetID string, tagIDs []string, source string) error
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
