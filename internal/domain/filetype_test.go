package domain_test

import (
	"testing"

	"github.com/akmadian/alexandria/internal/domain"
)

func TestClassify(t *testing.T) {
	// Case-insensitive and dot-tolerant.
	info, ok := domain.Classify("JPG")
	if !ok || info.Type != domain.FileTypeImage || info.MIME != "image/jpeg" {
		t.Fatalf("JPG: %+v ok=%v", info, ok)
	}
	info, ok = domain.Classify(".cr2")
	if !ok || info.Type != domain.FileTypeRaw {
		t.Fatalf("cr2: %+v ok=%v", info, ok)
	}
	if _, ok := domain.Classify("exe"); ok {
		t.Fatal("exe should be unsupported")
	}
}

func TestIsSupported(t *testing.T) {
	if !domain.IsSupported("png") {
		t.Fatal("png should be supported")
	}
	if domain.IsSupported("xyz") {
		t.Fatal("xyz should not be supported")
	}
}
