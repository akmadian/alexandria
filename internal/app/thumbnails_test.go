package app_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	app "github.com/akmadian/alexandria/internal/app"
)

// passthroughSentinel is what the wrapped next handler answers — asserting on it
// proves a request flowed past the middleware untouched.
const passthroughSentinel = http.StatusTeapot

// serveThumbnail runs one request through the middleware (wrapping a sentinel
// next handler) and returns the response.
func serveThumbnail(t *testing.T, directory, path string) *http.Response {
	t.Helper()
	next := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(passthroughSentinel)
	})
	recorder := httptest.NewRecorder()
	app.ThumbnailMiddleware(directory)(next).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Result()
}

func TestThumbnailMiddleware_ServesShardedJPEG(t *testing.T) {
	directory := t.TempDir()
	shardDir := filepath.Join(directory, "512", "ab")
	if err := os.MkdirAll(shardDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := []byte("jpeg-bytes")
	if err := os.WriteFile(filepath.Join(shardDir, "abcd1234.jpg"), want, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	response := serveThumbnail(t, directory, "/thumbnails/512/ab/abcd1234.jpg")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", got)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(body, want) {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestThumbnailMiddleware_MissingThumbnail404sWithoutPassthrough(t *testing.T) {
	response := serveThumbnail(t, t.TempDir(), "/thumbnails/512/ab/missing.jpg")
	defer response.Body.Close()
	// The prefix is owned here: a missing thumbnail is a plain 404, never the
	// next handler (whose dev-mode SPA fallback would answer 200 text/html).
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

func TestThumbnailMiddleware_OutsidePrefixPassesThrough(t *testing.T) {
	response := serveThumbnail(t, t.TempDir(), "/api/anything")
	defer response.Body.Close()
	if response.StatusCode != passthroughSentinel {
		t.Fatalf("status = %d, want passthrough sentinel %d", response.StatusCode, passthroughSentinel)
	}
}

func TestThumbnailMiddleware_TraversalNeverEscapes(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "thumbnails")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	secret := []byte("catalog-db-bytes")
	if err := os.WriteFile(filepath.Join(parent, "catalog.db"), secret, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	response := serveThumbnail(t, directory, "/thumbnails/../catalog.db")
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode == http.StatusOK || bytes.Equal(body, secret) {
		t.Fatalf("traversal escaped: status %d, body %q", response.StatusCode, body)
	}
}
