package catalog

import (
	"context"

	"github.com/akmadian/alexandria/internal/domain"
)

type SourceRepository interface {
	List(ctx context.Context) ([]*domain.Source, error)
	Get(ctx context.Context, id string) (*domain.Source, error)
	Create(ctx context.Context, source *domain.Source) error
	Update(ctx context.Context, source *domain.Source) error
	UpdateStatus(ctx context.Context, id string, status domain.SourceStatus) error
	FindByFilesystemUUID(ctx context.Context, uuid string) (*domain.Source, error)
	FindBySharePath(ctx context.Context, host, shareName string) (*domain.Source, error)
}

type AssetRepository interface {
	Get(ctx context.Context, id string) (*domain.Asset, error)
	List(ctx context.Context, filter domain.AssetFilter) ([]*domain.Asset, error)
	Create(ctx context.Context, asset *domain.Asset) error
	Update(ctx context.Context, asset *domain.Asset) error
	Patch(ctx context.Context, id string, patch domain.AssetPatch) error
	BulkPatch(ctx context.Context, ids []string, patch domain.AssetPatch) error
	SoftDelete(ctx context.Context, id string) error
	FindByHash(ctx context.Context, hash string, sizeBytes int64) (*domain.Asset, error)

	FindBySourcePath(ctx context.Context, sourceID, relativePath string) (*domain.Asset, error)
	UpdatePath(ctx context.Context, assetID, sourceID, relativePath string) error
	UpdateFileStatus(ctx context.Context, assetID string, status domain.FileStatus) error
	MarkOfflineBySource(ctx context.Context, sourceID string) error
	MarkOnlineBySource(ctx context.Context, sourceID string) error
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
