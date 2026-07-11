package ast_test

// The NULL-negation policy (decision log, 2026-07-10): negation includes
// absent. "title neq x" matches untitled assets; "colorLabel notIn [red]"
// matches unlabeled ones; NOT groups are true set complements. SQL's
// three-valued logic would silently exclude NULL rows from every negative
// predicate — these tests pin the compiled forms that prevent that.

import (
	"strings"
	"testing"

	"github.com/akmadian/alexandria/internal/ast"
)

func compileLeafSQL(t *testing.T, leaf ast.Leaf) string {
	t.Helper()
	query := ast.Query{Version: ast.Version, Where: leaf}
	statement, err := ast.CompileWhere(query, nil, fixedNow())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return statement.SQL
}

func TestNullPolicy_NeqOnNullableIncludesAbsent(t *testing.T) {
	sql := compileLeafSQL(t, ast.Leaf{Field: ast.FieldTitle, Cmp: ast.OpNeq, Value: "x"})
	if !strings.Contains(sql, "(title != ? OR title IS NULL)") {
		t.Fatalf("nullable neq must include IS NULL arm: %s", sql)
	}
}

func TestNullPolicy_NeqOnNonNullableStaysPlain(t *testing.T) {
	sql := compileLeafSQL(t, ast.Leaf{Field: ast.FieldFilename, Cmp: ast.OpNeq, Value: "x"})
	if strings.Contains(sql, "filename IS NULL") {
		t.Fatalf("non-nullable neq must not carry a dead IS NULL arm: %s", sql)
	}
}

func TestNullPolicy_NotInOnNullableEnum(t *testing.T) {
	sql := compileLeafSQL(t, ast.Leaf{Field: ast.FieldColorLabel, Cmp: ast.OpNotIn, Value: []string{"red"}})
	if !strings.Contains(sql, "color_label IS NULL") {
		t.Fatalf("nullable notIn must include unlabeled rows: %s", sql)
	}
}

func TestNullPolicy_NotInOnNonNullableEnum(t *testing.T) {
	sql := compileLeafSQL(t, ast.Leaf{Field: ast.FieldFileType, Cmp: ast.OpNotIn, Value: []string{"raw"}})
	if strings.Contains(sql, "file_type IS NULL") {
		t.Fatalf("non-nullable notIn must stay plain: %s", sql)
	}
}

func TestNullPolicy_NotWithinOnNullableDate(t *testing.T) {
	value := ast.DateValue{Anchor: ast.DateAnchor{Now: true}, Duration: ast.DateDuration{Days: -30}}
	sql := compileLeafSQL(t, ast.Leaf{Field: ast.FieldCapturedAt, Cmp: ast.OpNotWithin, Value: value})
	if !strings.Contains(sql, "captured_at IS NULL") {
		t.Fatalf("notWithin on capturedAt must include undated rows: %s", sql)
	}
}

func TestNullPolicy_NotWithinOnNonNullableDate(t *testing.T) {
	value := ast.DateValue{Anchor: ast.DateAnchor{Now: true}, Duration: ast.DateDuration{Days: -30}}
	sql := compileLeafSQL(t, ast.Leaf{Field: ast.FieldIngestedAt, Cmp: ast.OpNotWithin, Value: value})
	if strings.Contains(sql, "ingested_at IS NULL") {
		t.Fatalf("notWithin on ingestedAt must stay plain: %s", sql)
	}
}

func TestNullPolicy_NotGroupIsSetComplement(t *testing.T) {
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Group{
			Op: ast.GroupNot,
			Children: []ast.Node{
				ast.Group{
					Op: ast.GroupAnd,
					Children: []ast.Node{
						ast.Leaf{Field: ast.FieldTitle, Cmp: ast.OpContains, Value: "x"},
					},
				},
			},
		},
	}
	statement, err := ast.CompileWhere(query, nil, fixedNow())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(statement.SQL, "NOT ifnull((") {
		t.Fatalf("NOT group must be two-valued via ifnull: %s", statement.SQL)
	}
}
