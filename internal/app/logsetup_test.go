package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSymlinkForce_CreatesLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")

	if err := symlinkForce(target, link); err != nil {
		t.Fatalf("symlinkForce: %v", err)
	}
	if got, _ := os.Readlink(link); got != target {
		t.Fatalf("link -> %q, want %q", got, target)
	}
}

func TestSymlinkForce_Idempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")

	for range 2 {
		if err := symlinkForce(target, link); err != nil {
			t.Fatalf("symlinkForce: %v", err)
		}
	}
	if got, _ := os.Readlink(link); got != target {
		t.Fatalf("link -> %q, want %q", got, target)
	}
}

func TestSymlinkForce_ReplacesStaleLink(t *testing.T) {
	dir := t.TempDir()
	oldTarget := filepath.Join(dir, "old")
	newTarget := filepath.Join(dir, "new")
	link := filepath.Join(dir, "link")
	if err := os.Symlink(oldTarget, link); err != nil {
		t.Fatal(err)
	}

	if err := symlinkForce(newTarget, link); err != nil {
		t.Fatalf("symlinkForce: %v", err)
	}
	if got, _ := os.Readlink(link); got != newTarget {
		t.Fatalf("link -> %q, want %q", got, newTarget)
	}
}

func TestSymlinkForce_RefusesRealDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.Mkdir(link, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := symlinkForce(target, link); err == nil {
		t.Fatal("expected error when link path is a real dir, got nil")
	}
	if info, err := os.Lstat(link); err != nil || !info.IsDir() {
		t.Fatalf("real dir was clobbered: info=%v err=%v", info, err)
	}
}
