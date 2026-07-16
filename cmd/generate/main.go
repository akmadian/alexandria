// Command generate is the schema compiler (C15): it projects the Go-declared
// vocabularies and shared models into every derived medium. The declarations
// live in internal/domain (nouns, enums), internal/ast (the query grammar),
// and internal/seam (error codes, event catalog, payload shapes); this command
// imports them directly so it compiles against the source of truth and cannot
// drift (C13). Outputs:
//
//   - vocabulary.ts — the token grammar (TokenField/TokenOperator/ValueKind +
//     fieldGrammar) and the query-shape unions (ScopeKind/GroupOp/SortField/
//     SortDir), from internal/ast.
//   - enums.ts — the domain closed-set enums as literal unions.
//   - errors.ts / events.ts — the seam's error and event catalogs.
//   - models.ts — shared struct models (AssetRow, event payloads), reflected
//     from the Go structs whose json tags are the wire contract.
//   - docs/data-dictionary.md — the human-readable inventory of all of the
//     above, with the crosswalk map.
//
// Output is committed and CI enforces freshness; it is NOT hooked into
// `wails build`. Run with `make generate` (or `go generate ./...` — the source
// packages carry go:generate directives pointing here).
//
// Output is deterministic: unions are sorted, struct fields keep declaration
// order, so two runs produce byte-identical files.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/constant"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/log"
	"golang.org/x/tools/go/packages"

	"github.com/akmadian/alexandria/internal/ast"
)

func main() {
	outDir := flag.String("out", "frontend/src/_generated-types",
		"directory to write generated TypeScript into (relative to cwd)")
	docsDir := flag.String("docs", "docs",
		"directory to write the generated data dictionary into (relative to cwd)")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o750); err != nil {
		log.Fatalf("generate: mkdir %s: %v", *outDir, err)
	}
	if err := os.MkdirAll(*docsDir, 0o750); err != nil {
		log.Fatalf("generate: mkdir %s: %v", *docsDir, err)
	}
	write(*outDir, "vocabulary.ts", renderVocabulary())
	write(*outDir, "enums.ts", renderDomainEnums())
	write(*outDir, "errors.ts", renderSeamCodes())
	write(*outDir, "events.ts", renderSeamEvents())
	write(*outDir, "models.ts", renderModels())
	write(*docsDir, "data-dictionary.md", renderDataDictionary())
}

func write(dir, name string, source []byte) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, source, 0o600); err != nil {
		log.Fatalf("generate: write %s: %v", path, err)
	}
	log.Info("generate: wrote", "path", path)
}

// renderVocabulary builds vocabulary.ts from internal/ast. Split out from main so
// the test can assert on the string without touching the filesystem.
func renderVocabulary() []byte {
	fields := ast.AllFields()
	sort.Slice(fields, func(i, j int) bool { return fields[i] < fields[j] })

	// The operator and kind unions are the reachable grammar: every operator/kind
	// any field allows. Deriving them from the vocabulary (not a separate const
	// list) keeps them honest — an operator no field permits is not part of the
	// grammar and should not appear in the union.
	operatorSet := map[ast.Operator]struct{}{}
	kindSet := map[ast.ValueKind]struct{}{}
	for _, field := range fields {
		spec, _ := ast.LookupField(field)
		for _, operator := range spec.Operators() {
			operatorSet[operator] = struct{}{}
		}
		kindSet[spec.Kind] = struct{}{}
	}

	var buffer bytes.Buffer
	header(&buffer, "internal/ast (vocabulary.go, types.go)")

	writeUnion(&buffer, "TokenField", stringsOf(fields))
	writeUnion(&buffer, "TokenOperator", sortedKeys(operatorSet))
	writeUnion(&buffer, "ValueKind", sortedKeys(kindSet))

	// The query-shape unions ride the same file: they are query vocabulary
	// too, and generating them retires the hand-written TS copies that had
	// already drifted (SortField members, ScopeKind alphabet — audit F1/F2).
	queryShapeMembers := loadEnumMembers(astPackage, queryShapeTypes)
	for _, typeName := range queryShapeTypes {
		writeUnion(&buffer, typeName, stringsOf(queryShapeMembers[typeName]))
	}

	buffer.WriteString("export interface FieldGrammar {\n")
	buffer.WriteString("  operators: readonly TokenOperator[];\n")
	buffer.WriteString("  kind: ValueKind;\n")
	buffer.WriteString("  suggestable: boolean;\n")
	buffer.WriteString("}\n\n")

	// `satisfies Record<TokenField, FieldGrammar>` is the completeness gate
	// (C10, frontend/09): the frontend typechecks this generated table, so a
	// field missing from the grammar map is a compile error downstream.
	buffer.WriteString("export const fieldGrammar = {\n")
	for _, field := range fields {
		spec, _ := ast.LookupField(field)
		// Field names are camelCase identifiers by construction (they are the
		// TokenField vocabulary), so object keys need no quoting.
		fmt.Fprintf(&buffer, "  %s: { operators: [%s], kind: %s, suggestable: %t },\n",
			string(field),
			operatorList(spec.Operators()),
			quote(string(spec.Kind)),
			spec.Suggestable,
		)
	}
	buffer.WriteString("} as const satisfies Record<TokenField, FieldGrammar>;\n")

	return buffer.Bytes()
}

// domainPackage and domainEnumTypes are the generation manifest: which domain
// string-enums get published to the frontend as literal unions. This lists only
// the *type names* — the members are discovered from the consts by type-checking
// the package (loadEnumMembers), so domain stays pure `type`+`const` with no
// list to drift, and adding a const surfaces automatically.
const domainPackage = "github.com/akmadian/alexandria/internal/domain"

// astPackage and queryShapeTypes: the query-shape unions generated into
// vocabulary.ts alongside the token grammar, discovered by type-checking
// internal/ast exactly like the domain enums.
const astPackage = "github.com/akmadian/alexandria/internal/ast"

var queryShapeTypes = []string{"ScopeKind", "GroupOp", "SortField", "SortDir"}

var domainEnumTypes = []string{
	"FileType",
	"ColorLabel",
	"Flag",
	"FileStatus",
	"SourceKind",
	"SourceConnectivity",
	"EnrichmentKind",
}

// renderDomainEnums builds enums.ts from the discovered members. The type order
// is authored (fixed) and each union's members are sorted, so output is
// deterministic.
func renderDomainEnums() []byte {
	members := loadEnumMembers(domainPackage, domainEnumTypes)

	var buffer bytes.Buffer
	header(&buffer, "internal/domain (asset.go, source.go)")
	for _, typeName := range domainEnumTypes {
		writeUnion(&buffer, typeName, stringsOf(members[typeName]))
	}
	return bytes.TrimRight(buffer.Bytes(), "\n")
}

// seamPackage and seamCodeTypes are the error-catalog manifest: the ApiError kind
// and code unions the frontend switches on (impl/15 §4). Members are discovered
// the same way as the domain enums — from the consts in apierror.go — so the
// catalog is single-sourced in Go and cannot drift.
const seamPackage = "github.com/akmadian/alexandria/internal/seam"

var seamCodeTypes = []string{"ApiErrorKind", "ErrorCode"}

// renderSeamCodes builds errors.ts from the seam's ApiErrorKind / ErrorCode consts.
func renderSeamCodes() []byte {
	members := loadEnumMembers(seamPackage, seamCodeTypes)

	var buffer bytes.Buffer
	header(&buffer, "internal/seam (apierror.go)")
	for _, typeName := range seamCodeTypes {
		writeUnion(&buffer, typeName, stringsOf(members[typeName]))
	}
	return bytes.TrimRight(buffer.Bytes(), "\n")
}

// seamEventsSourceLabel is the events.ts header's source attribution. The C8
// topic/type/job-state unions the event pump switches on are discovered from
// the consts in events.go the same way as the domain enums; the event PAYLOAD
// structs are generated too, by the model emitter (models.go → models.ts).
const seamEventsSourceLabel = "internal/seam (events.go)"

// tsUnionName maps a Go enum type name to its TypeScript union name where they
// differ. Topic → EventTopic (a bare "Topic" is too generic in the TS namespace);
// everything else keeps its Go name.
var tsUnionName = map[string]string{"Topic": "EventTopic"}

// renderSeamEvents builds events.ts from the seam's Topic / EventType / JobState
// consts.
func renderSeamEvents() []byte {
	typeNames := []string{"Topic", "EventType", "JobState"}
	members := loadEnumMembers(seamPackage, typeNames)

	var buffer bytes.Buffer
	header(&buffer, seamEventsSourceLabel)
	for _, typeName := range typeNames {
		name := typeName
		if mapped, ok := tsUnionName[typeName]; ok {
			name = mapped
		}
		writeUnion(&buffer, name, stringsOf(members[typeName]))
	}
	return bytes.TrimRight(buffer.Bytes(), "\n")
}

// loadEnumMembers type-checks pkgPath and collects, for each requested named
// type, the string values of every constant of that type. This is how the
// generator enumerates a Go pseudo-enum without a hand-maintained list — the
// consts are the single source of truth. It fails loudly if a requested type
// has no members (renamed or removed), so the manifest can't silently drift.
func loadEnumMembers(pkgPath string, typeNames []string) map[string][]string {
	pkgs := loadPackages(pkgPath)

	want := make(map[string]bool, len(typeNames))
	for _, name := range typeNames {
		want[name] = true
	}

	members := map[string][]string{}
	scope := pkgs[0].Types.Scope()
	for _, name := range scope.Names() {
		enumConst, ok := scope.Lookup(name).(*types.Const)
		if !ok {
			continue
		}
		named, ok := enumConst.Type().(*types.Named)
		if !ok || !want[named.Obj().Name()] {
			continue
		}
		members[named.Obj().Name()] = append(members[named.Obj().Name()], stringValue(enumConst))
	}

	for _, name := range typeNames {
		if len(members[name]) == 0 {
			log.Fatalf("generate: enum %q in %s has no string consts (renamed or removed?)", name, pkgPath)
		}
	}
	return members
}

// packageCache memoizes type-checked loads — every emitter (and the
// determinism test's double renders) reuses the same packages, and
// packages.Load is by far the slowest step.
var packageCache = map[string][]*packages.Package{}

func loadPackages(patterns ...string) []*packages.Package {
	key := strings.Join(patterns, "\x00")
	if cached, ok := packageCache[key]; ok {
		return cached
	}
	config := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedDeps | packages.NeedImports}
	pkgs, err := packages.Load(config, patterns...)
	if err != nil {
		log.Fatalf("generate: load %s: %v", strings.Join(patterns, ", "), err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		log.Fatalf("generate: %s has type errors", strings.Join(patterns, ", "))
	}
	packageCache[key] = pkgs
	return pkgs
}

// stringValue extracts a string-constant's value, failing on a non-string enum
// (the generator only knows how to emit string-literal unions).
func stringValue(c *types.Const) string {
	if c.Val().Kind() != constant.String {
		log.Fatalf("generate: const %s is not a string enum", c.Name())
	}
	return constant.StringVal(c.Val())
}

func header(buffer *bytes.Buffer, source string) {
	buffer.WriteString("// Code generated by cmd/generate; DO NOT EDIT.\n")
	fmt.Fprintf(buffer, "// Source of truth: %s, per C13/C15.\n", source)
	buffer.WriteString("// Regenerate with `make generate` after changing the Go source.\n\n")
}

// writeUnion emits `export type Name = "a" | "b" | ...;`, one member per line so
// diffs stay readable when the set grows. Members must be pre-ordered.
func writeUnion(buffer *bytes.Buffer, name string, members []string) {
	fmt.Fprintf(buffer, "export type %s =\n", name)
	for i, member := range members {
		terminator := ""
		if i == len(members)-1 {
			terminator = ";"
		}
		fmt.Fprintf(buffer, "  | %s%s\n", quote(member), terminator)
	}
	buffer.WriteString("\n")
}

func operatorList(operators []ast.Operator) string {
	quoted := make([]string, len(operators))
	for i, operator := range operators {
		quoted[i] = quote(string(operator))
	}
	return strings.Join(quoted, ", ")
}

// stringsOf converts a slice of string-kinded values to sorted plain strings —
// the shape writeUnion wants, deterministic regardless of source order.
func stringsOf[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	sort.Strings(out)
	return out
}

// sortedKeys returns the sorted string forms of a set's keys.
func sortedKeys[T ~string](set map[T]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, string(key))
	}
	sort.Strings(out)
	return out
}

func quote(value string) string {
	return `"` + value + `"`
}
