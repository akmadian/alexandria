package ast

import (
	"fmt"
	"strings"

	"github.com/akmadian/alexandria/internal/domain"
)

// Structured validation errors so the seam can map them to codes, not strings.

type ErrUnknownField struct{ Field Field }

func (e *ErrUnknownField) Error() string { return fmt.Sprintf("unknown field %q", e.Field) }

type ErrInvalidOperator struct {
	Field    Field
	Operator Operator
}

func (e *ErrInvalidOperator) Error() string {
	return fmt.Sprintf("operator %q not allowed for field %q", e.Operator, e.Field)
}

type ErrInvalidValue struct {
	Field   Field
	Message string
}

func (e *ErrInvalidValue) Error() string {
	return fmt.Sprintf("invalid value for field %q: %s", e.Field, e.Message)
}

type ErrStructure struct{ Message string }

func (e *ErrStructure) Error() string { return fmt.Sprintf("structural error: %s", e.Message) }

const maxDepth = 16

// Validate checks a query for structural, grammar, and value correctness.
// Pure — no I/O, no DB.
func Validate(query Query) error {
	// Typed version errors so the seam can map them to stable codes (a too-new
	// query written by a newer app version vs. a structurally invalid one), rather
	// than an opaque string. UnmarshalJSON catches this at the wire too; Validate
	// also catches queries built as structs (not decoded from JSON).
	if query.Version > Version {
		return &ErrVersionTooNew{Got: query.Version, Want: Version}
	}
	if query.Version < 1 {
		return &ErrStructure{Message: fmt.Sprintf("invalid query version %d", query.Version)}
	}
	if query.Scope != nil {
		if err := validateScope(query.Scope); err != nil {
			return err
		}
	}
	if query.Where != nil {
		return validateNode(query.Where, 0)
	}
	return nil
}

func validateScope(scope *Scope) error {
	switch scope.Kind {
	case ScopeLibrary:
		if scope.ID != "" || scope.SourceID != "" || scope.Path != "" {
			return &ErrStructure{Message: "library scope carries no target"}
		}
		return nil
	case ScopeFolder:
		if scope.SourceID == "" {
			return &ErrStructure{Message: "folder scope requires a sourceId"}
		}
		if scope.ID != "" {
			return &ErrStructure{Message: "folder scope uses sourceId+path, not id"}
		}
		if strings.HasPrefix(scope.Path, "/") || strings.HasSuffix(scope.Path, "/") {
			return &ErrStructure{Message: fmt.Sprintf("folder path %q must be relative with no trailing slash", scope.Path)}
		}
		return nil
	case ScopeCollection, ScopeTag:
		if scope.ID == "" {
			return &ErrStructure{Message: fmt.Sprintf("scope %q requires an ID", scope.Kind)}
		}
		if scope.SourceID != "" || scope.Path != "" {
			return &ErrStructure{Message: fmt.Sprintf("scope %q carries only an id", scope.Kind)}
		}
		return nil
	default:
		return &ErrStructure{Message: fmt.Sprintf("unknown scope kind %q", scope.Kind)}
	}
}

func validateNode(node Node, depth int) error {
	if depth > maxDepth {
		return &ErrStructure{Message: fmt.Sprintf("predicate tree exceeds max depth %d", maxDepth)}
	}
	switch v := node.(type) {
	case Group:
		return validateGroup(v, depth)
	case Leaf:
		return validateLeaf(v)
	default:
		return &ErrStructure{Message: fmt.Sprintf("unknown node type %T", node)}
	}
}

func validateGroup(group Group, depth int) error {
	switch group.Op {
	case GroupNot:
		if len(group.Children) != 1 {
			return &ErrStructure{Message: fmt.Sprintf("not requires exactly 1 child, got %d", len(group.Children))}
		}
		// NOT's child must be a Group (leaf negation is an operator concern).
		if _, ok := group.Children[0].(Group); !ok {
			return &ErrStructure{Message: "not's child must be a group, not a leaf"}
		}
	case GroupAnd, GroupOr:
		if len(group.Children) == 0 {
			return &ErrStructure{Message: fmt.Sprintf("empty %s group", group.Op)}
		}
	default:
		return &ErrStructure{Message: fmt.Sprintf("unknown group operator %q", group.Op)}
	}
	for i, child := range group.Children {
		if err := validateNode(child, depth+1); err != nil {
			return fmt.Errorf("children[%d]: %w", i, err)
		}
	}
	return nil
}

func validateLeaf(leaf Leaf) error {
	spec, ok := LookupField(leaf.Field)
	if !ok {
		return &ErrUnknownField{Field: leaf.Field}
	}

	if !spec.allowsOperator(leaf.Cmp) {
		return &ErrInvalidOperator{Field: leaf.Field, Operator: leaf.Cmp}
	}

	return validateValue(leaf.Field, leaf.Cmp, leaf.Value, spec.Kind)
}

func validateValue(field Field, operator Operator, value any, kind ValueKind) error {
	// empty/notEmpty operators take no value.
	if operator == OpEmpty || operator == OpNotEmpty {
		if value != nil {
			return &ErrInvalidValue{Field: field, Message: "empty/notEmpty operators take no value"}
		}
		return nil
	}

	if value == nil {
		return &ErrInvalidValue{Field: field, Message: "value required"}
	}

	switch kind {
	case KindText:
		if _, ok := value.(string); !ok {
			return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("expected string, got %T", value)}
		}
	case KindNumeric:
		if !isNumeric(value) {
			return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("expected number, got %T", value)}
		}
	case KindEnum:
		return validateEnumValue(field, operator, value)
	case KindDateRange:
		dateValue, ok := value.(DateValue)
		if !ok {
			return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("expected DateValue, got %T", value)}
		}
		return validateDateValue(field, &dateValue)
	case KindTagReference:
		if _, ok := value.(string); !ok {
			return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("expected string (tag ID), got %T", value)}
		}
	case KindEntityReference:
		return validateEntityRefValue(field, operator, value)
	case KindFreeText:
		if _, ok := value.(string); !ok {
			return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("expected string, got %T", value)}
		}
	}
	return nil
}

// validateDateValue enforces the interval rules the wire codec also enforces,
// so DateValues built as Go structs (not decoded from JSON) meet the same bar:
// a non-zero duration whose components share one sign (the ISO wire form is a
// single signed magnitude — mixed signs have no representation).
func validateDateValue(field Field, value *DateValue) error {
	duration := value.Duration
	if duration.IsZero() {
		return &ErrInvalidValue{Field: field, Message: "date duration must be non-zero"}
	}
	positive := duration.Years > 0 || duration.Months > 0 || duration.Days > 0 ||
		duration.Hours > 0 || duration.Minutes > 0 || duration.Seconds > 0
	negative := duration.Years < 0 || duration.Months < 0 || duration.Days < 0 ||
		duration.Hours < 0 || duration.Minutes < 0 || duration.Seconds < 0
	if positive && negative {
		return &ErrInvalidValue{Field: field, Message: "date duration components must not mix signs"}
	}
	return nil
}

func isNumeric(v any) bool {
	switch v.(type) {
	case float64, int, int64:
		return true
	}
	return false
}

func validateEnumValue(field Field, operator Operator, value any) error {
	switch operator {
	case OpIn, OpNotIn:
		values, ok := value.([]string)
		if !ok {
			return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("in/notIn expects []string, got %T", value)}
		}
		for _, v := range values {
			if err := validateEnumMember(field, v); err != nil {
				return err
			}
		}
		return nil
	default:
		s, ok := value.(string)
		if !ok {
			return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("expected string, got %T", value)}
		}
		return validateEnumMember(field, s)
	}
}

// validateEnumMember checks that a string is a valid member of the field's
// domain enum. This and compile.go are the ONLY two places domain may appear.
func validateEnumMember(field Field, value string) error {
	switch field { //nolint:exhaustive // only enum fields have membership to validate
	case FieldFileType:
		switch domain.FileType(value) {
		case domain.FileTypeImage, domain.FileTypeVideo, domain.FileTypeRaw,
			domain.FileTypeVector, domain.FileTypeDocument, domain.FileTypeAudio:
			return nil
		}
		return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("unknown file type %q", value)}
	case FieldColorLabel:
		switch domain.ColorLabel(value) {
		case domain.ColorLabelRed, domain.ColorLabelOrange, domain.ColorLabelYellow,
			domain.ColorLabelGreen, domain.ColorLabelBlue, domain.ColorLabelPurple:
			return nil
		}
		return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("unknown color label %q", value)}
	case FieldFlag:
		switch domain.Flag(value) {
		case domain.FlagPick, domain.FlagReject:
			return nil
		}
		return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("unknown flag %q", value)}
	case FieldFileStatus:
		switch domain.FileStatus(value) {
		case domain.FileStatusOnline, domain.FileStatusOffline, domain.FileStatusMissing:
			return nil
		}
		return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("unknown file status %q", value)}
	}
	return nil
}

func validateEntityRefValue(field Field, operator Operator, value any) error {
	switch operator {
	case OpIn, OpNotIn:
		if _, ok := value.([]string); !ok {
			return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("in/notIn expects []string, got %T", value)}
		}
		return nil
	default:
		if _, ok := value.(string); !ok {
			return &ErrInvalidValue{Field: field, Message: fmt.Sprintf("expected string, got %T", value)}
		}
		return nil
	}
}
