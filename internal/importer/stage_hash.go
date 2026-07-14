package importer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/cespare/xxhash/v2"
)

// HASH reads the first 64KB, computes the partial fingerprint (xxhash of head +
// size), and runs the magic-byte Sniff on the SAME buffer (zero extra I/O, D7 —
// see mismatch.go for the reclassify policy). Sidecars hash here too, then bypass
// the matrix/extract/thumb stages. A read failure rejects the file with a DLQ row.

func (pipe *pipeline) hash(ctx context.Context, in <-chan *pipelineItem, out chan<- *pipelineItem) error {
	for item := range in {
		_, span := pipe.importer.Tracer.Start(item.ctx, "import.hash")
		hash, head, err := hashAndHead(pipe.fsys, &item.scanned)
		if err != nil {
			span.Fail(err)
			item.rejected = true
			item.addError("hash", "read_failed", err.Error())
		} else {
			item.hash = hash
			if !item.isSidecar {
				item.head = head
				applyMismatchPolicy(item)
				item.head = nil // sniff done; don't carry the buffer downstream
			}
		}
		span.End() // before emit, so the gap to the next stage span IS the queue time
		if err := pipe.emit(ctx, out, item); err != nil {
			return err
		}
	}
	return nil
}

// partialHashBytes is how many leading bytes form the fingerprint. Reading a
// whole 2GB video at ingest over a NAS is prohibitive; the first 64KB plus the
// size distinguishes files in a creative library well enough for change
// detection and dedup. See docs/original prd/05-ingest-pipeline.md.
const partialHashBytes = 64 * 1024

// partialHash is the pure fingerprint: xxhash of head concatenated with the file
// size. Pure — testable with a byte literal, no filesystem. The size is appended
// so a small file can't collide with a prefix of a larger one.
func partialHash(head []byte, size int64) string {
	hasher := xxhash.New()
	_, _ = hasher.Write(head)
	fmt.Fprintf(hasher, "%d", size)
	return fmt.Sprintf("%x", hasher.Sum64())
}

// hashFile opens the file on fsys, reads up to the first 64KB, and returns the
// partial hash. Orchestration owns the open; partialHash stays pure.
func hashFile(fsys fs.FS, scanned *scannedFile) (string, error) {
	hash, _, err := hashAndHead(fsys, scanned)
	return hash, err
}

// hashAndHead is hashFile plus the leading bytes it read, so the pipeline can
// run the magic-byte Sniff on the same buffer (zero extra I/O, D7). The returned
// head is the actual bytes read (≤64KB); callers must not retain it past the
// stage — it is per-file and sized for the sniff, not held for the whole run.
func hashAndHead(fsys fs.FS, scanned *scannedFile) (string, []byte, error) {
	file, err := fsys.Open(scanned.relPath)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	head := make([]byte, partialHashBytes)
	bytesRead, err := io.ReadFull(file, head)
	// Short files return EOF/ErrUnexpectedEOF with bytesRead valid — that's fine.
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", nil, err
	}
	head = head[:bytesRead]
	return partialHash(head, scanned.size), head, nil
}
