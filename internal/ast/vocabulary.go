// The schema compiler (C15) projects this vocabulary into the generated TS
// and the data dictionary. Regenerate after any change here:
//go:generate go run github.com/akmadian/alexandria/cmd/generate -out ../../frontend/src/_generated-types -docs ../../docs

package ast

import "strings"

// Field identifies a filterable property of an asset. These generate the TS
// TokenField literal union via the schema generator (C13).
type Field string

const (
	FieldFilename    Field = "filename"
	FieldFileType    Field = "fileType"
	FieldRating      Field = "rating"
	FieldColorLabel  Field = "colorLabel"
	FieldFlag        Field = "flag"
	FieldTag         Field = "tag"
	FieldCapturedAt  Field = "capturedAt"
	FieldIngestedAt  Field = "ingestedAt"
	FieldSource      Field = "source"
	FieldWidth       Field = "width"
	FieldHeight      Field = "height"
	FieldCameraMake  Field = "cameraMake"
	FieldCameraModel Field = "cameraModel"
	FieldLensModel   Field = "lensModel"
	FieldTitle       Field = "title"
	FieldCaption     Field = "caption"
	FieldCreator     Field = "creator"
	FieldCopyright   Field = "copyright"
	FieldFileStatus  Field = "fileStatus"
	FieldText        Field = "text"
	// Cheap culling signals (task 20). Derived, nullable (NULL = not yet
	// computed). phash is intentionally NOT here — a scalar predicate over a
	// perceptual hash is meaningless; its near-dup query surface is deferred with
	// clustering (DEFERRED §12).
	FieldSharpness          Field = "sharpness"
	FieldClippingHighlights Field = "clippingHighlights"
	FieldClippingShadows    Field = "clippingShadows"
)

// Operator is a comparison operator in a predicate leaf. These generate the
// TS TokenOperator literal union (C13).
type Operator string

const (
	OpEq         Operator = "eq"
	OpNeq        Operator = "neq"
	OpGte        Operator = "gte"
	OpLte        Operator = "lte"
	OpIn         Operator = "in"
	OpNotIn      Operator = "notIn"
	OpContains   Operator = "contains"
	OpStartsWith Operator = "startsWith"
	OpHas        Operator = "has"
	OpLacks      Operator = "lacks"
	OpUnder      Operator = "under"
	OpNotUnder   Operator = "notUnder"
	OpWithin     Operator = "within"
	OpNotWithin  Operator = "notWithin"
	OpEmpty      Operator = "empty"
	OpNotEmpty   Operator = "notEmpty"
	OpMatches    Operator = "matches"
)

// ValueKind classifies the shape of a leaf's value. The kind is the load-bearing
// classification of the grammar: it decides which operator family a field
// supports, how its value validates, which editor the frontend renders, and
// which compile strategy produces its SQL.
type ValueKind string

const (
	KindText            ValueKind = "text"
	KindNumeric         ValueKind = "numeric"
	KindEnum            ValueKind = "enum"
	KindDateRange       ValueKind = "dateRange"
	KindTagReference    ValueKind = "tagReference"
	KindEntityReference ValueKind = "entityReference"
	KindFreeText        ValueKind = "freeText"
)

// kindOperators is the operator family each value kind supports — stated once.
// Fields never enumerate operators; they inherit their kind's family, plus the
// presence pair (empty/notEmpty) when the field is Nullable. Every map entry
// pairs with a compile strategy in compileLeaf's kind switch (exhaustive).
var kindOperators = map[ValueKind][]Operator{
	KindText:            {OpEq, OpNeq, OpContains, OpStartsWith},
	KindNumeric:         {OpEq, OpNeq, OpGte, OpLte},
	KindEnum:            {OpIn, OpNotIn},
	KindDateRange:       {OpWithin, OpNotWithin},
	KindTagReference:    {OpHas, OpLacks, OpUnder, OpNotUnder},
	KindEntityReference: {OpIn, OpNotIn},
	KindFreeText:        {OpMatches},
}

// FieldSpec is the whole query-layer truth about one field: the grammar half
// (Kind, Nullable → operators; value validation) and the storage half (column).
// One row per field, everything else derives.
type FieldSpec struct {
	Kind ValueKind
	// Nullable marks fields that can lack a value in the catalog (nullable
	// column, or an empty tag set). It appends the presence pair to the
	// operator family AND opts the field into the NULL-negation policy
	// (negation includes absent — see compile.go).
	Nullable bool
	// Suggestable marks fields whose distinct values can be queried for
	// autocomplete (CompileDistinctValues).
	Suggestable bool
	// columnOverride replaces the mechanical camelCase→snake_case column
	// derivation. Only `source` needs it (source → source_id). Virtual fields
	// (tag, text) set virtual instead and have no column.
	columnOverride string
	// virtual marks fields with no assets column: tag (junction table) and
	// text (FTS). They compile through dedicated strategies.
	virtual bool
}

// vocabulary is the single grammar authority: one FieldSpec per field. Adding
// a filterable capability = adding a row (C7); operators, column names, and
// the generated TS grammar all derive from it.
var vocabulary = map[Field]FieldSpec{
	FieldFilename:    {Kind: KindText},
	FieldFileType:    {Kind: KindEnum},
	FieldRating:      {Kind: KindNumeric, Nullable: true},
	FieldColorLabel:  {Kind: KindEnum, Nullable: true},
	FieldFlag:        {Kind: KindEnum, Nullable: true},
	FieldTag:         {Kind: KindTagReference, Nullable: true, virtual: true},
	FieldCapturedAt:  {Kind: KindDateRange, Nullable: true},
	FieldIngestedAt:  {Kind: KindDateRange},
	FieldSource:      {Kind: KindEntityReference, columnOverride: "source_id"},
	FieldWidth:       {Kind: KindNumeric, Nullable: true},
	FieldHeight:      {Kind: KindNumeric, Nullable: true},
	FieldCameraMake:  {Kind: KindText, Nullable: true, Suggestable: true},
	FieldCameraModel: {Kind: KindText, Nullable: true, Suggestable: true},
	FieldLensModel:   {Kind: KindText, Nullable: true, Suggestable: true},
	FieldTitle:       {Kind: KindText, Nullable: true},
	FieldCaption:     {Kind: KindText, Nullable: true},
	FieldCreator:     {Kind: KindText, Nullable: true, Suggestable: true},
	FieldCopyright:   {Kind: KindText, Nullable: true, Suggestable: true},
	FieldFileStatus:  {Kind: KindEnum},
	FieldText:        {Kind: KindFreeText, virtual: true},

	FieldSharpness:          {Kind: KindNumeric, Nullable: true},
	FieldClippingHighlights: {Kind: KindNumeric, Nullable: true},
	FieldClippingShadows:    {Kind: KindNumeric, Nullable: true},
}

// Operators derives the field's full operator set: its kind's family, plus the
// presence pair when the field can lack a value.
func (s FieldSpec) Operators() []Operator {
	family := kindOperators[s.Kind]
	if !s.Nullable {
		return family
	}
	operators := make([]Operator, 0, len(family)+2)
	operators = append(operators, family...)
	return append(operators, OpEmpty, OpNotEmpty)
}

// allowsOperator reports whether the operator is in the field's derived set.
func (s FieldSpec) allowsOperator(operator Operator) bool {
	for _, allowed := range s.Operators() {
		if allowed == operator {
			return true
		}
	}
	return false
}

// column returns the field's assets-table column, or "" for virtual fields.
func (s FieldSpec) column(field Field) string {
	if s.virtual {
		return ""
	}
	if s.columnOverride != "" {
		return s.columnOverride
	}
	return camelToSnake(string(field))
}

// camelToSnake converts a camelCase field name to its snake_case column name.
// Field names are ASCII camelCase by construction (they are the TokenField
// vocabulary), so a byte-wise conversion is sufficient.
func camelToSnake(name string) string {
	var out strings.Builder
	out.Grow(len(name) + 4)
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'A' && c <= 'Z' {
			out.WriteByte('_')
			out.WriteByte(c - 'A' + 'a')
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}

// LookupField returns the grammar spec for a field, or false if unknown.
func LookupField(field Field) (FieldSpec, bool) {
	spec, ok := vocabulary[field]
	return spec, ok
}

// AllFields returns every registered field. Used by completeness tests and
// the schema generator.
func AllFields() []Field {
	fields := make([]Field, 0, len(vocabulary))
	for field := range vocabulary {
		fields = append(fields, field)
	}
	return fields
}

// FieldColumns returns the assets-table column for every non-virtual field.
// Consumed by the crosswalk completeness suite (column ↔ schema) and the
// schema generator's data dictionary.
func FieldColumns() map[Field]string {
	columns := make(map[Field]string, len(vocabulary))
	for field, spec := range vocabulary {
		if column := spec.column(field); column != "" {
			columns[field] = column
		}
	}
	return columns
}
