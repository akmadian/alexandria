// Command generate emits the TypeScript the frontend query layer consumes, in
// the string-literal-union shape frontend/09 mandates (no TS `enum`s). Two
// surfaces, two files, each importing its Go source of truth directly so it
// compiles against it and cannot drift (C13):
//
//   - vocabulary.ts — from internal/ast: the TokenField / TokenOperator /
//     ValueKind unions and the per-field grammar table.
//   - enums.ts — from internal/domain: the closed-set enums (FileType,
//     ColorLabel, …) as literal unions.
//
// This is the whole TS-generation story for these shapes: Wails handles struct
// models by reflection, but Go has no reflectable enum, so the member sets come
// from domain's authored lists and are emitted here. Output is committed and CI
// enforces freshness; it is NOT hooked into `wails build`. Run it with
// `make generate-seam` after changing the Go vocabulary or a domain enum.
//
// Output is deterministic: every union's members are sorted, so two runs produce
// byte-identical files.
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
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o750); err != nil {
		log.Fatalf("generate: mkdir %s: %v", *outDir, err)
	}
	write(*outDir, "vocabulary.ts", renderVocabulary())
	write(*outDir, "enums.ts", renderDomainEnums())
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
		for _, operator := range spec.Operators {
			operatorSet[operator] = struct{}{}
		}
		kindSet[spec.Kind] = struct{}{}
	}

	var buffer bytes.Buffer
	header(&buffer, "internal/ast (vocabulary.go)")

	writeUnion(&buffer, "TokenField", stringsOf(fields))
	writeUnion(&buffer, "TokenOperator", sortedKeys(operatorSet))
	writeUnion(&buffer, "ValueKind", sortedKeys(kindSet))

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
			operatorList(spec.Operators),
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

var domainEnumTypes = []string{
	"FileType",
	"ColorLabel",
	"Flag",
	"FileStatus",
	"SourceKind",
	"SourceConnectivity",
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

// loadEnumMembers type-checks pkgPath and collects, for each requested named
// type, the string values of every constant of that type. This is how the
// generator enumerates a Go pseudo-enum without a hand-maintained list — the
// consts are the single source of truth. It fails loudly if a requested type
// has no members (renamed or removed), so the manifest can't silently drift.
func loadEnumMembers(pkgPath string, typeNames []string) map[string][]string {
	config := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedDeps | packages.NeedImports}
	pkgs, err := packages.Load(config, pkgPath)
	if err != nil {
		log.Fatalf("generate: load %s: %v", pkgPath, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		log.Fatalf("generate: %s has type errors", pkgPath)
	}

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
			log.Fatalf("generate: domain enum %q has no string consts (renamed or removed?)", name)
		}
	}
	return members
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
	buffer.WriteString("// Code generated by internal/seam/generate; DO NOT EDIT.\n")
	fmt.Fprintf(buffer, "// Source of truth: %s, per C13.\n", source)
	buffer.WriteString("// Regenerate with `make generate-seam` after changing the Go source.\n\n")
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
