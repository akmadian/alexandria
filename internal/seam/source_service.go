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

// sourceLister is the slice of the source repository the seam needs. Kept as a
// local interface so the service is unit-testable without a database and depends
// on a capability, not a concrete repo.
type sourceLister interface {
	List(ctx context.Context) ([]*domain.Source, error)
}

// SourceService exposes source-management reads and writes to the UI. impl/14
// binds one method — ListSources — as the walking skeleton that proves the
// Go → Wails binding → generated TS → webview pipe end to end; impl/15 fills in
// the rest of the surface on this same struct.
type SourceService struct {
	sources sourceLister
}

// NewSourceService constructs the bound service over a source repository.
func NewSourceService(sources sourceLister) *SourceService {
	return &SourceService{sources: sources}
}

// ListSources returns every configured source. Wails generates both its TS
// signature (ListSources(): Promise<domain.Source[]>) and the domain.Source
// model from this method (C13).
//
// ponytail: uses context.Background() — the walking skeleton proves generation
// and transport, not request cancellation. Wails v2 bound methods receive no
// per-call context; impl/15 picks the context strategy for the real surface
// (startup-captured context vs. per-call background) once there are methods that
// warrant cancellation.
func (s *SourceService) ListSources() ([]*domain.Source, error) {
	sources, err := s.sources.List(context.Background())
	if err != nil {
		log.Error("seam: ListSources failed", "err", err)
		return nil, err
	}
	log.Debug("seam: listed sources", "count", len(sources))
	return sources, nil
}
