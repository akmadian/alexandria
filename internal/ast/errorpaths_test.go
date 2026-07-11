package ast_test

// Error propagation through the public compile family: every Compile* entry
// rejects an invalid query before touching SQL, and errors inside the tree
// surface through every nesting level.

import (
	"strings"
	"testing"

	"github.com/akmadian/alexandria/internal/ast"
)

func invalidQuery() ast.Query {
	// Version 0 fails structural validation.
	return ast.Query{Version: 0}
}

func TestCompileFamily_RejectsInvalidQuery(t *testing.T) {
	now := fixedNow()
	arrangement := defaultArrangement()

	if _, err := ast.CompileSelect(invalidQuery(), arrangement, ast.Page{Limit: 1}, now); err == nil {
		t.Error("CompileSelect: expected validation error")
	}
	if _, err := ast.CompileCount(invalidQuery(), now); err == nil {
		t.Error("CompileCount: expected validation error")
	}
	if _, err := ast.CompileIDSlice(invalidQuery(), arrangement, 0, 10, now); err == nil {
		t.Error("CompileIDSlice: expected validation error")
	}
	if _, err := ast.CompileIndexOf(invalidQuery(), arrangement, "id", now); err == nil {
		t.Error("CompileIndexOf: expected validation error")
	}
	if _, err := ast.CompileWhere(invalidQuery(), nil, now); err == nil {
		t.Error("CompileWhere: expected validation error")
	}
}

func TestCompileDistinctValues_Errors(t *testing.T) {
	if _, err := ast.CompileDistinctValues("bogus"); err == nil {
		t.Error("unknown field: expected error")
	}
	if _, err := ast.CompileDistinctValues(ast.FieldRating); err == nil {
		t.Error("non-suggestable field: expected error")
	}
}

func TestCompileDistinctValues_Suggestable(t *testing.T) {
	statement, err := ast.CompileDistinctValues(ast.FieldCameraMake)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(statement.SQL, "SELECT DISTINCT camera_make") {
		t.Fatalf("unexpected SQL: %s", statement.SQL)
	}
}

func TestMergeScope_AllBranches(t *testing.T) {
	scope := &ast.Scope{Kind: ast.ScopeLibrary}
	predicate := ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(3)}

	empty := ast.MergeScope(ast.Query{Version: 1, Scope: scope}, nil)
	if empty.Where != nil {
		t.Error("nil/nil: expected nil where")
	}
	storedOnly := ast.MergeScope(ast.Query{Version: 1}, predicate)
	if storedOnly.Where == nil {
		t.Error("nil outer: expected stored predicate")
	}
	outerOnly := ast.MergeScope(ast.Query{Version: 1, Where: predicate}, nil)
	if outerOnly.Where == nil {
		t.Error("nil stored: expected outer predicate")
	}
}

func TestCompile_UnknownSortFieldFallsBack(t *testing.T) {
	statement, err := ast.CompileSelect(
		ast.Query{Version: ast.Version},
		ast.Arrangement{SortField: "bogus", SortDir: ast.SortAsc},
		ast.Page{Limit: 1}, fixedNow())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(statement.SQL, "ORDER BY ingested_at ASC") {
		t.Fatalf("expected ingested_at fallback: %s", statement.SQL)
	}
}

// Errors inside nested nodes surface through every compile path: a leaf that
// validates per-node but fails at compile time does not exist by construction,
// so nesting errors are exercised with an unknown field smuggled past Validate
// via CompileWhere's tree — Validate catches it first, which IS the contract.
func TestCompile_NestedInvalidLeafCaughtByValidation(t *testing.T) {
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Group{
			Op: ast.GroupAnd,
			Children: []ast.Node{
				ast.Group{
					Op:       ast.GroupOr,
					Children: []ast.Node{ast.Leaf{Field: "bogus", Cmp: ast.OpEq, Value: "x"}},
				},
			},
		},
	}
	if _, err := ast.CompileWhere(query, nil, fixedNow()); err == nil {
		t.Fatal("expected validation error for nested unknown field")
	}
}

func TestJSON_ErrorPaths(t *testing.T) {
	var query ast.Query

	cases := map[string]string{
		"unknown key":       `{"version":1,"bogus":true}`,
		"zero version":      `{"version":0}`,
		"ambiguous node":    `{"version":1,"where":{"op":"and","field":"rating"}}`,
		"empty node":        `{"version":1,"where":{}}`,
		"bad group child":   `{"version":1,"where":{"op":"and","children":[{"op":"or","children":[{}]}]}}`,
		"group unknown key": `{"version":1,"where":{"op":"and","children":[],"bogus":1}}`,
		"leaf unknown key":  `{"version":1,"where":{"field":"rating","cmp":"eq","value":3,"bogus":1}}`,
		"non-object node":   `{"version":1,"where":[1,2]}`,
		"enum array mix":    `{"version":1,"where":{"field":"flag","cmp":"in","value":["pick",3]}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if err := query.UnmarshalJSON([]byte(raw)); err == nil {
				t.Fatalf("expected decode error for %s", name)
			}
		})
	}

	tooNew := `{"version":99}`
	err := query.UnmarshalJSON([]byte(tooNew))
	if err == nil {
		t.Fatal("expected version-too-new error")
	}

	// Unknown field passes coercion untouched — validation owns the rejection.
	unknownField := `{"version":1,"where":{"field":"bogus","cmp":"eq","value":"x"}}`
	if err := query.UnmarshalJSON([]byte(unknownField)); err != nil {
		t.Fatalf("unknown field must decode (validate rejects later): %v", err)
	}

	// A non-marshalable leaf value errors on encode.
	bad := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldTitle, Cmp: ast.OpEq, Value: make(chan int)},
	}
	if _, err := bad.MarshalJSON(); err == nil {
		t.Fatal("expected marshal error for non-marshalable value")
	}
	badNested := ast.Query{
		Version: ast.Version,
		Where:   ast.Group{Op: ast.GroupAnd, Children: []ast.Node{ast.Leaf{Field: ast.FieldTitle, Cmp: ast.OpEq, Value: make(chan int)}}},
	}
	if _, err := badNested.MarshalJSON(); err == nil {
		t.Fatal("expected marshal error for non-marshalable nested value")
	}
}

func TestJSON_CoercionPassthroughs(t *testing.T) {
	var query ast.Query

	// Numeric value stays a float64; scalar enum value stays a string.
	raw := `{"version":1,"where":{"op":"and","children":[
		{"field":"rating","cmp":"gte","value":3},
		{"field":"filename","cmp":"contains","value":"IMG"}
	]}}`
	if err := query.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := ast.Validate(query); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Non-string, non-array enum value passes coercion (validate rejects).
	rawBadEnum := `{"version":1,"where":{"field":"flag","cmp":"in","value":42}}`
	if err := query.UnmarshalJSON([]byte(rawBadEnum)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := ast.Validate(query); err == nil {
		t.Fatal("expected validation error for numeric enum value")
	}

	// A scalar string for an enum `in` also passes coercion untouched —
	// validation owns the array-shape rejection.
	rawScalarEnum := `{"version":1,"where":{"field":"flag","cmp":"in","value":"pick"}}`
	if err := query.UnmarshalJSON([]byte(rawScalarEnum)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := ast.Validate(query); err == nil {
		t.Fatal("expected validation error for scalar enum value")
	}
}

func TestCompileSelect_Paging(t *testing.T) {
	statement, err := ast.CompileSelect(
		ast.Query{Version: ast.Version}, defaultArrangement(),
		ast.Page{Limit: 50, Offset: 100}, fixedNow())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(statement.SQL, "LIMIT 50 OFFSET 100") {
		t.Fatalf("expected LIMIT/OFFSET: %s", statement.SQL)
	}
}
