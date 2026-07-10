package seam_test

import (
	"errors"
	"testing"

	"github.com/akmadian/alexandria/internal/seam"
)

// asApiError asserts err is a normalized *seam.ApiError and returns it — every
// bound method must return errors in this shape (impl/15 §4), never a raw engine
// error.
func asApiError(t *testing.T, err error) *seam.ApiError {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var apiErr *seam.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *seam.ApiError, got %T: %v", err, err)
	}
	return apiErr
}

// assertDomainCode asserts err is a domain-kind ApiError carrying the given code.
func assertDomainCode(t *testing.T, err error, code string) {
	t.Helper()
	apiErr := asApiError(t, err)
	if apiErr.Kind != seam.KindDomain {
		t.Fatalf("expected domain kind, got %q (%v)", apiErr.Kind, apiErr)
	}
	if string(apiErr.Code) != code {
		t.Fatalf("expected code %q, got %q", code, apiErr.Code)
	}
}

// assertUnexpected asserts err is the catch-all unexpected kind (no code, no raw
// string leaked).
func assertUnexpected(t *testing.T, err error) {
	t.Helper()
	apiErr := asApiError(t, err)
	if apiErr.Kind != seam.KindUnexpected {
		t.Fatalf("expected unexpected kind, got %q (%v)", apiErr.Kind, apiErr)
	}
	if apiErr.Code != "" {
		t.Fatalf("unexpected errors carry no code, got %q", apiErr.Code)
	}
}

// TestApiError_ErrorIsJSON checks the wire form is parseable JSON carrying the
// kind + code, so they survive Wails's error→string serialization.
func TestApiError_ErrorIsJSON(t *testing.T) {
	apiErr := &seam.ApiError{Kind: seam.KindDomain, Code: seam.CodeNotFound, Detail: "asset not found: x"}
	got := apiErr.Error()
	want := `{"kind":"domain","code":"not_found","detail":"asset not found: x"}`
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
