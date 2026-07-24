package ast_test

import (
	"errors"
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
		for _, operator := range spec.Operators() {
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
		for _, op := range spec.Operators() {
			allowed[op] = true
		}
		for _, operator := range allOperators {
			if allowed[operator] {
				continue
			}
			value := syntheticValue(field, operator)
			query := ast.Query{
				Version: ast.Version,
				Where:   ast.Leaf{Field: field, Cmp: operator, Value: value},
			}
			if err := ast.Validate(query); err == nil {
				t.Errorf("field=%q op=%q: expected validation error for disallowed operator", field, operator)
			}
		}
	}
}

// The derived grammar, pinned field by field. This is the golden contract the
// generated TS grammar reflects: kind + nullability decide the operator set.
// A deliberate grammar change updates this table in the same commit; an
// accidental one fails here.
func TestDerivedGrammar_Golden(t *testing.T) {
	text := []ast.Operator{ast.OpEq, ast.OpNeq, ast.OpContains, ast.OpStartsWith}
	numeric := []ast.Operator{ast.OpEq, ast.OpNeq, ast.OpGte, ast.OpLte}
	enum := []ast.Operator{ast.OpIn, ast.OpNotIn}
	date := []ast.Operator{ast.OpWithin, ast.OpNotWithin}
	presence := []ast.Operator{ast.OpEmpty, ast.OpNotEmpty}
	withPresence := func(family []ast.Operator) []ast.Operator {
		return append(append([]ast.Operator{}, family...), presence...)
	}

	expected := map[ast.Field][]ast.Operator{
		ast.FieldFilename:    text,
		ast.FieldFileType:    enum,
		ast.FieldFileStatus:  enum,
		ast.FieldColorLabel:  withPresence(enum),
		ast.FieldFlag:        withPresence(enum),
		ast.FieldRating:      withPresence(numeric),
		ast.FieldWidth:       withPresence(numeric),
		ast.FieldHeight:      withPresence(numeric),
		ast.FieldCapturedAt:  withPresence(date),
		ast.FieldIngestedAt:  date,
		ast.FieldVolume:      enum,
		ast.FieldTag:         withPresence([]ast.Operator{ast.OpHas, ast.OpLacks, ast.OpUnder, ast.OpNotUnder}),
		ast.FieldText:        {ast.OpMatches},
		ast.FieldCameraMake:  withPresence(text),
		ast.FieldCameraModel: withPresence(text),
		ast.FieldLensModel:   withPresence(text),
		ast.FieldTitle:       withPresence(text),
		ast.FieldCaption:     withPresence(text),
		ast.FieldCreator:     withPresence(text),
		ast.FieldCopyright:   withPresence(text),

		ast.FieldSharpness:          withPresence(numeric),
		ast.FieldClippingHighlights: withPresence(numeric),
		ast.FieldClippingShadows:    withPresence(numeric),
	}

	fields := ast.AllFields()
	if len(fields) != len(expected) {
		t.Fatalf("vocabulary has %d fields, golden table has %d — update the golden", len(fields), len(expected))
	}
	for field, want := range expected {
		spec, ok := ast.LookupField(field)
		if !ok {
			t.Errorf("field %q missing from vocabulary", field)
			continue
		}
		got := spec.Operators()
		if len(got) != len(want) {
			t.Errorf("field %q: got operators %v, want %v", field, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("field %q: got operators %v, want %v", field, got, want)
				break
			}
		}
	}
}

// Column derivation: mechanical camelCase→snake_case with the one exception,
// and no column at all for virtual fields.
func TestFieldColumns(t *testing.T) {
	columns := ast.FieldColumns()
	cases := map[ast.Field]string{
		ast.FieldFilename:   "filename",
		ast.FieldFileType:   "file_type",
		ast.FieldCapturedAt: "captured_at",
		ast.FieldCameraMake: "camera_make",
		ast.FieldVolume:     "volume_id",
	}
	for field, want := range cases {
		if columns[field] != want {
			t.Errorf("field %q: column %q, want %q", field, columns[field], want)
		}
	}
	for _, virtual := range []ast.Field{ast.FieldTag, ast.FieldText} {
		if column, ok := columns[virtual]; ok {
			t.Errorf("virtual field %q must have no column, got %q", virtual, column)
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
	var target *ast.ErrUnknownField
	if !errors.As(err, &target) {
		t.Fatalf("expected ErrUnknownField, got %T: %v", err, err)
	}
}
