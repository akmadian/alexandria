package importer

import (
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

func TestPartialHash_StableAndSensitive(t *testing.T) {
	base := partialHash([]byte("hello"), 5)
	if partialHash([]byte("hello"), 5) != base {
		t.Fatal("hash must be stable for the same input")
	}
	if partialHash([]byte("hello"), 6) == base {
		t.Fatal("a different size must change the hash")
	}
	if partialHash([]byte("world"), 5) == base {
		t.Fatal("different content must change the hash")
	}
}

func TestClassify(t *testing.T) {
	mime, ft, ok := classify("jpg")
	if !ok || ft != domain.FileTypeImage || mime != "image/jpeg" {
		t.Fatalf("jpg: got %q %q %v", mime, ft, ok)
	}
	if _, ft, ok := classify("cr2"); !ok || ft != domain.FileTypeRaw {
		t.Fatalf("cr2 should classify as raw, got %q %v", ft, ok)
	}
	if _, _, ok := classify("exe"); ok {
		t.Fatal("exe should be unsupported")
	}
}

func TestUnchanged(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	known := map[string]domain.FileStat{
		"a.jpg": {MTime: base, SizeBytes: 100},
	}
	// Same size, mtime within the 2s tolerance → unchanged.
	if !unchanged(scannedFile{relPath: "a.jpg", size: 100, mtime: base.Add(time.Second)}, known) {
		t.Fatal("expected unchanged within mtime tolerance")
	}
	// Size differs → changed, even if mtime matches.
	if unchanged(scannedFile{relPath: "a.jpg", size: 101, mtime: base}, known) {
		t.Fatal("a size change must count as changed")
	}
	// Unknown path → not unchanged.
	if unchanged(scannedFile{relPath: "new.jpg", size: 100, mtime: base}, known) {
		t.Fatal("an unknown path is never unchanged")
	}
}
