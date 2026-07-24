//go:build linux

package volume

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akmadian/alexandria/internal/domain"
)

// SystemProber is the Linux prober — stdlib file reads only, no subprocess:
//
//   - /proc/self/mountinfo maps the path to its mount point, filesystem type,
//     and mount source (parsed by the pure parseMountInfo).
//   - cifs/smb/nfs mounts: identity is host+share parsed from the mount source
//     (network filesystems have no filesystem UUID — D24's identity for them).
//   - block-device mounts: the /dev/disk/by-uuid symlink farm maps the device
//     to its REAL filesystem UUID; /dev/disk/by-label supplies the label.
//   - anything yielding no identity: the dev:N residual fallback
//     (prober_devid.go).
type SystemProber struct{}

// NewSystemProber returns the OS-native prober.
func NewSystemProber() *SystemProber { return &SystemProber{} }

const (
	mountInfoPath  = "/proc/self/mountinfo"
	diskByUUIDDir  = "/dev/disk/by-uuid"
	diskByLabelDir = "/dev/disk/by-label"
)

func (SystemProber) Probe(_ context.Context, absolutePath string) (Probe, error) {
	absolute, err := filepath.Abs(absolutePath)
	if err != nil {
		return Probe{}, fmt.Errorf("resolve %q: %w", absolutePath, err)
	}
	content, err := os.ReadFile(mountInfoPath)
	if err != nil {
		return Probe{}, fmt.Errorf("read %s: %w", mountInfoPath, err)
	}
	entries, err := parseMountInfo(content)
	if err != nil {
		return Probe{}, err
	}
	entry, ok := bestMountFor(entries, absolute)
	if !ok {
		return Probe{}, fmt.Errorf("no mountinfo entry contains %q", absolute)
	}

	switch {
	case strings.HasPrefix(entry.FilesystemType, "cifs"), strings.HasPrefix(entry.FilesystemType, "smb"):
		return networkProbe(entry.MountPoint, domain.VolumeKindSMB, parseSMBSource, entry.Source)
	case strings.HasPrefix(entry.FilesystemType, "nfs"):
		return networkProbe(entry.MountPoint, domain.VolumeKindNFS, parseNFSSource, entry.Source)
	}

	if uuid, ok := deviceSymlinkName(diskByUUIDDir, entry.Source); ok {
		probe := Probe{
			MountPoint:     entry.MountPoint,
			FilesystemUUID: uuid,
			// Local vs external is not derivable from mountinfo alone on Linux
			// (it needs the sysfs removable flag per parent disk); default local,
			// user-correctable via the [jdg] kind. DiskSerial likewise stays nil.
			Kind: domain.VolumeKindLocal,
		}
		if label, ok := deviceSymlinkName(diskByLabelDir, entry.Source); ok {
			probe.VolumeLabel = &label
		}
		return probe, nil
	}
	// No UUID obtainable (virtual/exotic filesystem) → residual dev:N fallback.
	return fallbackDeviceProbe(entry.MountPoint)
}

// deviceSymlinkName scans a /dev/disk/by-* directory for the symlink that
// resolves to devicePath, returning the symlink's name (the UUID/label). This is
// how udev publishes filesystem identity — a directory read, no subprocess.
func deviceSymlinkName(directory, devicePath string) (string, bool) {
	resolvedDevice, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return "", false
	}
	dirEntries, err := os.ReadDir(directory)
	if err != nil {
		return "", false
	}
	for _, dirEntry := range dirEntries {
		resolved, err := filepath.EvalSymlinks(filepath.Join(directory, dirEntry.Name()))
		if err != nil {
			continue
		}
		if resolved == resolvedDevice {
			return unescapeUdevName(dirEntry.Name()), true
		}
	}
	return "", false
}
