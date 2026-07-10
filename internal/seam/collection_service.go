package seam

import (
	"context"
	"encoding/json"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/domain"
)

// collectionRepository is the collection slice the seam needs — matches
// catalog.CollectionRepository so sqlite.CollectionRepo satisfies it. The repo
// already ast.Validates a smart collection's stored query on write; the service
// validates too, early, so an invalid query returns query_invalid rather than a
// wrapped repo error.
type collectionRepository interface {
	List(ctx context.Context) ([]*domain.Collection, error)
	Get(ctx context.Context, id string) (*domain.Collection, error)
	Create(ctx context.Context, collection *domain.Collection) error
	Update(ctx context.Context, collection *domain.Collection) error
	Delete(ctx context.Context, id string) error
	AddAsset(ctx context.Context, collectionID, assetID string) error
	RemoveAsset(ctx context.Context, collectionID, assetID string) error
}

// CollectionService exposes collection CRUD + membership (ledger #9). Collections
// are scopes, not filter fields (C1) — a smart collection persists an AST query
// (with a version, C6) that the query system reuses; a manual collection is a
// membership list. Both live in one table, discriminated by kind.
type CollectionService struct {
	collections collectionRepository
}

// NewCollectionService constructs the bound service over the collection repo.
func NewCollectionService(collections collectionRepository) *CollectionService {
	return &CollectionService{collections: collections}
}

// CollectionInput creates a collection. A smart collection carries an AST query;
// a manual one carries none. Kind defaults to manual when empty.
type CollectionInput struct {
	Name     string                `json:"name"`
	ParentID *string               `json:"parentId,omitempty"`
	Kind     domain.CollectionKind `json:"kind,omitempty"`
	Query    *ast.Query            `json:"query,omitempty"`
}

// CollectionPatch updates a collection. Every field is a pointer: nil leaves the
// existing value untouched (read-modify-write). Clearing parentId/coverAssetId to
// root/none is a tree-management operation the rebuild's collection UI will own —
// see DEFERRED §7.
type CollectionPatch struct {
	Name         *string    `json:"name,omitempty"`
	CoverAssetID *string    `json:"coverAssetId,omitempty"`
	Query        *ast.Query `json:"query,omitempty"`
}

// ListCollections returns every collection, ordered by name.
func (s *CollectionService) ListCollections() ([]*domain.Collection, error) {
	collections, err := s.collections.List(seamContext())
	if err != nil {
		log.Error("seam: ListCollections failed", "err", err)
		return nil, normalizeError(err)
	}
	log.Debug("seam: listed collections", "count", len(collections))
	return collections, nil
}

// CreateCollection mints a collection from the input and returns it. For a smart
// collection the query is validated and stored as JSON; a manual collection stores
// no query.
func (s *CollectionService) CreateCollection(input CollectionInput) (*domain.Collection, error) {
	kind := input.Kind
	if kind == "" {
		kind = domain.CollectionKindManual
	}
	// Reject contradictory shapes the repo's narrower guard would let through: a
	// smart collection with no query persists a useless row the scope resolver
	// can't read; a manual collection carrying a query stores dead data the query
	// system never consults. (Mirrors CreateSource's required-field guard.)
	if input.Name == "" {
		return nil, normalizeError(&domain.ValidationError{Field: "name", Message: "collection name is required"})
	}
	if kind == domain.CollectionKindSmart && input.Query == nil {
		return nil, normalizeError(&domain.ValidationError{Field: "query", Message: "a smart collection requires a query"})
	}
	if kind == domain.CollectionKindManual && input.Query != nil {
		return nil, normalizeError(&domain.ValidationError{Field: "query", Message: "a manual collection cannot carry a query"})
	}
	collection := &domain.Collection{
		ID:       domain.NewID(),
		Name:     input.Name,
		ParentID: input.ParentID,
		Kind:     kind,
		SortDir:  "asc",
	}
	if input.Query != nil {
		stored, err := marshalQuery(input.Query)
		if err != nil {
			return nil, normalizeError(err)
		}
		collection.Query = stored
	}
	if err := s.collections.Create(seamContext(), collection); err != nil {
		log.Error("seam: CreateCollection failed", "name", input.Name, "err", err)
		return nil, normalizeError(err)
	}
	log.Info("seam: created collection", "id", collection.ID, "kind", collection.Kind)
	return collection, nil
}

// UpdateCollection applies the patch to an existing collection (read-modify-write).
func (s *CollectionService) UpdateCollection(id string, patch CollectionPatch) error {
	collection, err := s.collections.Get(seamContext(), id)
	if err != nil {
		return normalizeError(err)
	}
	if patch.Name != nil {
		collection.Name = *patch.Name
	}
	if patch.CoverAssetID != nil {
		collection.CoverAssetID = patch.CoverAssetID
	}
	if patch.Query != nil {
		stored, err := marshalQuery(patch.Query)
		if err != nil {
			return normalizeError(err)
		}
		collection.Query = stored
	}
	if err := s.collections.Update(seamContext(), collection); err != nil {
		log.Error("seam: UpdateCollection failed", "id", id, "err", err)
		return normalizeError(err)
	}
	log.Info("seam: updated collection", "id", id)
	return nil
}

// DeleteCollection removes a collection (its membership rows cascade in the DB).
func (s *CollectionService) DeleteCollection(id string) error {
	if err := s.collections.Delete(seamContext(), id); err != nil {
		log.Error("seam: DeleteCollection failed", "id", id, "err", err)
		return normalizeError(err)
	}
	log.Info("seam: deleted collection", "id", id)
	return nil
}

// AddToCollection adds the given assets to a manual collection.
//
// ponytail: one repo call per id — the collection repo exposes only single-asset
// AddAsset/RemoveAsset. Fine for interactive selection sizes; if a bulk "add all
// N results" flow lands, give the repo a batch AddAssets (one multi-row INSERT)
// and call it here.
func (s *CollectionService) AddToCollection(collectionID string, assetIDs []string) error {
	for _, assetID := range assetIDs {
		if err := s.collections.AddAsset(seamContext(), collectionID, assetID); err != nil {
			log.Error("seam: AddToCollection failed", "collection", collectionID, "asset", assetID, "err", err)
			return normalizeError(err)
		}
	}
	log.Info("seam: added assets to collection", "collection", collectionID, "count", len(assetIDs))
	return nil
}

// RemoveFromCollection removes the given assets from a manual collection.
func (s *CollectionService) RemoveFromCollection(collectionID string, assetIDs []string) error {
	for _, assetID := range assetIDs {
		if err := s.collections.RemoveAsset(seamContext(), collectionID, assetID); err != nil {
			log.Error("seam: RemoveFromCollection failed", "collection", collectionID, "asset", assetID, "err", err)
			return normalizeError(err)
		}
	}
	log.Info("seam: removed assets from collection", "collection", collectionID, "count", len(assetIDs))
	return nil
}

// marshalQuery validates an AST query and marshals it to the JSON string the
// collection row stores. Validating here gives a query_invalid code before the
// repo's own re-validation wraps it as an opaque error.
func marshalQuery(query *ast.Query) (*string, error) {
	if err := ast.Validate(*query); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	stored := string(encoded)
	return &stored, nil
}
