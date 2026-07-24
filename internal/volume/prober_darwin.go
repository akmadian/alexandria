//go:build darwin

package volume

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// SystemProber is the macOS prober. statfs(2) — pure Go, no cgo — yields the
// mount point, filesystem type, and mount source in one call; from there:
//
//   - smbfs/nfs mounts: identity is host+share parsed from the mount source
//     (network filesystems have no filesystem UUID — D24's identity for them).
//   - block-device mounts: `diskutil info -plist <mountpoint>` (an OS-shipped
//     binary, run as a subprocess — never cgo; DiskArbitration is off-limits)
//     yields the REAL VolumeUUID, the label, and internal/external
//     classification. The plist parsing is the pure parseDiskutilInfo.
//   - anything yielding no identity: the dev:N residual fallback
//     (prober_devid.go).
//
// diskutil is deliberately NOT routed through internal/dependency: that package
// exists for third-party tools (discovery across candidate paths, version
// floors, user-consented downloads). diskutil ships with macOS at a fixed path
// on every supported version — a plain exec with a timeout is the honest shape.
type SystemProber struct{}

// NewSystemProber returns the OS-native prober.
func NewSystemProber() *SystemProber { return &SystemProber{} }

// diskutilTimeout bounds the info subprocess; diskutil answers in tens of
// milliseconds, so a stuck disk daemon fails the probe rather than hanging it.
const diskutilTimeout = 10 * time.Second

func (SystemProber) Probe(ctx context.Context, absolutePath string) (Probe, error) {
	absolute, err := filepath.Abs(absolutePath)
	if err != nil {
		return Probe{}, fmt.Errorf("resolve %q: %w", absolutePath, err)
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(absolute, &stat); err != nil {
		return Probe{}, fmt.Errorf("statfs %q: %w", absolute, err)
	}
	// Two mount points on purpose: the canonical one (statfs) is what diskutil
	// understands; the literal one (walk-up) is an ancestor of the INPUT path,
	// which is what the resolver Rel()s against — on macOS firmlinks make these
	// differ (/Users/… lives on the volume mounted at /System/Volumes/Data).
	canonicalMount := int8String(stat.Mntonname[:])
	filesystemType := int8String(stat.Fstypename[:])
	mountSource := int8String(stat.Mntfromname[:])
	mountPoint, err := literalMountRoot(absolute)
	if err != nil {
		return Probe{}, err
	}

	switch filesystemType {
	case "smbfs", "cifs":
		return networkProbe(mountPoint, domain.VolumeKindSMB, parseSMBSource, mountSource)
	case "nfs":
		return networkProbe(mountPoint, domain.VolumeKindNFS, parseNFSSource, mountSource)
	}

	if probe, ok := diskutilProbe(ctx, canonicalMount); ok {
		probe.MountPoint = mountPoint
		return probe, nil
	}
	// No UUID obtainable (virtual/exotic filesystem) → residual dev:N fallback.
	return fallbackDeviceProbe(mountPoint)
}

// diskutilProbe asks diskutil for the mount point's volume record and shapes it
// into a Probe. ok=false when diskutil fails or reports no VolumeUUID — the
// caller falls back rather than erroring, because an unprobeable filesystem is
// still usable storage.
func diskutilProbe(ctx context.Context, mountPoint string) (Probe, bool) {
	commandContext, cancel := context.WithTimeout(ctx, diskutilTimeout)
	defer cancel()
	output, err := exec.CommandContext(commandContext, "/usr/sbin/diskutil", "info", "-plist", mountPoint).Output()
	if err != nil {
		return Probe{}, false
	}
	info, err := parseDiskutilInfo(output)
	if err != nil || info.VolumeUUID == "" {
		return Probe{}, false
	}
	probe := Probe{
		MountPoint:     mountPoint,
		FilesystemUUID: info.VolumeUUID,
		Kind:           classifyDarwinKind(info),
	}
	if info.VolumeName != "" {
		label := info.VolumeName
		probe.VolumeLabel = &label
	}
	// DiskSerial stays nil: the hardware serial lives on the parent disk device,
	// not the volume record — an IORegistry walk this probe deliberately skips.
	return probe, true
}

// classifyDarwinKind maps diskutil's placement facts onto the volume kind:
// an internal, non-removable disk is local; anything ejectable/removable or
// external is an external drive.
func classifyDarwinKind(info diskutilInfo) domain.VolumeKind {
	if info.Internal && !info.RemovableMedia && !info.Ejectable {
		return domain.VolumeKindLocal
	}
	return domain.VolumeKindExternalDrive
}

// int8String converts a NUL-terminated [n]int8 C string field (darwin statfs)
// to a Go string.
func int8String(chars []int8) string {
	out := make([]byte, 0, len(chars))
	for _, c := range chars {
		if c == 0 {
			break
		}
		out = append(out, byte(c)) //nolint:gosec // reinterpreting C char bytes; values round-trip exactly
	}
	return string(out)
}
