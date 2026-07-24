package main

// models.ts — shared struct models reflected from Go. The json tags on the Go
// structs ARE the wire contract (impl/16 shaped them field-for-field); this
// emitter projects them to TS interfaces so hand-written parallel shapes in
// contract.ts retire (C13/C15). Not a general tygo: it maps exactly the type
// vocabulary these models use and fails loudly on anything new.

import (
	"bytes"
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"github.com/charmbracelet/log"
)

const catalogPackage = "github.com/akmadian/alexandria/internal/catalog"

// modelManifest lists the shared structs, in emission order. Adding a shared
// model = adding a row; the emitter discovers fields by type-checking.
var modelManifest = []struct {
	pkg      string
	typeName string
	doc      string
}{
	{catalogPackage, "AssetRow", "The slim grid-card projection. The seam adapter layers thumbURL + the kind discriminator on top."},
	{seamPackage, "AssetDetail", "The full-asset detail projection GetAsset returns — the inspector's read."},
	{seamPackage, "Envelope", "The one C8 event envelope."},
	{seamPackage, "CatalogChange", "catalog/changed payload."},
	{seamPackage, "JobProgress", "jobs/progress payload (C9)."},
	{seamPackage, "JobSummary", "Completion tally carried by JobDone."},
	{seamPackage, "JobDone", "jobs/done payload."},
	{seamPackage, "HistoryState", "catalog/historyChanged payload."},
	{seamPackage, "VolumeStatus", "watcher/volumeStatus payload."},
	{seamPackage, "VolumeNode", "A storage volume with its tracked-root folders — a node in the getFolderTree forest (D41)."},
	{seamPackage, "FolderNode", "One node in a volume's folder tree; recursive via children (D41)."},
	{seamPackage, "CollectionNode", "A collection projected for the rail; flat list, parentId adjacency (D41)."},
	{seamPackage, "FolderBehaviorChange", "One tracked root whose sync policy would change under a proposed absorb (D41)."},
	{seamPackage, "CreateFolderOutcome", "The disposition of a createFolder attempt plus the folders it touched (D41)."},
	{seamPackage, "FolderPatch", "The sparse updateFolder input (name / sync mode)."},
}

// enumImports maps named Go enum types to the generated union file that
// declares them. Anything else named (outside the manifest + time.Time) is a
// fatal error — extend deliberately, never coerce silently.
var enumImports = map[string]string{
	"FileType": "enums", "ColorLabel": "enums", "Flag": "enums",
	"FileStatus": "enums", "VolumeKind": "enums", "VolumeConnectivity": "enums",
	"EnrichmentKind": "enums", "CollectionKind": "enums", "SyncMode": "enums",
	"CreateFolderOutcomeKind": "enums",
	"Topic":                   "events", "EventType": "events", "JobState": "events",
}

func renderModels() []byte {
	structs := loadStructTypes()

	manifestNames := make(map[string]bool, len(modelManifest))
	for _, entry := range modelManifest {
		manifestNames[entry.typeName] = true
	}

	var body bytes.Buffer
	imports := map[string]map[string]bool{} // file → union names
	for _, entry := range modelManifest {
		structType, ok := structs[entry.typeName]
		if !ok {
			log.Fatalf("generate: model %q not found in %s", entry.typeName, entry.pkg)
		}
		fmt.Fprintf(&body, "/** %s */\n", entry.doc)
		fmt.Fprintf(&body, "export interface %s {\n", entry.typeName)
		for i := 0; i < structType.NumFields(); i++ {
			field := structType.Field(i)
			if !field.Exported() {
				continue
			}
			jsonName, omitEmpty := jsonTag(structType.Tag(i), entry.typeName, field.Name())
			tsName, optional := jsonName, ""
			if omitEmpty {
				optional = "?"
			}
			rendered := tsType(field.Type(), manifestNames, imports, !omitEmpty)
			fmt.Fprintf(&body, "  %s%s: %s;\n", tsName, optional, rendered)
		}
		body.WriteString("}\n\n")
	}

	var buffer bytes.Buffer
	header(&buffer, "internal/catalog + internal/seam (struct json tags)")
	for _, file := range []string{"enums", "events"} {
		names := sortedKeysBool(imports[file])
		if len(names) == 0 {
			continue
		}
		fmt.Fprintf(&buffer, "import type { %s } from \"./%s\";\n", strings.Join(names, ", "), file)
	}
	buffer.WriteString("\n")
	buffer.Write(bytes.TrimRight(body.Bytes(), "\n"))
	buffer.WriteString("\n")
	return buffer.Bytes()
}

// loadStructTypes type-checks the model packages and returns each manifest
// type's underlying struct.
func loadStructTypes() map[string]*types.Struct {
	pkgs := loadPackages(catalogPackage, seamPackage)

	out := map[string]*types.Struct{}
	for _, pkg := range pkgs {
		scope := pkg.Types.Scope()
		for _, entry := range modelManifest {
			object := scope.Lookup(entry.typeName)
			if object == nil {
				continue
			}
			structType, ok := object.Type().Underlying().(*types.Struct)
			if !ok {
				log.Fatalf("generate: model %q is not a struct", entry.typeName)
			}
			out[entry.typeName] = structType
		}
	}
	return out
}

// jsonTag extracts the wire name and omitempty flag. A shared model field
// without an explicit json tag is a contract bug — fail loudly.
func jsonTag(tag, typeName, fieldName string) (name string, omitEmpty bool) {
	value, ok := reflect.StructTag(tag).Lookup("json")
	if !ok || value == "" || value == "-" {
		log.Fatalf("generate: %s.%s has no json tag — shared models declare their wire name explicitly", typeName, fieldName)
	}
	parts := strings.Split(value, ",")
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
		}
	}
	return parts[0], omitEmpty
}

// tsType maps a Go type to its TS rendering. nullable pointers (no omitempty)
// render `T | null`; omitempty pointers render plain `T` behind a `?` key.
func tsType(goType types.Type, manifest map[string]bool, imports map[string]map[string]bool, pointerIsNull bool) string {
	goType = types.Unalias(goType) // `any` → interface{}
	switch typed := goType.(type) {
	case *types.Pointer:
		inner := tsType(typed.Elem(), manifest, imports, false)
		if pointerIsNull {
			return inner + " | null"
		}
		return inner
	case *types.Slice:
		return tsType(typed.Elem(), manifest, imports, false) + "[]"
	case *types.Basic:
		switch info := typed.Info(); {
		case info&types.IsString != 0:
			return "string"
		case info&types.IsBoolean != 0:
			return "boolean"
		case info&types.IsNumeric != 0:
			return "number"
		}
	case *types.Named:
		object := typed.Obj()
		if object.Pkg() != nil && object.Pkg().Path() == "time" && object.Name() == "Time" {
			return "string" // RFC 3339 on the wire
		}
		if manifest[object.Name()] {
			return object.Name()
		}
		if file, ok := enumImports[object.Name()]; ok {
			unionName := object.Name()
			if mapped, renamed := tsUnionName[unionName]; renamed {
				unionName = mapped
			}
			if imports[file] == nil {
				imports[file] = map[string]bool{}
			}
			imports[file][unionName] = true
			return unionName
		}
	case *types.Interface:
		if typed.Empty() {
			return "unknown"
		}
	case *types.Map:
		key := tsType(typed.Key(), manifest, imports, false)
		value := tsType(typed.Elem(), manifest, imports, false)
		return "Record<" + key + ", " + value + ">"
	}
	log.Fatalf("generate: unmapped Go type %s — extend the model emitter deliberately", goType.String())
	return ""
}

func sortedKeysBool(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
