// Package xmp reads and writes the metadata Alexandria shares with other DAMs
// (Lightroom Classic above all) through XMP. It reads sidecar `.xmp` files and
// embedded XMP; it writes sidecars only, never asset files (the reference model —
// D15, impl/06). exiftool owns the RDF/XML: there are several legal serializations
// and every writer's dialect differs, so this package never hand-parses XMP — it
// drives the exiftool daemon (internal/dependency) and maps its normalized -json
// output onto catalog concepts.
//
// This file is the inbound read path and the pure field mapping. Conflict
// resolution is in conflict.go; outbound writes in write.go; per-asset debounce
// in debounce.go. The DB application spans judgment (ApplyXMPInbound), keywords
// (TagRepository.ImportKeywords), and the sync-state cursors (RecordXMPWritten).
// Caption/title inbound is still pending (needs a sparse observation writer).
package xmp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/akmadian/alexandria/internal/dependency"
	"github.com/akmadian/alexandria/internal/domain"
)

// Fields is the normalized result of reading one XMP source — a sidecar or an
// asset's embedded packet. Absent values stay zero: a nil Rating means "the file
// carried no rating", distinct from a zero rating. Label is the raw XMP string
// (locale-dependent, e.g. "Rot") — NormalizeLabel turns it into a canonical
// ColorLabel; the raw string is preserved so an unknown label round-trips.
type Fields struct {
	Rating       *int
	Label        string   // raw xmp:Label, pre-normalization
	Tags         []string // dc:subject, flat keywords
	Hierarchical []string // lr:hierarchicalSubject, each "Travel|Japan|Tokyo"
	Caption      string   // dc:description
	Title        string   // dc:title
}

// readTags are the tags we ask exiftool for. Requesting an explicit set (rather
// than -XMP:all) keeps foreign namespaces — crs: develop settings, custom fields —
// out of the read; writes preserve them by merging, not by round-tripping here.
var readTags = []string{
	"-Rating",
	"-Label",
	"-Subject",
	"-HierarchicalSubject",
	"-Description",
	"-Title",
}

// Read parses one XMP source (sidecar path or asset with embedded XMP) via the
// exiftool daemon. A file with no XMP is not an error — it returns a zero Fields.
func Read(ctx context.Context, daemon *dependency.ExiftoolDaemon, path string) (Fields, error) {
	args := append([]string{"-json"}, readTags...)
	args = append(args, path)

	out, err := daemon.Execute(ctx, args...)
	if err != nil {
		return Fields{}, fmt.Errorf("xmp: read %s: %w", path, err)
	}
	return decodeExiftoolJSON(out)
}

// decodeExiftoolJSON coerces exiftool's -json output (an array of one record) into
// Fields. exiftool is loosely typed — a single-value list tag comes back as a bare
// string, a multi-value one as an array; a rating may be a number or a numeric
// string — so every field goes through a coercion helper rather than a rigid
// struct tag.
func decodeExiftoolJSON(raw []byte) (Fields, error) {
	// exiftool prints warnings (e.g. minor XMP issues) inline before the JSON on
	// the merged stream; trim to the first '[' so those don't break the decode.
	if start := bytes.IndexByte(raw, '['); start > 0 {
		raw = raw[start:]
	}
	var records []map[string]any
	if err := json.Unmarshal(raw, &records); err != nil {
		return Fields{}, fmt.Errorf("xmp: decode exiftool json: %w", err)
	}
	if len(records) == 0 {
		return Fields{}, nil
	}
	record := records[0]
	return Fields{
		Rating:       coerceIntPtr(record["Rating"]),
		Label:        coerceString(record["Label"]),
		Tags:         coerceStringList(record["Subject"]),
		Hierarchical: coerceStringList(record["HierarchicalSubject"]),
		Caption:      coerceString(record["Description"]),
		Title:        coerceString(record["Title"]),
	}, nil
}

func coerceString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func coerceIntPtr(value any) *int {
	switch typed := value.(type) {
	case float64:
		rounded := int(typed)
		return &rounded
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return &parsed
		}
	}
	return nil
}

// coerceStringList handles exiftool's string-or-array duality for list tags and
// drops empties so an absent tag never becomes a "" keyword.
func coerceStringList(value any) []string {
	var items []string
	switch typed := value.(type) {
	case string:
		items = []string{typed}
	case []any:
		for _, element := range typed {
			if text, ok := element.(string); ok {
				items = append(items, text)
			}
		}
	default:
		return nil
	}
	cleaned := items[:0]
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

// labelVocabulary maps known localized xmp:Label strings onto the canonical six.
// LrC writes the label in the OS display language ("Rot" on a German system), so a
// bare string match against English names loses every non-English catalog. The
// minimum set per impl/06: EN/DE/FR/ES/IT/JA. Matching is case-insensitive; an
// unknown string is preserved by the caller and left unmapped (never guessed).
var labelVocabulary = map[string]domain.ColorLabel{
	// English
	"red": domain.ColorLabelRed, "orange": domain.ColorLabelOrange,
	"yellow": domain.ColorLabelYellow, "green": domain.ColorLabelGreen,
	"blue": domain.ColorLabelBlue, "purple": domain.ColorLabelPurple,
	// German
	"rot": domain.ColorLabelRed, "gelb": domain.ColorLabelYellow,
	"grün": domain.ColorLabelGreen, "grun": domain.ColorLabelGreen,
	"blau": domain.ColorLabelBlue, "lila": domain.ColorLabelPurple,
	"violett": domain.ColorLabelPurple,
	// French
	"rouge": domain.ColorLabelRed, "jaune": domain.ColorLabelYellow,
	"vert": domain.ColorLabelGreen, "bleu": domain.ColorLabelBlue,
	"violet": domain.ColorLabelPurple,
	// Spanish
	"rojo": domain.ColorLabelRed, "naranja": domain.ColorLabelOrange,
	"amarillo": domain.ColorLabelYellow, "verde": domain.ColorLabelGreen,
	"azul": domain.ColorLabelBlue, "morado": domain.ColorLabelPurple,
	// Italian (verde/green shared with Spanish above)
	"rosso": domain.ColorLabelRed, "arancione": domain.ColorLabelOrange,
	"giallo": domain.ColorLabelYellow,
	"blu":    domain.ColorLabelBlue, "viola": domain.ColorLabelPurple,
	// Japanese
	"赤": domain.ColorLabelRed, "オレンジ": domain.ColorLabelOrange,
	"黄": domain.ColorLabelYellow, "緑": domain.ColorLabelGreen,
	"青": domain.ColorLabelBlue, "紫": domain.ColorLabelPurple,
}

// NormalizeLabel maps a raw xmp:Label string to a canonical ColorLabel. ok is
// false for an empty or unrecognized string — the caller leaves color_label unset
// and preserves the raw string for round-trip (impl/06 field map: unknown labels
// are never dropped, never guessed).
func NormalizeLabel(raw string) (label domain.ColorLabel, ok bool) {
	label, ok = labelVocabulary[strings.ToLower(strings.TrimSpace(raw))]
	return label, ok
}
