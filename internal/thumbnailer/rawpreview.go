package thumbnailer

import (
	"bytes"
	"context"
	"fmt"
)

// previewTags are the embedded-preview tags tried in order, best first.
// PreviewImage is the near-universal camera-written preview; JpgFromRaw is the
// full-size embedded JPEG some makers (Canon CR2, Fuji RAF) write instead of —
// or at better resolution than — PreviewImage; ThumbnailImage is the tiny EXIF
// IFD1 thumbnail, a last resort (a small real preview beats a failed state).
// Three daemon round-trips worst case, one in the common case; each is
// single-digit milliseconds against a warm daemon.
var previewTags = []string{"-PreviewImage", "-JpgFromRaw", "-ThumbnailImage"}

// generateRawPreview is the GenerateRawPreview strategy: ask the exiftool
// daemon for the RAW file's embedded JPEG preview and feed those bytes through
// the shared raster backend. The daemon needs the file PATH (its -stay_open
// protocol takes path arguments; bytes cannot be streamed in), which is why
// GenFunc carries a path, not a reader.
//
// -q -q silences exiftool's informational and warning chatter: the daemon
// transport merges stdout and stderr onto one pipe, so any text emitted around
// the binary payload would corrupt it. A missing tag then yields zero bytes,
// which is the "try the next tag" signal.
func (thumb *Thumbnailer) generateRawPreview(ctx context.Context, sourcePath string, assetID string) error {
	if thumb.Exiftool == nil {
		return ErrExiftoolUnavailable
	}
	var previewBytes []byte
	for _, tag := range previewTags {
		extracted, err := thumb.Exiftool.Execute(ctx, "-q", "-q", "-b", tag, sourcePath)
		if err != nil {
			return fmt.Errorf("extract preview (%s): %w", tag, err)
		}
		if len(extracted) > 0 {
			previewBytes = extracted
			break
		}
	}
	if len(previewBytes) == 0 {
		return fmt.Errorf("no embedded preview in %s (tried %v)", sourcePath, previewTags)
	}
	return thumb.resizeAndEncode(bytes.NewReader(previewBytes), assetID)
}
