package ast_test

import (
	"strings"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
)

// Parameterization property: no user value substring appears in compiled SQL text.
func TestCompile_ParameterizationSafety(t *testing.T) {
	hostile := []string{
		"'; DROP TABLE assets; --",
		`" OR 1=1 --`,
		"%_\\",
		`*?[`,
		`AND OR NOT NEAR`,
	}
	now := fixedNow()

	for _, input := range hostile {
		query := ast.Query{
			Version: ast.Version,
			Where: ast.Group{
				Op: ast.GroupAnd,
				Children: []ast.Node{
					ast.Leaf{Field: ast.FieldFilename, Cmp: ast.OpContains, Value: input},
					ast.Leaf{Field: ast.FieldText, Cmp: ast.OpMatches, Value: input},
					ast.Leaf{Field: ast.FieldCameraMake, Cmp: ast.OpEq, Value: input},
				},
			},
		}
		statement, err := ast.CompileSelect(query, defaultArrangement(), ast.Page{Limit: 10}, now)
		if err != nil {
			t.Fatalf("compile with hostile input %q: %v", input, err)
		}
		// The raw hostile input must NOT appear in the SQL text.
		if strings.Contains(statement.SQL, input) {
			t.Errorf("hostile input %q found verbatim in SQL: %s", input, statement.SQL)
		}
		// Values must appear only in Args.
		if len(statement.Args) == 0 {
			t.Errorf("expected bound args for hostile input %q", input)
		}
	}
}

// Determinism: same query + same now = identical Statement.
func TestCompile_Determinism(t *testing.T) {
	now := fixedNow()
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Group{
			Op: ast.GroupAnd,
			Children: []ast.Node{
				ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(3)},
				ast.Leaf{Field: ast.FieldFileType, Cmp: ast.OpIn, Value: []string{"image", "raw"}},
			},
		},
	}
	arrangement := defaultArrangement()

	statement1, err := ast.CompileSelect(query, arrangement, ast.Page{Limit: 50}, now)
	if err != nil {
		t.Fatalf("compile 1: %v", err)
	}
	statement2, err := ast.CompileSelect(query, arrangement, ast.Page{Limit: 50}, now)
	if err != nil {
		t.Fatalf("compile 2: %v", err)
	}
	if statement1.SQL != statement2.SQL {
		t.Fatalf("SQL not deterministic:\n  1: %s\n  2: %s", statement1.SQL, statement2.SQL)
	}
	if len(statement1.Args) != len(statement2.Args) {
		t.Fatalf("arg count differs: %d vs %d", len(statement1.Args), len(statement2.Args))
	}
}

// AnchorNow with two different `now` values produces different args.
func TestCompile_AnchorNowRolling(t *testing.T) {
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Leaf{
			Field: ast.FieldCapturedAt,
			Cmp:   ast.OpWithin,
			Value: ast.DateValue{
				Anchor:   ast.DateAnchor{Now: true},
				Duration: ast.DateDuration{Days: -30},
			},
		},
	}
	arrangement := defaultArrangement()

	now1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	now2 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)

	statement1, _ := ast.CompileSelect(query, arrangement, ast.Page{Limit: 10}, now1)
	statement2, _ := ast.CompileSelect(query, arrangement, ast.Page{Limit: 10}, now2)

	if statement1.SQL != statement2.SQL {
		t.Fatal("SQL should be identical (same structure)")
	}
	// Args should differ because the resolved dates differ.
	args1Match := argsEqual(statement1.Args, statement2.Args)
	if args1Match {
		t.Fatal("args should differ for different now values")
	}
}

// Tiebreaker invariant: every compiled ORDER BY ends in ", id ASC" regardless
// of sort direction (seam/01 §Additions #4 — tied rows must not reorder when
// the user flips direction).
func TestCompile_TiebreakerInvariant(t *testing.T) {
	now := fixedNow()
	query := ast.Query{Version: ast.Version}

	for _, sortField := range []ast.SortField{ast.SortCapturedAt, ast.SortIngestedAt, ast.SortRating, ast.SortFilename, ast.SortSize} {
		for _, sortDir := range []ast.SortDir{ast.SortAsc, ast.SortDesc} {
			arrangement := ast.Arrangement{SortField: sortField, SortDir: sortDir}
			statement, err := ast.CompileSelect(query, arrangement, ast.Page{Limit: 10}, now)
			if err != nil {
				t.Fatalf("sort=%q dir=%q: %v", sortField, sortDir, err)
			}
			orderBy := extractOrderBy(statement.SQL)
			if !strings.HasSuffix(orderBy, ", id ASC") {
				t.Errorf("sort=%q dir=%q: ORDER BY %q missing invariant suffix %q", sortField, sortDir, orderBy, ", id ASC")
			}
		}
	}
}

// CompileCount produces a counting statement.
func TestCompileCount(t *testing.T) {
	now := fixedNow()
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(3)},
	}
	statement, err := ast.CompileCount(query, now)
	if err != nil {
		t.Fatalf("compile count: %v", err)
	}
	if !strings.HasPrefix(statement.SQL, "SELECT COUNT(*)") {
		t.Fatalf("expected SELECT COUNT(*), got: %s", statement.SQL)
	}
}

// CompileIDSlice produces an id-only window.
func TestCompileIDSlice(t *testing.T) {
	now := fixedNow()
	query := ast.Query{Version: ast.Version}
	statement, err := ast.CompileIDSlice(query, defaultArrangement(), 10, 20, now)
	if err != nil {
		t.Fatalf("compile id slice: %v", err)
	}
	if !strings.HasPrefix(statement.SQL, "SELECT id FROM assets") {
		t.Fatalf("expected SELECT id, got: %s", statement.SQL)
	}
	if !strings.Contains(statement.SQL, "LIMIT 10 OFFSET 10") {
		t.Fatalf("expected LIMIT 10 OFFSET 10, got: %s", statement.SQL)
	}
}

// CompileIndexOf produces a position lookup.
func TestCompileIndexOf(t *testing.T) {
	now := fixedNow()
	query := ast.Query{Version: ast.Version}
	statement, err := ast.CompileIndexOf(query, defaultArrangement(), "asset-123", now)
	if err != nil {
		t.Fatalf("compile index of: %v", err)
	}
	if !strings.Contains(statement.SQL, "ROW_NUMBER()") {
		t.Fatalf("expected ROW_NUMBER(), got: %s", statement.SQL)
	}
	if statement.Args[len(statement.Args)-1] != "asset-123" {
		t.Fatalf("expected asset-123 as last arg, got: %v", statement.Args)
	}
}

// CompileWhere with exceptIDs.
func TestCompileWhere_ExceptIDs(t *testing.T) {
	now := fixedNow()
	query := ast.Query{Version: ast.Version}
	statement, err := ast.CompileWhere(query, []string{"id-1", "id-2"}, now)
	if err != nil {
		t.Fatalf("compile where: %v", err)
	}
	if !strings.Contains(statement.SQL, "NOT IN") {
		t.Fatalf("expected NOT IN clause, got: %s", statement.SQL)
	}
	if len(statement.Args) != 2 {
		t.Fatalf("expected 2 args for exceptIDs, got %d", len(statement.Args))
	}
}

// CompileDistinctValues.
func TestCompileDistinctValues(t *testing.T) {
	statement, err := ast.CompileDistinctValues(ast.FieldCameraMake)
	if err != nil {
		t.Fatalf("compile distinct: %v", err)
	}
	if !strings.Contains(statement.SQL, "SELECT DISTINCT") {
		t.Fatalf("expected SELECT DISTINCT, got: %s", statement.SQL)
	}
}

func TestCompileDistinctValues_NonSuggestable(t *testing.T) {
	_, err := ast.CompileDistinctValues(ast.FieldRating)
	if err == nil {
		t.Fatal("expected error for non-suggestable field")
	}
}

// IsDeleted is always injected.
func TestCompile_AlwaysExcludesDeleted(t *testing.T) {
	now := fixedNow()
	query := ast.Query{Version: ast.Version}
	statement, err := ast.CompileSelect(query, defaultArrangement(), ast.Page{Limit: 10}, now)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(statement.SQL, "is_deleted = 0") {
		t.Fatalf("missing is_deleted = 0: %s", statement.SQL)
	}
}

// MergeScope AND-composes two predicates.
func TestMergeScope(t *testing.T) {
	outer := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeFolder, VolumeID: "s1", Recursive: true},
		Where:   ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(3)},
	}
	stored := ast.Leaf{Field: ast.FieldFileType, Cmp: ast.OpIn, Value: []string{"raw"}}
	merged := ast.MergeScope(outer, stored)

	group, ok := merged.Where.(ast.Group)
	if !ok {
		t.Fatalf("expected Group, got %T", merged.Where)
	}
	if group.Op != ast.GroupAnd {
		t.Fatalf("expected AND, got %s", group.Op)
	}
	if len(group.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(group.Children))
	}
}

// Nesting semantics: (A OR B) AND NOT(C).
func TestCompile_NestingSemantics(t *testing.T) {
	now := fixedNow()
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Group{
			Op: ast.GroupAnd,
			Children: []ast.Node{
				ast.Group{
					Op: ast.GroupOr,
					Children: []ast.Node{
						ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpEq, Value: float64(5)},
						ast.Leaf{Field: ast.FieldFlag, Cmp: ast.OpIn, Value: []string{"pick"}},
					},
				},
				ast.Group{
					Op: ast.GroupNot,
					Children: []ast.Node{
						ast.Group{
							Op: ast.GroupAnd,
							Children: []ast.Node{
								ast.Leaf{Field: ast.FieldColorLabel, Cmp: ast.OpIn, Value: []string{"red"}},
							},
						},
					},
				},
			},
		},
	}
	statement, err := ast.CompileSelect(query, defaultArrangement(), ast.Page{Limit: 10}, now)
	if err != nil {
		t.Fatalf("compile nested: %v", err)
	}
	if !strings.Contains(statement.SQL, "OR") {
		t.Fatalf("expected OR in SQL: %s", statement.SQL)
	}
	if !strings.Contains(statement.SQL, "NOT") {
		t.Fatalf("expected NOT in SQL: %s", statement.SQL)
	}
}

// COALESCE fallback for captured_at sort.
func TestCompile_CapturedAtSortUsesCoalesce(t *testing.T) {
	now := fixedNow()
	query := ast.Query{Version: ast.Version}
	arrangement := ast.Arrangement{SortField: ast.SortCapturedAt, SortDir: ast.SortDesc}
	statement, err := ast.CompileSelect(query, arrangement, ast.Page{Limit: 10}, now)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(statement.SQL, "COALESCE(captured_at, mtime)") {
		t.Fatalf("expected COALESCE(captured_at, mtime) in ORDER BY: %s", statement.SQL)
	}
}

// Scope compilation.
func TestCompile_Scopes(t *testing.T) {
	now := fixedNow()
	cases := []struct {
		name  string
		scope ast.Scope
		want  string
	}{
		{"folder root recursive", ast.Scope{Kind: ast.ScopeFolder, VolumeID: "s1", Recursive: true}, "volume_id = ?"},
		{"collection", ast.Scope{Kind: ast.ScopeCollection, ID: "c1"}, "collection_assets"},
		{"tag", ast.Scope{Kind: ast.ScopeTag, ID: "t1"}, "asset_tags"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := ast.Query{Version: ast.Version, Scope: &tc.scope}
			statement, err := ast.CompileSelect(query, defaultArrangement(), ast.Page{Limit: 10}, now)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if !strings.Contains(statement.SQL, tc.want) {
				t.Fatalf("expected %q in SQL: %s", tc.want, statement.SQL)
			}
		})
	}
}

// Validation rejects structurally invalid trees.
func TestValidate_StructuralErrors(t *testing.T) {
	cases := []struct {
		name  string
		query ast.Query
	}{
		{"not with two children", ast.Query{
			Version: ast.Version,
			Where: ast.Group{Op: ast.GroupNot, Children: []ast.Node{
				ast.Group{Op: ast.GroupAnd, Children: []ast.Node{ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpEq, Value: float64(1)}}},
				ast.Group{Op: ast.GroupAnd, Children: []ast.Node{ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpEq, Value: float64(2)}}},
			}},
		}},
		{"not with leaf child", ast.Query{
			Version: ast.Version,
			Where: ast.Group{Op: ast.GroupNot, Children: []ast.Node{
				ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpEq, Value: float64(1)},
			}},
		}},
		{"empty and group", ast.Query{
			Version: ast.Version,
			Where:   ast.Group{Op: ast.GroupAnd, Children: []ast.Node{}},
		}},
		{"scope collection without ID", ast.Query{
			Version: ast.Version,
			Scope:   &ast.Scope{Kind: ast.ScopeCollection},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ast.Validate(tc.query); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// Dependency check is done via `go list` in the acceptance test script, but
// we can at least verify the package compiles without unexpected imports.
func TestCompile_LIKEEscaping(t *testing.T) {
	now := fixedNow()
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldFilename, Cmp: ast.OpContains, Value: "100%_done"},
	}
	statement, err := ast.CompileSelect(query, defaultArrangement(), ast.Page{Limit: 10}, now)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// The LIKE pattern must escape % and _.
	found := false
	for _, arg := range statement.Args {
		if s, ok := arg.(string); ok && strings.Contains(s, "\\%") && strings.Contains(s, "\\_") {
			found = true
		}
	}
	if !found {
		t.Fatalf("LIKE metacharacters not escaped in args: %v", statement.Args)
	}
}

// --- helpers ---

func extractOrderBy(sql string) string {
	idx := strings.Index(sql, "ORDER BY ")
	if idx < 0 {
		return ""
	}
	rest := sql[idx+len("ORDER BY "):]
	// Trim LIMIT clause if present.
	if limitIdx := strings.Index(rest, " LIMIT"); limitIdx >= 0 {
		rest = rest[:limitIdx]
	}
	return rest
}

func argsEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
