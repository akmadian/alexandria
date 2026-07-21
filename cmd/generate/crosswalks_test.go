package main

// The crosswalk completeness suite (C15): one registry of every hand-maintained
// relationship between parallel declarations, each pinned by a check. Adding a
// projection = adding a row here. Lives beside the schema compiler because the
// compiler and the checker are the same concern — the machine that keeps
// projections honest — and both sit at the bottom of the import graph.
//
// What is deliberately NOT here: crosswalks already enforced by a stronger
// mechanism (generation + `satisfies` for all TS, the exhaustive linter for
// enum switches), and the XMP property map, whose relationship to domain is
// semantic (spec-driven, direction-dependent) — restating it in a test would
// itself be the drift risk. That one is documented in docs/vocabulary.md.

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/enrichment"
	"github.com/akmadian/alexandria/internal/metadata"
	"github.com/akmadian/alexandria/internal/seam"
	"github.com/akmadian/alexandria/internal/testutil"
)

var crosswalks = []struct {
	name  string
	check func(t *testing.T)
}{
	{"ast fields name real domain.Asset fields", checkFieldsOnDomainAsset},
	{"every domain.Asset field is queryable or deliberately unexposed", checkAssetFieldsAccountedFor},
	{"every grammar combination executes against the real schema", checkGrammarExecutes},
	{"every sort field executes against the real schema", checkSortFieldsExecute},
	{"metadata.Metadata is a subset of domain.Asset", checkExtractionSubset},
	{"catalog.FilePatch is a subset of domain.Asset", checkFilePatchSubset},
	{"catalog triage shapes name domain.Asset judgment fields", checkTriageSubset},
	{"seam.AssetDetail is a subset of domain.Asset", checkAssetDetailSubset},
	{"seam.TriagePatchInput wire fields mirror catalog.TriagePatch", checkTriagePatchInputWire},
	{"domain.EnrichmentKind matches the engine registry kinds", checkEnrichmentKinds},
}

// checkEnrichmentKinds pins the published domain.EnrichmentKind union to the
// engine's registry kinds: every registry kind must have a const (so the seam's
// decoration never ships a kind the frontend union lacks) and every const must be
// a live registry kind (no stale member). Definitions(nil, nil) is safe — only the
// static Kind fields are read, the producers are never invoked.
func checkEnrichmentKinds(t *testing.T) {
	declared := map[string]bool{}
	for _, value := range loadEnumMembers(domainPackage, []string{"EnrichmentKind"})["EnrichmentKind"] {
		declared[value] = true
	}
	registryKinds := map[string]bool{}
	definitions := enrichment.Definitions(nil, nil)
	for index := range definitions {
		registryKinds[definitions[index].Kind] = true
	}
	for kind := range registryKinds {
		if !declared[kind] {
			t.Errorf("registry kind %q has no domain.EnrichmentKind const", kind)
		}
	}
	for value := range declared {
		if !registryKinds[value] {
			t.Errorf("domain.EnrichmentKind %q is not a registry kind — stale const", value)
		}
	}
}

func TestCrosswalks(t *testing.T) {
	for _, crosswalk := range crosswalks {
		t.Run(crosswalk.name, crosswalk.check)
	}
}

// fieldStructNames maps ast fields to their domain.Asset struct field where
// mechanical capitalization isn't enough. Virtual fields have no struct field.
var fieldStructNames = map[ast.Field]string{
	ast.FieldSource: "SourceID",
	ast.FieldTag:    "", // junction table, not a column
	ast.FieldText:   "", // FTS, not a column
}

func structName(field ast.Field) string {
	if name, ok := fieldStructNames[field]; ok {
		return name
	}
	name := string(field)
	return string(name[0]-'a'+'A') + name[1:]
}

func checkFieldsOnDomainAsset(t *testing.T) {
	assetType := reflect.TypeOf(domain.Asset{})
	for _, field := range ast.AllFields() {
		name := structName(field)
		if name == "" {
			continue
		}
		if _, ok := assetType.FieldByName(name); !ok {
			t.Errorf("ast field %q expects domain.Asset.%s — no such struct field", field, name)
		}
	}
}

// unexposedAssetFields is the deliberately-not-filterable manifest: every
// domain.Asset field must be either a query token or listed here with its
// reason. Adding a field to the struct without deciding its queryability
// fails the suite; promoting one means deleting its row here in the same
// change (a field in both places is also a failure).
var unexposedAssetFields = map[string]string{
	"ID":                 "identity, not a predicate (queries return it, never filter on it)",
	"RelativePath":       "location is the folder SCOPE's job, not a token",
	"LastVerifiedAt":     "reconciliation bookkeeping",
	"Extension":          "fileType is the semantic token; extension is its raw material",
	"MIMEType":           "seam/webview attribute (assettype registry output), not a user concept",
	"SizeBytes":          "sort-only for now (SortField `size`); a size token awaits a byte-size value editor",
	"MTime":              "change-detection fingerprint input",
	"PartialHash":        "identity-matrix internal",
	"DurationSecs":       "awaiting promotion with the video milestone (grid shows it; nobody filters yet)",
	"ColorSpace":         "awaiting promotion (needs enum normalization first)",
	"BitDepth":           "awaiting promotion",
	"FocalLengthMM":      "awaiting promotion (P-later capture-tech tokens)",
	"Aperture":           "awaiting promotion",
	"ShutterSpeed":       "awaiting promotion (stored as a string; needs a comparable form first)",
	"ISO":                "awaiting promotion",
	"GPSLat":             "awaiting promotion (a location token wants a radius/box grammar, not lat/lon leaves)",
	"GPSLon":             "awaiting promotion",
	"ExtendedMetadata":   "the blob is never load-bearing (C15/vocabulary.md); promotion recipe exists",
	"Note":               "awaiting decision: filter token vs FTS-only (frontend search-tier design owns it)",
	"JudgmentModifiedAt": "XMP conflict-resolution cursor",
	"XMPLastReadAt":      "sync cursor",
	"XMPLastWrittenAt":   "sync cursor",
	"XMPHash":            "sync cursor",
	"ThumbnailAt":        "derived-state marker",
	"IsDeleted":          "compiled into every query as is_deleted = 0, never user-facing",
	"DeletedAt":          "soft-delete bookkeeping",
	"UpdatedAt":          "row bookkeeping",
}

func checkAssetFieldsAccountedFor(t *testing.T) {
	exposed := map[string]bool{}
	for _, field := range ast.AllFields() {
		if name := structName(field); name != "" {
			exposed[name] = true
		}
	}

	assetType := reflect.TypeOf(domain.Asset{})
	seen := map[string]bool{}
	for i := 0; i < assetType.NumField(); i++ {
		name := assetType.Field(i).Name
		seen[name] = true
		isExposed := exposed[name]
		_, isUnexposed := unexposedAssetFields[name]
		switch {
		case isExposed && isUnexposed:
			t.Errorf("domain.Asset.%s is a query token AND in the unexposed manifest — delete its manifest row", name)
		case !isExposed && !isUnexposed:
			t.Errorf("domain.Asset.%s is neither a query token nor in the unexposed manifest — decide its queryability (add a vocabulary row, or a manifest row with the reason)", name)
		}
	}
	for name := range unexposedAssetFields {
		if !seen[name] {
			t.Errorf("unexposed manifest names %q, which is not a domain.Asset field — stale row", name)
		}
	}
}

// checkGrammarExecutes compiles and RUNS every field × operator combination
// against a migrated in-memory catalog. This is stronger than comparing
// column-name lists: a typo'd column, bad join, or invalid SQL fails here.
func checkGrammarExecutes(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "crosswalk")
	testutil.NewTestAsset(t, db, source.ID, "probe.jpg")
	now := time.Now()

	for _, field := range ast.AllFields() {
		spec, _ := ast.LookupField(field)
		for _, operator := range spec.Operators() {
			leaf := ast.Leaf{Field: field, Cmp: operator, Value: syntheticLeafValue(spec.Kind, field, operator)}
			statement, err := ast.CompileSelect(
				ast.Query{Version: ast.Version, Where: leaf},
				ast.Arrangement{SortField: ast.SortIngestedAt, SortDir: ast.SortDesc},
				ast.Page{Limit: 1}, now)
			if err != nil {
				t.Errorf("field=%q op=%q: compile: %v", field, operator, err)
				continue
			}
			if rows, err := db.Query(statement.SQL, statement.Args...); err != nil {
				t.Errorf("field=%q op=%q: execute: %v\nsql: %s", field, operator, err, statement.SQL)
			} else {
				_ = rows.Close()
			}
		}
	}

	// Scopes execute too (folder in both modes, collection, tag).
	scopes := []ast.Scope{
		{Kind: ast.ScopeLibrary},
		{Kind: ast.ScopeFolder, SourceID: source.ID, Recursive: true},
		{Kind: ast.ScopeFolder, SourceID: source.ID, Path: "2026/07"},
		{Kind: ast.ScopeCollection, ID: "c1"},
		{Kind: ast.ScopeTag, ID: "t1"},
	}
	for _, scope := range scopes {
		statement, err := ast.CompileSelect(
			ast.Query{Version: ast.Version, Scope: &scope},
			ast.Arrangement{SortField: ast.SortFilename, SortDir: ast.SortAsc},
			ast.Page{Limit: 1}, now)
		if err != nil {
			t.Errorf("scope=%q: compile: %v", scope.Kind, err)
			continue
		}
		if rows, err := db.Query(statement.SQL, statement.Args...); err != nil {
			t.Errorf("scope=%q: execute: %v", scope.Kind, err)
		} else {
			_ = rows.Close()
		}
	}
}

func checkSortFieldsExecute(t *testing.T) {
	db := testutil.NewTestDB(t)
	now := time.Now()
	for _, sortField := range []ast.SortField{ast.SortCapturedAt, ast.SortIngestedAt, ast.SortRating, ast.SortFilename, ast.SortSize} {
		statement, err := ast.CompileSelect(
			ast.Query{Version: ast.Version},
			ast.Arrangement{SortField: sortField, SortDir: ast.SortDesc},
			ast.Page{Limit: 1}, now)
		if err != nil {
			t.Errorf("sort=%q: compile: %v", sortField, err)
			continue
		}
		if rows, err := db.Query(statement.SQL, statement.Args...); err != nil {
			t.Errorf("sort=%q: execute: %v", sortField, err)
		} else {
			_ = rows.Close()
		}
	}
}

func syntheticLeafValue(kind ast.ValueKind, field ast.Field, operator ast.Operator) any {
	if operator == ast.OpEmpty || operator == ast.OpNotEmpty {
		return nil
	}
	membership := operator == ast.OpIn || operator == ast.OpNotIn
	switch kind {
	case ast.KindText, ast.KindFreeText:
		return "probe"
	case ast.KindNumeric:
		return float64(3)
	case ast.KindEnum:
		member := map[ast.Field]string{
			ast.FieldFileType:   string(domain.FileTypeImage),
			ast.FieldColorLabel: string(domain.ColorLabelRed),
			ast.FieldFlag:       string(domain.FlagPick),
			ast.FieldFileStatus: string(domain.FileStatusOnline),
		}[field]
		if membership {
			return []string{member}
		}
		return member
	case ast.KindDateRange:
		return ast.DateValue{Anchor: ast.DateAnchor{Now: true}, Duration: ast.DateDuration{Days: -30}}
	case ast.KindTagReference:
		return "tag-1"
	case ast.KindEntityReference:
		if membership {
			return []string{"src-1"}
		}
		return "src-1"
	}
	return nil
}

// subsetNames maps projection struct fields whose domain.Asset counterpart has
// a different name.
var subsetNames = map[string]string{"Extended": "ExtendedMetadata"}

func checkSubset(t *testing.T, projection reflect.Type, label string) {
	t.Helper()
	assetType := reflect.TypeOf(domain.Asset{})
	for i := 0; i < projection.NumField(); i++ {
		name := projection.Field(i).Name
		if mapped, ok := subsetNames[name]; ok {
			name = mapped
		}
		if _, ok := assetType.FieldByName(name); !ok {
			t.Errorf("%s.%s has no domain.Asset counterpart", label, projection.Field(i).Name)
		}
	}
}

func checkExtractionSubset(t *testing.T) {
	checkSubset(t, reflect.TypeOf(metadata.Metadata{}), "metadata.Metadata")
}

func checkFilePatchSubset(t *testing.T) {
	checkSubset(t, reflect.TypeOf(catalog.FilePatch{}), "catalog.FilePatch")
}

func checkTriageSubset(t *testing.T) {
	checkSubset(t, reflect.TypeOf(catalog.TriagePatch{}), "catalog.TriagePatch")
	checkSubset(t, reflect.TypeOf(catalog.TriageState{}), "catalog.TriageState")
}

func checkAssetDetailSubset(t *testing.T) {
	checkSubset(t, reflect.TypeOf(seam.AssetDetail{}), "seam.AssetDetail")
}

// checkTriagePatchInputWire pins the seam's triage WIRE struct's json field names
// (the mutation contract the frontend TriagePatch mirrors, task 34) to
// catalog.TriagePatch's fields, bidirectionally: every wire field names a patch
// field and every patch field is exposed on the wire. The frontend TriagePatch is
// a hand-authored composite (the three-state RawMessage fields can't be reflected
// into typed nullable-optional TS), so this Go-side check is the mechanism (C15)
// that keeps the wire vocabulary from drifting off the engine patch — add a field
// to TriagePatch without wiring it, or rename a json tag, and this fails.
func checkTriagePatchInputWire(t *testing.T) {
	wire := reflect.TypeOf(seam.TriagePatchInput{})
	patch := reflect.TypeOf(catalog.TriagePatch{})

	wireNames := map[string]bool{}
	for i := 0; i < wire.NumField(); i++ {
		name := strings.Split(wire.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			t.Errorf("seam.TriagePatchInput.%s has no json wire name", wire.Field(i).Name)
			continue
		}
		wireNames[name] = true
	}
	patchNames := map[string]bool{}
	for i := 0; i < patch.NumField(); i++ {
		name := patch.Field(i).Name
		patchNames[strings.ToLower(name[:1])+name[1:]] = true
	}
	for name := range wireNames {
		if !patchNames[name] {
			t.Errorf("seam wire field %q has no catalog.TriagePatch counterpart", name)
		}
	}
	for name := range patchNames {
		if !wireNames[name] {
			t.Errorf("catalog.TriagePatch field %q is not exposed on the seam wire (seam.TriagePatchInput) — the triage wire must mirror the engine patch", name)
		}
	}
}
