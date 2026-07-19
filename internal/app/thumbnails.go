package app

import (
	"net/http"
	"strings"

	"github.com/charmbracelet/log"
)

// thumbnailPrefix is the URL namespace of the binary channel; paths under it
// mirror the on-disk layout the thumbnailer owns
// (/thumbnails/<size>/<shard>/<id>.jpg).
const thumbnailPrefix = "/thumbnails/"

// ThumbnailMiddleware serves the catalog's thumbnail tree to the webview — the
// binary channel of the seam contract: bytes never cross the seam, rows carry a
// URL and the webview fetches it here. It mounts as asset-server MIDDLEWARE,
// not the not-found Handler: under wails dev the frontend dev server's SPA
// fallback answers 200 text/html for any GET, so thumbnail requests must
// short-circuit before the dev-server proxy ever sees them — middleware wraps
// that whole chain, identically in dev and prod. Requests outside the prefix
// pass through untouched; inside it, this is a bare file server (http.Dir
// rejects path traversal), and a missing thumbnail is a plain 404. Cache-busting
// content tokens arrive with in-place thumbnail regeneration (P2 — DEFERRED §7).
func ThumbnailMiddleware(directory string) func(http.Handler) http.Handler {
	fileServer := http.StripPrefix("/thumbnails", http.FileServer(http.Dir(directory)))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if !strings.HasPrefix(request.URL.Path, thumbnailPrefix) {
				next.ServeHTTP(writer, request)
				return
			}
			// Per-request Debug is the trace that proves the short-circuit
			// during wails dev.
			log.Debug("thumbnail request", "path", request.URL.Path)
			fileServer.ServeHTTP(writer, request)
		})
	}
}
