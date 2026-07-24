//go:build darwin

package volume

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/akmadian/alexandria/internal/domain"
)

// TestSystemProber_RealFilesystemUUID is the darwin-host integration proof: a
// probe of a real path yields a REAL filesystem UUID (not the dev:N residual
// form) that is stable across two calls, plus a sensible kind and label.
func TestSystemProber_RealFilesystemUUID(t *testing.T) {
	if _, err := exec.LookPath("diskutil"); err != nil {
		t.Skip("diskutil not on PATH — not a standard macOS host")
	}
	prober := NewSystemProber()
	ctx := context.Background()

	first, err := prober.Probe(ctx, os.TempDir())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	second, err := prober.Probe(ctx, os.TempDir())
	if err != nil {
		t.Fatalf("second probe: %v", err)
	}

	if first.FilesystemUUID == "" {
		t.Fatal("probe yielded no identity")
	}
	if strings.HasPrefix(first.FilesystemUUID, "dev:") {
		t.Fatalf("probe fell back to the residual dev:N form (%q) on a standard APFS volume — the real UUID path is broken", first.FilesystemUUID)
	}
	if first.FilesystemUUID != second.FilesystemUUID {
		t.Fatalf("identity unstable across calls: %q then %q", first.FilesystemUUID, second.FilesystemUUID)
	}
	// A diskutil VolumeUUID is the canonical 8-4-4-4-12 form.
	if len(first.FilesystemUUID) != 36 || strings.Count(first.FilesystemUUID, "-") != 4 {
		t.Fatalf("identity %q does not look like a filesystem UUID", first.FilesystemUUID)
	}
	// The mount point must be a literal ancestor of the probed path (that is
	// what the resolver rebases against) — on macOS this is the firmlink-side
	// root (e.g. /var), not statfs's canonical /System/Volumes/Data.
	if first.MountPoint == "" || !pathHasPrefix(strings.TrimSuffix(os.TempDir(), "/"), first.MountPoint) {
		t.Fatalf("mount point %q is not a literal ancestor of %q", first.MountPoint, os.TempDir())
	}
	if first.Kind != domain.VolumeKindLocal && first.Kind != domain.VolumeKindExternalDrive {
		t.Fatalf("kind = %q, want a classified block-device kind", first.Kind)
	}
	if first.VolumeLabel == nil || *first.VolumeLabel == "" {
		t.Fatal("expected a volume label from diskutil")
	}
}
