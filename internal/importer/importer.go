package importer

import (
	"fmt"
	"io"
	"os"

	"github.com/cespare/xxhash/v2"
	"github.com/h2non/filetype"
)

var (
	entryMap = make(map[string]AssetDetails)
)

type AssetDetails struct {
	BasePath string
	Name     string

	MTime       string
	Size        int64
	Type        filetype.Type
	Hash        uint64
	Fingerprint string
}

func Run(path string) {
	// Example usage of the logger
	fmt.Println("Running importer...")

	walkDir(path)

	fmt.Println("Importer finished")
}

func walkDir(path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		fmt.Println("Error reading directory:", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			walkDir(fmt.Sprintf("%s/%s", path, entry.Name()))
		} else {
			name := entry.Name()
			info, _ := entry.Info()
			mtime := info.ModTime()
			size := info.Size()

			kind, _ := filetype.MatchFile(fmt.Sprintf("%s/%s", path, name))
			if kind == filetype.Unknown {
				fmt.Printf("Unknown file type for %s/%s\n", path, name)
				continue
			}

			if entryMap[name] != (AssetDetails{}) {
				fmt.Printf("Duplicate file name found: %s. Skipping.\n", name)
				continue
			}

			hash := computeHash(fmt.Sprintf("%s/%s", path, name))

			entryMap[name] = AssetDetails{
				BasePath:    path,
				Name:        name,
				MTime:       mtime.String(),
				Size:        size,
				Type:        kind,
				Hash:        hash,
				Fingerprint: fmt.Sprintf("%x_%x", hash, size),
			}
		}
	}
}

func computeHash(filePath string) uint64 {
	file, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer file.Close()

	h := xxhash.New()
	_, err = io.Copy(h, file)
	if err != nil {
		return 0
	}

	return h.Sum64()
}
