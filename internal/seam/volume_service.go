// Package seam is the boundary between the Go engine and the React UI: the
// structs Wails binds to the webview and generates TypeScript for. Services here
// stay thin — they translate bound method calls into engine calls and shape the
// results; the business logic lives in the engine packages (internal/catalog,
// internal/ast, internal/volume, …), never here. Go domain models are the single
// source of truth; the TS the frontend consumes is generated from these
// signatures (C13).
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

// volumeRepository is the slice of the volume repository the seam needs — a local
// capability interface (not the concrete repo) so the service is unit-testable
// without a database. Volumes are found-or-created by the path resolver, never by
// the user, so there is no Create here; the user only reads them and renames
// them. Connectivity is an OBSERVATION (the volume monitor writes it), so it is
// intentionally absent from the user-editable surface.
type volumeRepository interface {
	List(ctx context.Context) ([]*domain.Volume, error)
	Get(ctx context.Context, id string) (*domain.Volume, error)
	Update(ctx context.Context, volume *domain.Volume) error
}

// VolumeService exposes volume reads and the user-editable writes to the UI. The
// tracked-root (folder) management surface — getFolderTree, add/remove — lands
// with the browser-rail bind (task 45); volumes are the identity anchor the
// resolver mints.
type VolumeService struct {
	volumes volumeRepository
}

// NewVolumeService constructs the bound service over a volume repository.
func NewVolumeService(volumes volumeRepository) *VolumeService {
	return &VolumeService{volumes: volumes}
}

// VolumePatch updates a volume's user-editable fields. Every field is a pointer:
// nil leaves the existing value untouched. Connectivity is not patchable — it is
// an observation the monitor owns.
type VolumePatch struct {
	Name *string `json:"name,omitempty"`
}

// ListVolumes returns every known volume. Wails generates both its TS signature
// and the domain.Volume model from this method (C13).
func (s *VolumeService) ListVolumes() ([]*domain.Volume, error) {
	volumes, err := s.volumes.List(seamContext())
	if err != nil {
		log.Error("seam: ListVolumes failed", "err", err)
		return nil, normalizeError(err)
	}
	log.Debug("seam: listed volumes", "count", len(volumes))
	return volumes, nil
}

// UpdateVolume applies the patch to an existing volume (read-modify-write).
func (s *VolumeService) UpdateVolume(id string, patch VolumePatch) error {
	volume, err := s.volumes.Get(seamContext(), id)
	if err != nil {
		return normalizeError(err)
	}
	if patch.Name != nil {
		volume.Name = *patch.Name
	}
	if err := s.volumes.Update(seamContext(), volume); err != nil {
		log.Error("seam: UpdateVolume failed", "id", id, "err", err)
		return normalizeError(err)
	}
	log.Info("seam: updated volume", "id", id)
	return nil
}
