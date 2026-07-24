//go:build !darwin && !linux

package volume

import (
	"context"
	"fmt"
	"path/filepath"
)

// SystemProber on platforms without a real identity probe (Windows, the BSDs)
// falls back to the path's volume name (the drive letter / UNC root on Windows)
// as both the mount point and the identity key. A real Windows probe lands with
// the Windows QA pass (DEFERRED §3).
type SystemProber struct{}

// NewSystemProber returns the OS-native prober.
func NewSystemProber() *SystemProber { return &SystemProber{} }

func (SystemProber) Probe(_ context.Context, absolutePath string) (Probe, error) {
	absolute, err := filepath.Abs(absolutePath)
	if err != nil {
		return Probe{}, fmt.Errorf("resolve %q: %w", absolutePath, err)
	}
	root := filepath.VolumeName(absolute) + string(filepath.Separator)
	return Probe{
		MountPoint:     root,
		FilesystemUUID: "vol:" + root,
	}, nil
}
