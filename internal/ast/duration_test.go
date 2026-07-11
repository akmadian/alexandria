package ast_test

import (
	"strings"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
)

func TestParseISODuration_Valid(t *testing.T) {
	cases := []struct {
		input string
		want  ast.DateDuration
	}{
		{"P30D", ast.DateDuration{Days: 30}},
		{"-P30D", ast.DateDuration{Days: -30}},
		{"+P30D", ast.DateDuration{Days: 30}},
		{"P3M", ast.DateDuration{Months: 3}},
		{"P1Y2M3D", ast.DateDuration{Years: 1, Months: 2, Days: 3}},
		{"P2W", ast.DateDuration{Days: 14}},
		{"PT2H", ast.DateDuration{Hours: 2}},
		{"PT2H30M", ast.DateDuration{Hours: 2, Minutes: 30}},
		{"PT45S", ast.DateDuration{Seconds: 45}},
		{"-P1DT12H", ast.DateDuration{Days: -1, Hours: -12}},
		{"P1MT1M", ast.DateDuration{Months: 1, Minutes: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ast.ParseISODuration(tc.input)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("parse %q: got %+v, want %+v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseISODuration_Invalid(t *testing.T) {
	cases := []string{
		"",        // empty
		"30D",     // missing P
		"P",       // no components
		"PT",      // T with no time components
		"P1X",     // unknown designator
		"PD",      // designator with no digits
		"P1",      // digits with no designator
		"P1D2D",   // repeated designator
		"P1W2D",   // ISO week form combines with nothing
		"P1WT2H",  // week form with time part
		"P0D",     // zero-valued component
		"PT1D",    // date designator in time part
		"P1.5D",   // non-integer
		"- P30D",  // interior space
		"P30DT",   // trailing T
		"PT2H30X", // unknown time designator
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			if _, err := ast.ParseISODuration(input); err == nil {
				t.Fatalf("parse %q: expected error", input)
			}
		})
	}
}

func TestFormatISODuration_RoundTrip(t *testing.T) {
	cases := []ast.DateDuration{
		{Days: 30},
		{Days: -30},
		{Months: 3},
		{Years: 1, Months: 2, Days: 3},
		{Hours: 2, Minutes: 30},
		{Days: -1, Hours: -12},
		{Seconds: 45},
	}
	for _, duration := range cases {
		formatted := ast.FormatISODuration(duration)
		parsed, err := ast.ParseISODuration(formatted)
		if err != nil {
			t.Fatalf("round trip %+v via %q: %v", duration, formatted, err)
		}
		if parsed != duration {
			t.Fatalf("round trip %+v: formatted %q, parsed back %+v", duration, formatted, parsed)
		}
	}
}

func TestDateValueResolve_TimeComponents(t *testing.T) {
	now := time.Date(2026, 7, 10, 15, 0, 0, 0, time.UTC)
	value := ast.DateValue{
		Anchor:   ast.DateAnchor{Now: true},
		Duration: ast.DateDuration{Hours: -2, Minutes: -30},
	}
	start, end := value.Resolve(now)
	wantStart := time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(now) {
		t.Fatalf("resolve: got [%v, %v), want [%v, %v)", start, end, wantStart, now)
	}
}

func TestValidate_DateValueRules(t *testing.T) {
	leafWith := func(duration ast.DateDuration) ast.Query {
		return ast.Query{
			Version: ast.Version,
			Where: ast.Leaf{
				Field: ast.FieldCapturedAt,
				Cmp:   ast.OpWithin,
				Value: ast.DateValue{Anchor: ast.DateAnchor{Now: true}, Duration: duration},
			},
		}
	}
	if err := ast.Validate(leafWith(ast.DateDuration{})); err == nil {
		t.Fatal("zero duration: expected validation error")
	}
	if err := ast.Validate(leafWith(ast.DateDuration{Days: -1, Hours: 2})); err == nil {
		t.Fatal("mixed-sign duration: expected validation error")
	}
	if err := ast.Validate(leafWith(ast.DateDuration{Days: -1, Hours: -2})); err != nil {
		t.Fatalf("same-sign duration: unexpected error: %v", err)
	}
}

func TestJSON_DateValueWire(t *testing.T) {
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Leaf{
			Field: ast.FieldCapturedAt,
			Cmp:   ast.OpWithin,
			Value: ast.DateValue{Anchor: ast.DateAnchor{Now: true}, Duration: ast.DateDuration{Days: -30}},
		},
	}
	encoded, err := query.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	wire := string(encoded)
	if !strings.Contains(wire, `"anchor":"now"`) {
		t.Fatalf("wire missing symbolic now anchor: %s", wire)
	}
	if !strings.Contains(wire, `"duration":"-P30D"`) {
		t.Fatalf("wire missing ISO duration: %s", wire)
	}
}

func TestJSON_DateValueAnchors(t *testing.T) {
	decode := func(t *testing.T, anchor, duration string) (ast.Query, error) {
		t.Helper()
		var query ast.Query
		raw := `{"version":1,"where":{"field":"capturedAt","cmp":"within","value":{"anchor":"` +
			anchor + `","duration":"` + duration + `"}}}`
		err := query.UnmarshalJSON([]byte(raw))
		return query, err
	}

	if _, err := decode(t, "2026-07-01T10:30:00Z", "P1D"); err != nil {
		t.Fatalf("RFC3339 anchor: %v", err)
	}

	query, err := decode(t, "2026-07-01", "P1D")
	if err != nil {
		t.Fatalf("date-only anchor: %v", err)
	}
	leaf := query.Where.(ast.Leaf)
	anchor := leaf.Value.(ast.DateValue).Anchor.Date
	wantMidnight := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	if !anchor.Equal(wantMidnight) {
		t.Fatalf("date-only anchor: got %v, want local midnight %v", anchor, wantMidnight)
	}

	if _, err := decode(t, "yesterday", "P1D"); err == nil {
		t.Fatal("garbage anchor: expected error")
	}
	if _, err := decode(t, "now", "30 days"); err == nil {
		t.Fatal("garbage duration: expected error")
	}
}

func TestJSON_DateValueMissingAnchor(t *testing.T) {
	var query ast.Query
	raw := `{"version":1,"where":{"field":"capturedAt","cmp":"within","value":{"anchor":"","duration":"P1D"}}}`
	if err := query.UnmarshalJSON([]byte(raw)); err == nil {
		t.Fatal("empty anchor: expected error")
	}
}
