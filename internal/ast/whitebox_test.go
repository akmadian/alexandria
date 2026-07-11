package ast

// White-box coverage for branches the validated public API makes unreachable:
// Validate gates every Compile* entry, so the per-strategy unknown-operator
// defaults and the sealed-Node panic can only be exercised by calling the
// internals directly. Defensive code stays; this file proves it fires.

import (
	"strings"
	"testing"
	"time"
)

// fakeNode covers the sealed-interface default branches.
type fakeNode struct{}

func (fakeNode) isNode() {}

func TestCompileNode_UnknownTypePanics(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic for unknown node type")
		}
	}()
	_, _, _ = compileNode(fakeNode{}, time.Now())
}

func TestValidateNode_UnknownType(t *testing.T) {
	if err := validateNode(fakeNode{}, 0); err == nil {
		t.Fatal("expected structural error for unknown node type")
	}
}

func TestMarshalNode_UnknownType(t *testing.T) {
	if _, err := marshalNode(fakeNode{}); err == nil {
		t.Fatal("expected error for unknown node type")
	}
}

func TestStrategies_RejectForeignOperators(t *testing.T) {
	now := time.Now()
	badLeaf := Leaf{Field: FieldTitle, Cmp: OpHas, Value: "x"}

	if _, _, err := compileTextColumn("title", true, badLeaf); err == nil {
		t.Error("text strategy: expected unsupported-operator error")
	}
	if _, _, err := compileNumericColumn("rating", true, badLeaf); err == nil {
		t.Error("numeric strategy: expected unsupported-operator error")
	}
	if _, _, err := compileEnumColumn("flag", true, badLeaf); err == nil {
		t.Error("enum strategy: expected unsupported-operator error")
	}
	if _, _, err := compileEntityColumn("source_id", badLeaf); err == nil {
		t.Error("entity strategy: expected unsupported-operator error")
	}
	if _, _, err := compileTag(Leaf{Field: FieldTag, Cmp: OpEq, Value: "x"}); err == nil {
		t.Error("tag strategy: expected unsupported-operator error")
	}
	if _, _, err := compileDateColumn("captured_at", true, badLeaf, now); err == nil {
		t.Error("date strategy: expected unsupported-operator error")
	}
}

func TestCompileLeaf_UnknownFieldAndKind(t *testing.T) {
	now := time.Now()
	if _, _, err := compileLeaf(Leaf{Field: "bogus", Cmp: OpEq, Value: "x"}, now); err == nil {
		t.Error("expected error for unknown field")
	}

	// A spec with an unknown kind exercises the defensive default in the kind
	// switch. Injected temporarily — the real vocabulary never contains one.
	vocabulary["__fakeField"] = FieldSpec{Kind: ValueKind("mystery")}
	defer delete(vocabulary, "__fakeField")
	if _, _, err := compileLeaf(Leaf{Field: "__fakeField", Cmp: OpEq, Value: "x"}, now); err == nil {
		t.Error("expected error for unknown value kind")
	}
}

func TestCompileDistinctValues_VirtualSuggestable(t *testing.T) {
	// Defensive branch: a suggestable field with no column cannot produce a
	// DISTINCT statement. No real field is shaped this way; prove the guard.
	vocabulary["__fakeVirtual"] = FieldSpec{Kind: KindFreeText, Suggestable: true, virtual: true}
	defer delete(vocabulary, "__fakeVirtual")
	if _, err := CompileDistinctValues("__fakeVirtual"); err == nil ||
		!strings.Contains(err.Error(), "virtual") {
		t.Fatalf("expected virtual-field error, got %v", err)
	}
}

func TestErrVersionTooNew_Error(t *testing.T) {
	err := &ErrVersionTooNew{Got: 2, Want: 1}
	if !strings.Contains(err.Error(), "2") || !strings.Contains(err.Error(), "1") {
		t.Fatalf("unexpected message: %s", err.Error())
	}
}

func TestErrorMessages(t *testing.T) {
	for _, err := range []error{
		&ErrUnknownField{Field: "x"},
		&ErrInvalidOperator{Field: "x", Operator: OpEq},
		&ErrInvalidValue{Field: "x", Message: "m"},
		&ErrStructure{Message: "m"},
	} {
		if err.Error() == "" {
			t.Errorf("%T: empty message", err)
		}
	}
}

func TestNodeMarkers(t *testing.T) {
	// The sealed-interface marker methods are empty by design; invoke them so
	// the 100% gate reflects reality rather than excluding them.
	Group{}.isNode()
	Leaf{}.isNode()
}

func TestCompileScope_UnknownKindDefensive(t *testing.T) {
	if _, _, err := compileScope(&Scope{Kind: "bogus"}); err == nil {
		t.Fatal("expected error for unresolved scope kind")
	}
}

func TestCompileFullWhere_ScopeErrorPropagates(t *testing.T) {
	// Validate blocks unknown scope kinds at the public entries; the internal
	// propagation is proven directly.
	query := Query{Version: Version, Scope: &Scope{Kind: "bogus"}}
	if _, _, err := compileFullWhere(query, time.Now()); err == nil {
		t.Fatal("expected scope error to propagate")
	}
}

// mysteryField injects a spec that passes validation (valueless operator on a
// Nullable field) but fails compilation (unknown kind), proving the
// compile-error propagation through every public entry.
func withMysteryField(t *testing.T) Query {
	t.Helper()
	vocabulary["__mystery"] = FieldSpec{Kind: ValueKind("mystery"), Nullable: true}
	t.Cleanup(func() { delete(vocabulary, "__mystery") })
	return Query{Version: Version, Where: Leaf{Field: "__mystery", Cmp: OpEmpty, Value: nil}}
}

func TestCompileFamily_CompileErrorPropagates(t *testing.T) {
	query := withMysteryField(t)
	now := time.Now()
	arrangement := Arrangement{SortField: SortFilename, SortDir: SortAsc}

	if _, err := CompileSelect(query, arrangement, Page{Limit: 1}, now); err == nil {
		t.Error("CompileSelect: expected compile error")
	}
	if _, err := CompileCount(query, now); err == nil {
		t.Error("CompileCount: expected compile error")
	}
	if _, err := CompileIDSlice(query, arrangement, 0, 1, now); err == nil {
		t.Error("CompileIDSlice: expected compile error")
	}
	if _, err := CompileIndexOf(query, arrangement, "id", now); err == nil {
		t.Error("CompileIndexOf: expected compile error")
	}
	if _, err := CompileWhere(query, nil, now); err == nil {
		t.Error("CompileWhere: expected compile error")
	}

	// Nested inside a group, the same leaf proves compileGroup's propagation.
	nested := Query{Version: Version, Where: Group{Op: GroupAnd, Children: []Node{query.Where}}}
	if _, err := CompileWhere(nested, nil, now); err == nil {
		t.Error("nested: expected compile error")
	}
	negated := Query{Version: Version, Where: Group{Op: GroupNot, Children: []Node{Group{Op: GroupAnd, Children: []Node{query.Where}}}}}
	if _, err := CompileWhere(negated, nil, now); err == nil {
		t.Error("negated: expected compile error")
	}
}

func TestValidateValue_Shapes(t *testing.T) {
	cases := []struct {
		name    string
		leaf    Leaf
		wantErr bool
	}{
		{"empty with value", Leaf{Field: FieldTitle, Cmp: OpEmpty, Value: "x"}, true},
		{"value required", Leaf{Field: FieldTitle, Cmp: OpEq, Value: nil}, true},
		{"text wrong type", Leaf{Field: FieldTitle, Cmp: OpEq, Value: 3.0}, true},
		{"numeric wrong type", Leaf{Field: FieldRating, Cmp: OpEq, Value: "x"}, true},
		{"numeric int ok", Leaf{Field: FieldRating, Cmp: OpEq, Value: int(3)}, false},
		{"numeric int64 ok", Leaf{Field: FieldRating, Cmp: OpEq, Value: int64(3)}, false},
		{"tag wrong type", Leaf{Field: FieldTag, Cmp: OpHas, Value: 3.0}, true},
		{"freeText wrong type", Leaf{Field: FieldText, Cmp: OpMatches, Value: 3.0}, true},
		{"date wrong type", Leaf{Field: FieldCapturedAt, Cmp: OpWithin, Value: "yesterday"}, true},
		{"enum in wrong type", Leaf{Field: FieldFlag, Cmp: OpIn, Value: "pick"}, true},
		{"enum unknown member", Leaf{Field: FieldFlag, Cmp: OpIn, Value: []string{"maybe"}}, true},
		{"fileType unknown member", Leaf{Field: FieldFileType, Cmp: OpIn, Value: []string{"hologram"}}, true},
		{"colorLabel unknown member", Leaf{Field: FieldColorLabel, Cmp: OpIn, Value: []string{"mauve"}}, true},
		{"fileStatus unknown member", Leaf{Field: FieldFileStatus, Cmp: OpIn, Value: []string{"lost"}}, true},
		{"entity in wrong type", Leaf{Field: FieldSource, Cmp: OpIn, Value: "s1"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(Query{Version: Version, Where: tc.leaf})
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateEnumAndEntity_ScalarBranches(t *testing.T) {
	// The enum/entity operator families are in/notIn only, so the scalar
	// branches are unreachable through Validate — proven directly so the
	// defensive shape checks stay covered.
	if err := validateEnumValue(FieldFlag, OpEq, "pick"); err != nil {
		t.Errorf("scalar enum string: %v", err)
	}
	if err := validateEnumValue(FieldFlag, OpEq, 3.0); err == nil {
		t.Error("scalar enum non-string: expected error")
	}
	if err := validateEnumMember("__notAnEnumField", "anything"); err != nil {
		t.Errorf("non-enum field passes membership: %v", err)
	}
	if err := validateEntityRefValue(FieldSource, OpEq, "s1"); err != nil {
		t.Errorf("scalar entity string: %v", err)
	}
	if err := validateEntityRefValue(FieldSource, OpEq, 3.0); err == nil {
		t.Error("scalar entity non-string: expected error")
	}
}

func TestValidate_VersionAndDepth(t *testing.T) {
	if err := Validate(Query{Version: Version + 1}); err == nil {
		t.Error("expected version-too-new error")
	}

	// Exceed maxDepth with nested NOT(AND(…)) pairs.
	var node Node = Leaf{Field: FieldRating, Cmp: OpEq, Value: 3.0}
	for i := 0; i < maxDepth+2; i++ {
		node = Group{Op: GroupAnd, Children: []Node{node}}
	}
	if err := Validate(Query{Version: Version, Where: node}); err == nil {
		t.Error("expected depth error")
	}
}

func TestValidateGroup_Shapes(t *testing.T) {
	cases := []struct {
		name  string
		group Group
	}{
		{"not with two children", Group{Op: GroupNot, Children: []Node{Group{Op: GroupAnd, Children: []Node{Leaf{Field: FieldRating, Cmp: OpEq, Value: 3.0}}}, Leaf{Field: FieldRating, Cmp: OpEq, Value: 3.0}}}},
		{"not with leaf child", Group{Op: GroupNot, Children: []Node{Leaf{Field: FieldRating, Cmp: OpEq, Value: 3.0}}}},
		{"empty and", Group{Op: GroupAnd}},
		{"unknown op", Group{Op: "xor", Children: []Node{Leaf{Field: FieldRating, Cmp: OpEq, Value: 3.0}}}},
		{"bad child", Group{Op: GroupAnd, Children: []Node{Leaf{Field: "bogus", Cmp: OpEq, Value: "x"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(Query{Version: Version, Where: tc.group}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCompileGroup_UnknownOpDefensive(t *testing.T) {
	if _, _, err := compileGroup(Group{Op: "xor"}, time.Now()); err == nil {
		t.Fatal("expected error for unknown group op")
	}
}

func TestResolve_DateAnchor(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	value := DateValue{Anchor: DateAnchor{Date: anchor}, Duration: DateDuration{Days: 7}}
	start, end := value.Resolve(time.Now())
	if !start.Equal(anchor) || !end.Equal(anchor.AddDate(0, 0, 7)) {
		t.Fatalf("resolve: got [%v, %v)", start, end)
	}
}

func TestParseISODuration_Overflow(t *testing.T) {
	if _, err := ParseISODuration("P99999999999999999999D"); err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestCoerceDateValue_UnmarshalableInput(t *testing.T) {
	if _, err := coerceDateValue(make(chan int)); err == nil {
		t.Fatal("expected error for unmarshalable input")
	}
	if _, err := coerceDateValue("not an object"); err == nil {
		t.Fatal("expected error for non-object date value")
	}
}
