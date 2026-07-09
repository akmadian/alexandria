package ast_test

import (
	"testing"

	"github.com/akmadian/alexandria/internal/ast"
)

// Completeness (C10): every vocabulary field has a compiler entry, and every
// compiler entry has a vocabulary field. Adding a field without its compiler
// (or vice versa) fails CI.
func TestVocabularyCompilerCompleteness(t *testing.T) {
	fields := ast.AllFields()
	if len(fields) == 0 {
		t.Fatal("AllFields returned empty")
	}

	now := fixedNow()
	for _, field := range fields {
		spec, ok := ast.LookupField(field)
		if !ok {
			t.Errorf("field %q in AllFields but not in LookupField", field)
			continue
		}
		// Every field × every allowed operator must compile (not just validate).
		for _, operator := range spec.Operators {
			value := syntheticValue(field, operator)
			leaf := ast.Leaf{Field: field, Cmp: operator, Value: value}
			query := ast.Query{
				Version: ast.Version,
				Where:   leaf,
			}
			_, err := ast.CompileSelect(query, defaultArrangement(), ast.Page{Limit: 1}, now)
			if err != nil {
				t.Errorf("field=%q op=%q: compile failed: %v", field, operator, err)
			}
		}
	}
}

// Every field × a disallowed operator must fail validation.
func TestVocabularyDisallowedOperators(t *testing.T) {
	allOperators := []ast.Operator{
		ast.OpEq, ast.OpNeq, ast.OpGte, ast.OpLte,
		ast.OpIn, ast.OpNotIn, ast.OpContains, ast.OpStartsWith,
		ast.OpHas, ast.OpLacks, ast.OpUnder, ast.OpNotUnder,
		ast.OpWithin, ast.OpNotWithin, ast.OpEmpty, ast.OpNotEmpty,
		ast.OpMatches,
	}

	for _, field := range ast.AllFields() {
		spec, _ := ast.LookupField(field)
		allowed := make(map[ast.Operator]bool)
		for _, op := range spec.Operators {
			allowed[op] = true
		}
		for _, op := range allOperators {
			if allowed[op] {
				continue
			}
			value := syntheticValue(field, op)
			query := ast.Query{
				Version: ast.Version,
				Where:   ast.Leaf{Field: field, Cmp: op, Value: value},
			}
			if err := ast.Validate(query); err == nil {
				t.Errorf("field=%q op=%q: expected validation error for disallowed operator", field, op)
			}
		}
	}
}

func TestUnknownFieldValidation(t *testing.T) {
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: "nonexistent", Cmp: ast.OpEq, Value: "x"},
	}
	err := ast.Validate(query)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if _, ok := err.(*ast.ErrUnknownField); !ok {
		t.Fatalf("expected ErrUnknownField, got %T: %v", err, err)
	}
}
