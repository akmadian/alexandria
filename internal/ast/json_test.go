package ast_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
)

func TestJSONRoundTrip_SimpleLeaf(t *testing.T) {
	original := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(3)},
	}
	assertRoundTrip(t, original)
}

func TestJSONRoundTrip_NestedTree(t *testing.T) {
	original := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeSource, ID: "src-1"},
		Where: ast.Group{
			Op: ast.GroupAnd,
			Children: []ast.Node{
				ast.Leaf{Field: ast.FieldFileType, Cmp: ast.OpIn, Value: []string{"raw", "image"}},
				ast.Group{
					Op: ast.GroupNot,
					Children: []ast.Node{
						ast.Group{
							Op: ast.GroupOr,
							Children: []ast.Node{
								ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpLte, Value: float64(2)},
								ast.Leaf{Field: ast.FieldFlag, Cmp: ast.OpIn, Value: []string{"reject"}},
							},
						},
					},
				},
			},
		},
	}
	assertRoundTrip(t, original)
}

func TestJSONRoundTrip_DateValue(t *testing.T) {
	original := ast.Query{
		Version: ast.Version,
		Where: ast.Leaf{
			Field: ast.FieldCapturedAt,
			Cmp:   ast.OpWithin,
			Value: ast.DateValue{
				Anchor:   ast.DateAnchor{Now: true},
				Duration: ast.DateDuration{Months: -3},
			},
		},
	}
	assertRoundTrip(t, original)
}

func TestJSONRoundTrip_ConcreteDate(t *testing.T) {
	anchor := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	original := ast.Query{
		Version: ast.Version,
		Where: ast.Leaf{
			Field: ast.FieldCapturedAt,
			Cmp:   ast.OpWithin,
			Value: ast.DateValue{
				Anchor:   ast.DateAnchor{Date: anchor},
				Duration: ast.DateDuration{Years: 1},
			},
		},
	}
	assertRoundTrip(t, original)
}

func TestJSONRoundTrip_ScopeOnly(t *testing.T) {
	original := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeCollection, ID: "col-1"},
	}
	assertRoundTrip(t, original)
}

func TestJSONRoundTrip_EmptyOperator(t *testing.T) {
	original := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpEmpty},
	}
	assertRoundTrip(t, original)
}

func TestJSON_RejectsVersionZero(t *testing.T) {
	data := `{"version":0}`
	var query ast.Query
	if err := json.Unmarshal([]byte(data), &query); err == nil {
		t.Fatal("expected error for version 0")
	}
}

func TestJSON_RejectsVersionTooNew(t *testing.T) {
	data := `{"version":999}`
	var query ast.Query
	err := json.Unmarshal([]byte(data), &query)
	if err == nil {
		t.Fatal("expected error for future version")
	}
	var versionErr *ast.ErrVersionTooNew
	if !errors.As(err, &versionErr) {
		t.Fatalf("expected ErrVersionTooNew, got %T: %v", err, err)
	}
}

func TestJSON_RejectsAmbiguousNode(t *testing.T) {
	data := `{"version":1,"where":{"op":"and","field":"rating","cmp":"eq","value":3,"children":[]}}`
	var query ast.Query
	if err := json.Unmarshal([]byte(data), &query); err == nil {
		t.Fatal("expected error for ambiguous node (both op and field)")
	}
}

func TestJSON_RejectsMissingNodeKeys(t *testing.T) {
	data := `{"version":1,"where":{"value":3}}`
	var query ast.Query
	if err := json.Unmarshal([]byte(data), &query); err == nil {
		t.Fatal("expected error for node missing both op and field")
	}
}

func TestJSON_RejectsUnknownKeys(t *testing.T) {
	data := `{"version":1,"where":{"field":"rating","cmp":"eq","value":3,"bogus":true}}`
	var query ast.Query
	if err := json.Unmarshal([]byte(data), &query); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func assertRoundTrip(t *testing.T, original ast.Query) {
	t.Helper()
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ast.Query
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(encoded) != string(reencoded) {
		t.Fatalf("round-trip mismatch:\n  original:  %s\n  roundtrip: %s", encoded, reencoded)
	}
}
