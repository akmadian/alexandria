// Package seam is the boundary between the Go engine and the React UI: the
// structs Wails binds to the webview and generates TypeScript for. Services here
// stay thin — they translate bound method calls into engine calls and shape the
// results; the business logic lives in the engine packages (internal/catalog,
// internal/ast, …), never here. Go domain models are the single source of truth;
// the TS the frontend consumes is generated from these signatures (C13).
package seam

import (
	"context"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/domain"
)

// The vocabulary generator (./generate) emits the frontend's TokenField /
// TokenOperator / ValueKind unions + grammar table from internal/ast, the domain
// enum unions from internal/domain, and the ApiErrorKind / ErrorCode unions from
// this package. Output is committed and freshness-gated, not built into wails —
// prefer `make generate-seam`; this keeps `go generate ./...` working too. Paths
// are relative to this package dir (go:generate's cwd).
//go:generate go run ./generate -out ../../frontend/src/_generated-types

// sourceRepository is the slice of the source repository the seam needs — a local
// capability interface (not the concrete repo) so the service is unit-testable
// without a database. Matches sqlite.SourceRepo's read + user-editable write
// methods. Connectivity is an OBSERVATION (the volume monitor / reconciler writes
// it), so it is intentionally absent here — the user edits Enabled, not reachability.
type sourceRepository interface {
	List(ctx context.Context) ([]*domain.Source, error)
	Get(ctx context.Context, id string) (*domain.Source, error)
	Create(ctx context.Context, source *domain.Source) error
	Update(ctx context.Context, source *domain.Source) error
}

// SourceService exposes source-management reads and writes to the UI.
type SourceService struct {
	sources sourceRepository
}

// NewSourceService constructs the bound service over a source repository.
func NewSourceService(sources sourceRepository) *SourceService {
	return &SourceService{sources: sources}
}

// SourceInput creates a source. Kind defaults to local when empty; the reconciler
// later corrects Connectivity from the real mount state.
type SourceInput struct {
	Name            string            `json:"name"`
	Kind            domain.SourceKind `json:"kind,omitempty"`
	BasePath        string            `json:"basePath"`
	ScanRecursively bool              `json:"scanRecursively,omitempty"`
}

// SourcePatch updates a source's user-editable fields. Every field is a pointer:
// nil leaves the existing value untouched. Enabled is the user's activate/
// deactivate judgment (row #6 split); Connectivity is not patchable — it is an
// observation the monitor owns.
type SourcePatch struct {
	Name            *string `json:"name,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
	ScanRecursively *bool   `json:"scanRecursively,omitempty"`
}

// ListSources returns every configured source. Wails generates both its TS
// signature and the domain.Source model from this method (C13).
func (s *SourceService) ListSources() ([]*domain.Source, error) {
	sources, err := s.sources.List(seamContext())
	if err != nil {
		log.Error("seam: ListSources failed", "err", err)
		return nil, normalizeError(err)
	}
	log.Debug("seam: listed sources", "count", len(sources))
	return sources, nil
}

// CreateSource mints a source from the input and returns it. A new source is
// enabled (the user just added it) and optimistically online; the reconciler
// corrects connectivity from the real mount.
func (s *SourceService) CreateSource(input SourceInput) (*domain.Source, error) {
	if input.Name == "" || input.BasePath == "" {
		return nil, normalizeError(&domain.ValidationError{
			Field:   "source",
			Message: "name and basePath are required",
		})
	}
	kind := input.Kind
	if kind == "" {
		kind = domain.SourceKindLocal
	}
	source := &domain.Source{
		ID:              domain.NewID(),
		Name:            input.Name,
		Kind:            kind,
		BasePath:        input.BasePath,
		ScanRecursively: input.ScanRecursively,
		Enabled:         true,
		Connectivity:    domain.SourceOnline,
	}
	if err := s.sources.Create(seamContext(), source); err != nil {
		log.Error("seam: CreateSource failed", "name", input.Name, "err", err)
		return nil, normalizeError(err)
	}
	log.Info("seam: created source", "id", source.ID, "kind", source.Kind)
	return source, nil
}

// UpdateSource applies the patch to an existing source (read-modify-write).
func (s *SourceService) UpdateSource(id string, patch SourcePatch) error {
	source, err := s.sources.Get(seamContext(), id)
	if err != nil {
		return normalizeError(err)
	}
	if patch.Name != nil {
		source.Name = *patch.Name
	}
	if patch.Enabled != nil {
		source.Enabled = *patch.Enabled
	}
	if patch.ScanRecursively != nil {
		source.ScanRecursively = *patch.ScanRecursively
	}
	if err := s.sources.Update(seamContext(), source); err != nil {
		log.Error("seam: UpdateSource failed", "id", id, "err", err)
		return normalizeError(err)
	}
	log.Info("seam: updated source", "id", id)
	return nil
}
