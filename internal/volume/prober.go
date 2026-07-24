// Package volume is the identity/portability engine of the D24 split: it maps
// absolute filesystem paths to the (volume, volume-relative path) model the
// catalog keys on, finds-or-creates the volume row by filesystem identity, and
// owns the folder-add semantics (the disjoint-roots invariant + the four-outcome
// graceful-merge union, D41).
//
// The volume prober is the "the volume monitor's probe is the source" seam: it
// answers "which storage does this path live on, and where is it mounted right
// now?" so the mount point is resolved live and never stored (a drive can mount
// at a different path each session). It is injected as a strategy (never a
// capability conditional) so tests drive it with a fake and the per-OS concrete
// prober stays isolated behind a build tag.
package volume

import (
	"context"

	"github.com/akmadian/alexandria/internal/domain"
)

// Probe is what the prober learns about the storage under a path: where the
// filesystem is mounted right now, its identity, and its classification.
//
// FilesystemUUID is the find-or-create key. On macOS and Linux it is a REAL
// identity for the common cases: the filesystem UUID for local/external
// block-device volumes (diskutil on macOS, /dev/disk/by-uuid on Linux), or a
// deterministic "smb://host/share" / "nfs://host/export" synthetic for network
// mounts (which have no filesystem UUID — host+share IS their D24 identity).
// Only when neither is obtainable (exotic virtual filesystems) does it fall
// back to a session-scoped "dev:N" device-number synthetic — see the fallback's
// ceiling comment in prober_devid.go. On other platforms (Windows — deferred to
// the Windows milestone) it is a mount-root synthetic.
type Probe struct {
	MountPoint     string            // absolute current mount point of the filesystem the path is on
	FilesystemUUID string            // identity; the find-or-create key (see above)
	Kind           domain.VolumeKind // classified from mount/fs facts; "" = caller defaults to local
	VolumeLabel    *string           // best-effort display label, if the OS exposes one
	DiskSerial     *string           // best-effort; the v1 probers leave it nil (a hardware serial needs a per-OS IOKit/sysfs walk — not cheap)
	Host           *string           // network mounts only: the server host
	ShareName      *string           // network mounts only: the share/export
}

// Prober resolves an absolute path to its filesystem's mount point and identity.
// One implementation per OS lives behind a build tag; tests inject a fake.
type Prober interface {
	Probe(ctx context.Context, absolutePath string) (Probe, error)
}
