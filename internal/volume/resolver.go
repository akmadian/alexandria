package volume

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/charmbracelet/log"
)

// Resolver maps between absolute filesystem paths and the (volume, volume-
// relative path) model. It is the single owner of find-or-create: an absolute
// path probes to a filesystem identity, and the volume row is looked up by that
// identity or minted once — so two folders on one filesystem always resolve to
// ONE volume (D24).
//
// The reverse direction (volume → current mount point) is served from an
// in-session cache populated as paths are resolved: the mount point is never
// stored (a remount would stale it), so Absolute/MountPoint answer only for
// volumes seen this session. Cold-start reverse resolution (enumerating live
// mounts and matching stored identities without a prior Resolve) is deliberately
// not built — no consumer needs it yet; the identities are real (see Probe), so
// it is purely an enumeration layer when one does.
type Resolver struct {
	Volumes catalog.VolumeRepository
	Prober  Prober
	Log     *log.Logger

	mountMutex sync.RWMutex
	// mountByID caches volumeID → current mount point (this session), one entry
	// per volume, last write wins. ponytail: on macOS a firmlinked system volume
	// is reachable through several literal roots (/Users vs /System/Volumes/Data);
	// tracking folders under TWO different firmlink roots of one volume would make
	// this cache reconstruct wrong absolute paths for the earlier root (loud
	// failure: file-not-found in enrichment). Upgrade path: key the cache per
	// tracked root instead of per volume.
	mountByID map[string]string
}

// NewResolver builds a resolver over the volume repository and a prober.
func NewResolver(volumes catalog.VolumeRepository, prober Prober, logger *log.Logger) *Resolver {
	if logger == nil {
		logger = log.Default()
	}
	return &Resolver{Volumes: volumes, Prober: prober, Log: logger, mountByID: map[string]string{}}
}

// Resolved is an absolute path decomposed onto the catalog's keying model.
type Resolved struct {
	VolumeID     string
	RelativePath string // volume-relative (slash-separated); "" = the volume root
	MountPoint   string // the filesystem's current mount point
}

// Resolve maps an absolute path to its volume (found-or-created by filesystem
// identity) and the path relative to the volume's mount point. This is the seam
// folder-add, the importer, and watcher event mapping all key through.
func (r *Resolver) Resolve(ctx context.Context, absolutePath string) (Resolved, error) {
	absolute, err := filepath.Abs(absolutePath)
	if err != nil {
		return Resolved{}, fmt.Errorf("resolve %q: %w", absolutePath, err)
	}
	probe, err := r.Prober.Probe(ctx, absolute)
	if err != nil {
		return Resolved{}, fmt.Errorf("probe %q: %w", absolute, err)
	}
	relativePath, err := relativeTo(probe.MountPoint, absolute)
	if err != nil {
		return Resolved{}, err
	}

	volume, err := r.findOrCreate(ctx, &probe)
	if err != nil {
		return Resolved{}, err
	}
	r.rememberMount(volume.ID, probe.MountPoint)
	return Resolved{VolumeID: volume.ID, RelativePath: relativePath, MountPoint: probe.MountPoint}, nil
}

// findOrCreate returns the volume carrying the probed filesystem identity,
// minting it the first time that identity is seen.
func (r *Resolver) findOrCreate(ctx context.Context, probe *Probe) (*domain.Volume, error) {
	existing, err := r.Volumes.FindByFilesystemUUID(ctx, probe.FilesystemUUID)
	if err != nil {
		return nil, fmt.Errorf("lookup volume by identity: %w", err)
	}
	if existing != nil {
		r.Log.Debug("resolved to existing volume", "volume", existing.ID, "name", existing.Name, "mount", probe.MountPoint)
		return existing, nil
	}
	now := time.Now().UTC()
	uuid := probe.FilesystemUUID
	kind := probe.Kind
	if kind == "" {
		// The prober classifies kind from mount/fs facts (internal vs external,
		// smb, nfs); an unclassified probe (the residual fallback, the non-unix
		// stub) defaults to local — a [jdg] column the user can correct.
		kind = domain.VolumeKindLocal
	}
	volume := &domain.Volume{
		ID:             domain.NewID(),
		Name:           volumeName(probe),
		Kind:           kind,
		FilesystemUUID: &uuid,
		VolumeLabel:    probe.VolumeLabel,
		DiskSerial:     probe.DiskSerial,
		Host:           probe.Host,
		ShareName:      probe.ShareName,
		Connectivity:   domain.VolumeOnline,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := r.Volumes.Create(ctx, volume); err != nil {
		// Two concurrent first-touches of a new filesystem both pass the Find
		// above and both INSERT; the loser hits the unique filesystem_uuid index.
		// The row it wanted now exists, so re-run the lookup and return the
		// winner's row — find-or-create converges instead of erroring.
		winner, findErr := r.Volumes.FindByFilesystemUUID(ctx, probe.FilesystemUUID)
		if findErr == nil && winner != nil {
			r.Log.Debug("lost volume-create race — adopting the winner's row",
				"volume", winner.ID, "identity", probe.FilesystemUUID)
			return winner, nil
		}
		return nil, fmt.Errorf("create volume: %w", err)
	}
	r.Log.Info("created volume", "volume", volume.ID, "name", volume.Name, "identity", uuid, "mount", probe.MountPoint)
	return volume, nil
}

// MountPoint returns a volume's current mount point if it was resolved this
// session. A volume never resolved this session errors (cold-start reverse
// resolution is a mount-enumeration layer nobody consumes yet — package doc).
func (r *Resolver) MountPoint(_ context.Context, volumeID string) (string, error) {
	r.mountMutex.RLock()
	defer r.mountMutex.RUnlock()
	if mount, ok := r.mountByID[volumeID]; ok {
		return mount, nil
	}
	return "", fmt.Errorf("volume %s is not mounted this session (mount point unknown)", volumeID)
}

// Absolute reconstructs the absolute path of a (volume, volume-relative path)
// pair using the session mount cache. This is how enrichment producers and the
// walk drivers turn a stored key back into a file to open.
func (r *Resolver) Absolute(ctx context.Context, volumeID, relativePath string) (string, error) {
	mount, err := r.MountPoint(ctx, volumeID)
	if err != nil {
		return "", err
	}
	if relativePath == "" {
		return mount, nil
	}
	return filepath.Join(mount, filepath.FromSlash(relativePath)), nil
}

func (r *Resolver) rememberMount(volumeID, mountPoint string) {
	r.mountMutex.Lock()
	r.mountByID[volumeID] = mountPoint
	r.mountMutex.Unlock()
}

// relativeTo computes the slash-separated path of absolute relative to mount.
// "." (the mount point itself) normalizes to "" (the volume root). An escaping
// path (rel starts with "..") is an error — the prober's mount point must be an
// ancestor of the path.
func relativeTo(mount, absolute string) (string, error) {
	relative, err := filepath.Rel(mount, absolute)
	if err != nil {
		return "", fmt.Errorf("relativize %q under %q: %w", absolute, mount, err)
	}
	if relative == "." {
		return "", nil
	}
	if relative == ".." || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("path %q escapes its mount point %q", absolute, mount)
	}
	return filepath.ToSlash(relative), nil
}

// volumeName picks a human default name for a freshly minted volume: its label,
// else the mount point's basename, else a generic fallback.
func volumeName(probe *Probe) string {
	if probe.VolumeLabel != nil && *probe.VolumeLabel != "" {
		return *probe.VolumeLabel
	}
	base := filepath.Base(probe.MountPoint)
	if base == "" || base == "." || base == string(filepath.Separator) || base == "/" {
		return "Local Disk"
	}
	return base
}
