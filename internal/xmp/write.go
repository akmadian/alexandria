package xmp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/akmadian/alexandria/internal/dependency"
	"github.com/akmadian/alexandria/internal/domain"
)

// Write merges our fields into an existing sidecar (or creates one) via
// exiftool, then does an atomic temp→rename so the watcher sees a single
// event. Only the fields Alexandria owns are touched; foreign namespaces
// (crs: develop settings, custom fields) are preserved by exiftool's
// merge-into-existing behavior.
//
// The caller (writeOutbound) owns the DB cursor update and echo-check hash
// storage; this function is pure I/O.
func Write(ctx context.Context, daemon *dependency.ExiftoolDaemon, sidecarPath string, fields WriteFields) error {
	tempPath := sidecarPath + ".alxtmp"
	args := buildWriteArgs(fields, sidecarPath, tempPath)
	output, err := daemon.Execute(ctx, args...)
	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("xmp: write %s: %w", sidecarPath, err)
	}
	// exiftool prints "1 image files updated" on success; anything else is a
	// problem. Check for the success marker rather than parsing error text.
	if !strings.Contains(string(output), "1 image files updated") {
		os.Remove(tempPath)
		return fmt.Errorf("xmp: write %s: unexpected output: %s", sidecarPath, string(output))
	}

	if err := os.Rename(tempPath, sidecarPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("xmp: atomic rename %s: %w", sidecarPath, err)
	}
	return nil
}

// WriteFields carries the catalog values to write outbound. nil/empty means
// "clear the field in the sidecar" (wholesale, matching LrC "Save Metadata").
type WriteFields struct {
	Rating       *int
	ColorLabel   *domain.ColorLabel
	Tags         []string // dc:subject (flat keywords)
	Hierarchical []string // lr:hierarchicalSubject ("Travel|Japan|Tokyo")
	Caption      string
	Title        string
}

// buildWriteArgs constructs the exiftool argument list for a merge write.
// exiftool writes to a temp file (-o tempPath) from the source sidecar, or
// creates a new sidecar if the source doesn't exist.
func buildWriteArgs(fields WriteFields, sidecarPath, tempPath string) []string {
	var args []string

	if fields.Rating != nil {
		args = append(args, "-Rating="+strconv.Itoa(*fields.Rating))
	} else {
		args = append(args, "-Rating=")
	}

	if fields.ColorLabel != nil {
		args = append(args, "-Label="+labelToXMP(*fields.ColorLabel))
	} else {
		args = append(args, "-Label=")
	}

	args = appendListTag(args, "Subject", fields.Tags)
	args = appendListTag(args, "HierarchicalSubject", fields.Hierarchical)

	if fields.Caption != "" {
		args = append(args, "-Description="+fields.Caption)
	} else {
		args = append(args, "-Description=")
	}
	if fields.Title != "" {
		args = append(args, "-Title="+fields.Title)
	} else {
		args = append(args, "-Title=")
	}

	args = append(args, "-overwrite_original")

	// Merge into existing sidecar (preserves foreign namespaces like crs: develop
	// settings) or create a new one. Either way we work on a temp copy and atomic-
	// rename over the target.
	if _, err := os.Stat(sidecarPath); err == nil {
		args = append(args, "-o", tempPath, sidecarPath)
	} else {
		// No existing sidecar: create a minimal XMP file at the temp path. exiftool
		// needs a target file to write tags into, so we touch it first.
		dir := filepath.Dir(sidecarPath)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(tempPath, nil, 0o644)
		args = append(args, tempPath)
	}

	return args
}

// appendListTag builds the exiftool args for a multi-value tag: first value
// replaces (=), subsequent append (+=), empty clears.
func appendListTag(args []string, tag string, values []string) []string {
	if len(values) == 0 {
		return append(args, "-"+tag+"=")
	}
	args = append(args, "-"+tag+"="+values[0])
	for _, value := range values[1:] {
		args = append(args, "-"+tag+"+="+value)
	}
	return args
}

var labelToXMPMap = map[domain.ColorLabel]string{
	domain.ColorLabelRed: "Red", domain.ColorLabelOrange: "Orange",
	domain.ColorLabelYellow: "Yellow", domain.ColorLabelGreen: "Green",
	domain.ColorLabelBlue: "Blue", domain.ColorLabelPurple: "Purple",
}

func labelToXMP(label domain.ColorLabel) string {
	if english, ok := labelToXMPMap[label]; ok {
		return english
	}
	return string(label)
}
