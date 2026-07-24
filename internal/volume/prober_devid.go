//go:build darwin || linux

package volume

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/akmadian/alexandria/internal/domain"
)

// networkProbe shapes a parsed SMB/NFS mount source into a Probe — shared by
// the darwin and linux probers. A source that fails to parse degrades to the
// dev:N fallback rather than erroring.
func networkProbe(mountPoint string, kind domain.VolumeKind, parse func(string) (networkShare, error), mountSource string) (Probe, error) {
	share, err := parse(mountSource)
	if err != nil {
		return fallbackDeviceProbe(mountPoint)
	}
	host, shareName := share.Host, share.Share
	label := share.Share
	return Probe{
		MountPoint:     mountPoint,
		FilesystemUUID: share.Identity,
		Kind:           kind,
		VolumeLabel:    &label,
		Host:           &host,
		ShareName:      &shareName,
	}, nil
}

// The dev:N device-number fallback — the LAST-RESORT identity when a filesystem
// yields neither a real UUID (local block devices) nor a host+share identity
// (network mounts): exotic virtual filesystems, overlay/union mounts, tmpfs-like
// trees a user points a folder at.
//
// ponytail: within this residual case the two known hazards of a device-number
// key remain, accepted: (1) DUPLICATE VOLUME ROW — st_dev is assigned at mount
// time, so a reboot/remount can hand the same filesystem a different number and
// the next resolve mints a second volume; (2) WRONG-VOLUME ATTRIBUTION — the OS
// can reuse a device number for a different filesystem across sessions, so a
// stale dev:N row can match storage it never described. The common cases no
// longer ride this path (real UUID / host+share probes above); shrinking the
// residue further means identifying virtual filesystems some other way, which
// has no known consumer.
func fallbackDeviceProbe(mountPoint string) (Probe, error) {
	device, err := deviceOf(mountPoint)
	if err != nil {
		return Probe{}, err
	}
	return Probe{
		MountPoint:     mountPoint,
		FilesystemUUID: fmt.Sprintf("dev:%d", device),
	}, nil
}

// literalMountRoot walks up from absolutePath until the device number changes —
// a mount boundary — returning the highest ancestor still on the path's
// filesystem, IN THE INPUT PATH'S NAMESPACE. This matters on macOS: statfs
// reports the canonical mount point (/System/Volumes/Data), which is NOT a
// literal ancestor of a firmlinked path like /Users/… or /var/… — and the
// resolver needs a mount root it can Rel() the input against. The walk-up stays
// the mount-point authority; the canonical statfs mount serves only identity
// lookups (diskutil).
func literalMountRoot(absolutePath string) (string, error) {
	device, err := deviceOf(absolutePath)
	if err != nil {
		return "", err
	}
	mountPoint := absolutePath
	for {
		parent := filepath.Dir(mountPoint)
		if parent == mountPoint {
			break // reached the filesystem root ("/")
		}
		parentDevice, err := deviceOf(parent)
		if err != nil || parentDevice != device {
			break // parent is on a different filesystem → mountPoint is the boundary
		}
		mountPoint = parent
	}
	return mountPoint, nil
}

// deviceOf returns the filesystem device number for a path (st_dev). The device
// number is identical for every path on one mounted filesystem and changes across
// a mount boundary.
func deviceOf(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat %q: %w", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("stat %q: no unix stat", path)
	}
	//nolint:gosec // st_dev is an opaque device id; width/sign reinterpretation is harmless — only equality matters
	return uint64(stat.Dev), nil
}
