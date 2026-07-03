package importer

import (
	"fmt"
	"io"
	"io/fs"

	"github.com/cespare/xxhash/v2"
)

// partialHashBytes is how many leading bytes form the fingerprint. Reading a
// whole 2GB video at ingest over a NAS is prohibitive; the first 64KB plus the
// size distinguishes files in a creative library well enough for change
// detection and dedup. See docs/original prd/05-ingest-pipeline.md.
const partialHashBytes = 64 * 1024

// partialHash is the pure fingerprint: xxhash of head concatenated with the file
// size. Pure — testable with a byte literal, no filesystem. The size is appended
// so a small file can't collide with a prefix of a larger one.
func partialHash(head []byte, size int64) string {
	h := xxhash.New()
	h.Write(head)
	fmt.Fprintf(h, "%d", size)
	return fmt.Sprintf("%x", h.Sum64())
}

// hashFile opens the file on fsys, reads up to the first 64KB, and returns the
// partial hash. Orchestration owns the open; partialHash stays pure.
func hashFile(fsys fs.FS, sf scannedFile) (string, error) {
	f, err := fsys.Open(sf.relPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	head := make([]byte, partialHashBytes)
	n, err := io.ReadFull(f, head)
	// Short files return EOF/ErrUnexpectedEOF with n valid — that's fine.
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	return partialHash(head[:n], sf.size), nil
}
