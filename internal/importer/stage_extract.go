package importer

import (
	"bytes"
	"context"
	"io"
	"io/fs"

	"github.com/akmadian/alexandria/internal/metadata"
	"github.com/charmbracelet/log"
)

// EXTRACT pulls normalized metadata from new/reimported files (a move keeps its
// existing metadata; sidecars and rejects skip it). It is best-effort: a decode
// failure records a DLQ row but never blocks the file from indexing (D13).

func (pipe *pipeline) extract(ctx context.Context, in <-chan *pipelineItem, out chan<- *pipelineItem) error {
	for item := range in {
		if !item.isSidecar && !item.rejected && (item.verdict == actionNew || item.verdict == actionReimport) {
			extractedMetadata, err := pipe.importer.extractMetadata(pipe.fsys, item.scanned, item.logger)
			item.extractedMetadata = extractedMetadata
			if err != nil {
				item.addError("extract", "decode_failed", err.Error())
			}
		}
		if err := pipe.emit(ctx, out, item); err != nil {
			return err
		}
	}
	return nil
}

// metadataFor extracts metadata only when it's needed — new files and reimports.
// A move keeps its existing metadata (content is unchanged); a skip never reaches
// here. This is the single-file (watcher) path; the EXTRACT stage above inlines
// the same guard.
func (imp *Importer) metadataFor(fsys fs.FS, scanned scannedFile, verdict action, logger *log.Logger) metadata.Metadata {
	if verdict != actionNew && verdict != actionReimport {
		return metadata.Metadata{}
	}
	extractedMetadata, _ := imp.extractMetadata(fsys, scanned, logger)
	return extractedMetadata
}

// extractMetadata runs the file's extractor (from its TypeHandler), best-effort:
// any failure yields whatever partial metadata came back plus the error (the
// caller logs a DLQ row but still indexes the asset — a corrupt EXIF block must
// not stop the file being indexed). A type with no extractor
// (handler.Metadata == nil) yields empty metadata and a nil error.
func (imp *Importer) extractMetadata(fsys fs.FS, scanned scannedFile, logger *log.Logger) (metadata.Metadata, error) {
	if scanned.handler.Metadata == nil {
		return metadata.Metadata{}, nil
	}
	reader, closeReader, err := openSeeker(fsys, scanned.relPath)
	if err != nil {
		logger.Warn("metadata: open failed", "path", scanned.relPath, "err", err)
		return metadata.Metadata{}, err
	}
	defer closeReader()

	extractedMetadata, err := scanned.handler.Metadata(reader)
	if err != nil {
		logger.Warn("metadata extraction failed", "path", scanned.relPath, "err", err)
	}
	width, height := 0, 0
	if extractedMetadata.Width != nil {
		width = *extractedMetadata.Width
	}
	if extractedMetadata.Height != nil {
		height = *extractedMetadata.Height
	}
	logger.Debug("metadata extracted", "path", scanned.relPath,
		"width", width, "height", height, "camera", extractedMetadata.CameraMake != nil, "gps", extractedMetadata.GPSLat != nil)
	return extractedMetadata, err
}

// openSeeker returns a seekable reader for a file. Mounted filesystems (os.DirFS)
// hand back a seekable *os.File; for any FS that doesn't, buffer into memory so
// extractors that seek (EXIF) still work. Shared by EXTRACT and THUMB.
func openSeeker(fsys fs.FS, name string) (io.ReadSeeker, func(), error) {
	file, err := fsys.Open(name)
	if err != nil {
		return nil, nil, err
	}
	if seeker, ok := file.(io.ReadSeeker); ok {
		return seeker, func() { file.Close() }, nil
	}
	buffered, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		return nil, nil, err
	}
	return bytes.NewReader(buffered), func() {}, nil
}
