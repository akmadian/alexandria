package importer

import (
	"bytes"
	"io"
	"io/fs"

	"github.com/akmadian/alexandria/internal/metadata"
)

// metadataFor extracts metadata only when it's needed — new files and reimports.
// A move keeps its existing metadata (content is unchanged); a skip never reaches
// here.
func (imp *Importer) metadataFor(fsys fs.FS, sf scannedFile, act action) metadata.Metadata {
	if act != actionNew && act != actionReimport {
		return metadata.Metadata{}
	}
	return imp.extractMetadata(fsys, sf)
}

// extractMetadata runs the metadata extractor for a file, best-effort: any
// failure logs a warning and yields empty metadata rather than failing ingest
// (a corrupt EXIF block must not stop the file being indexed).
func (imp *Importer) extractMetadata(fsys fs.FS, sf scannedFile) metadata.Metadata {
	if imp.Metadata == nil {
		return metadata.Metadata{}
	}
	rs, closeFn, err := openSeeker(fsys, sf.relPath)
	if err != nil {
		imp.Log.Warn("metadata: open failed", "path", sf.relPath, "err", err)
		return metadata.Metadata{}
	}
	defer closeFn()

	md, err := imp.Metadata.Extract(rs, sf.mime)
	if err != nil {
		imp.Log.Warn("metadata: extraction failed", "path", sf.relPath, "err", err)
	}
	return md
}

// openSeeker returns a seekable reader for a file. Mounted filesystems (os.DirFS)
// hand back a seekable *os.File; for any FS that doesn't, buffer into memory so
// extractors that seek (EXIF) still work.
func openSeeker(fsys fs.FS, name string) (io.ReadSeeker, func(), error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, nil, err
	}
	if rs, ok := f.(io.ReadSeeker); ok {
		return rs, func() { f.Close() }, nil
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, nil, err
	}
	return bytes.NewReader(data), func() {}, nil
}
