package domain_test

import (
	"strings"
	"testing"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/google/uuid"
)

func TestNewID_IsUUIDv7(t *testing.T) {
	id := domain.NewID()
	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("NewID produced a non-UUID %q: %v", id, err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("NewID must mint UUIDv7 (repo convention), got v%d", parsed.Version())
	}
	if domain.NewID() == id {
		t.Fatal("two NewID calls returned the same id")
	}
}

func TestOpt_ThreeStates(t *testing.T) {
	var untouched domain.Opt[int]
	if untouched.Set || untouched.Value != nil {
		t.Fatal("zero Opt must mean don't-touch")
	}
	set := domain.SetOpt(3)
	if !set.Set || set.Value == nil || *set.Value != 3 {
		t.Fatalf("SetOpt: got %+v", set)
	}
	cleared := domain.ClearOpt[int]()
	if !cleared.Set || cleared.Value != nil {
		t.Fatalf("ClearOpt: got %+v", cleared)
	}
}

// PathKey: NFC-normalized comparison keys. "é" as one code point (NFC) and as
// 'e'+combining accent (NFD, what macOS emits) must produce the same key —
// that mismatch is the phantom-identity minter D24 names. The mapping is
// one-way and must NOT case-fold (distinct-case names are distinct files on
// case-sensitive filesystems).
func TestPathKey(t *testing.T) {
	nfc := "café/IMG_0001.jpg"       // é precomposed
	nfd := "cafe\u0301/IMG_0001.jpg" // e + combining acute
	if nfc == nfd {
		t.Fatal("test premise broken: the two forms must differ byte-wise")
	}
	if domain.PathKey(nfc) != domain.PathKey(nfd) {
		t.Fatal("PathKey must equate NFC and NFD forms of the same name")
	}
	if domain.PathKey("A.jpg") == domain.PathKey("a.jpg") {
		t.Fatal("PathKey must not case-fold")
	}
	if domain.PathKey("plain/ascii.jpg") != "plain/ascii.jpg" {
		t.Fatal("ASCII paths must pass through unchanged")
	}
}

func TestErrorMessages(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&domain.NotFoundError{Resource: "asset", ID: "a1"}, "a1"},
		{&domain.ConflictError{Resource: "source", Field: "path", Message: "taken"}, "taken"},
		{&domain.CatalogLockedError{Path: "/cat"}, "/cat"},
		{&domain.ValidationError{Field: "rating", Message: "bad"}, "rating"},
		{&domain.ErrSchemaTooOld{Current: 1, Required: 2}, "2"},
		{&domain.ErrSchemaTooNew{Current: 3, Known: 2}, "3"},
		{&domain.SourceOfflineError{SourceID: "s1", Path: "/vol"}, "s1"},
	}
	for _, tc := range cases {
		if message := tc.err.Error(); !strings.Contains(message, tc.want) {
			t.Errorf("%T message %q missing %q", tc.err, message, tc.want)
		}
	}
}
