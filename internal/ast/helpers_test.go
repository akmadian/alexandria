package ast_test

import (
	"time"

	"github.com/akmadian/alexandria/internal/ast"
)

func fixedNow() time.Time {
	return time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
}

func defaultArrangement() ast.Arrangement {
	return ast.Arrangement{SortField: ast.SortAdded, SortDir: ast.SortDesc}
}

// syntheticValue returns a type-correct dummy value for a given field/operator
// pair, used by completeness tests.
func syntheticValue(field ast.Field, operator ast.Operator) any {
	if operator == ast.OpEmpty || operator == ast.OpNotEmpty {
		return nil
	}

	spec, _ := ast.LookupField(field)

	switch spec.Kind {
	case ast.KindText, ast.KindFreeText:
		return "test"
	case ast.KindNumeric:
		return float64(1)
	case ast.KindEnum:
		return enumDefault(field, operator)
	case ast.KindDateRange:
		return ast.DateValue{
			Anchor:   ast.DateAnchor{Now: true},
			Duration: ast.DateDuration{Days: -30},
		}
	case ast.KindTagReference:
		return "tag-id-1"
	case ast.KindEntityReference:
		return entityDefault(operator)
	}
	return "fallback"
}

func enumDefault(field ast.Field, operator ast.Operator) any {
	var value string
	switch field {
	case ast.FieldFileType:
		value = "image"
	case ast.FieldColorLabel:
		value = "red"
	case ast.FieldFlag:
		value = "pick"
	case ast.FieldFileStatus:
		value = "online"
	default:
		value = "unknown"
	}
	if operator == ast.OpIn || operator == ast.OpNotIn {
		return []string{value}
	}
	return value
}

func entityDefault(operator ast.Operator) any {
	if operator == ast.OpIn || operator == ast.OpNotIn {
		return []string{"source-1"}
	}
	return "source-1"
}
