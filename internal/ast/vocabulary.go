package ast

// Field identifies a filterable property of an asset. These generate the TS
// TokenField literal union via Wails bindings (C13).
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
)

// Operator is a comparison operator in a predicate leaf. These generate the
// TS TokenOperator literal union (C13).
type Operator string

const (
	OpEq       Operator = "eq"
	OpNeq      Operator = "neq"
	OpGte      Operator = "gte"
	OpLte      Operator = "lte"
	OpIn       Operator = "in"
	OpNotIn    Operator = "notIn"
	OpContains Operator = "contains"
	OpStartsWith Operator = "startsWith"
	OpHas      Operator = "has"
	OpLacks    Operator = "lacks"
	OpUnder    Operator = "under"
	OpNotUnder Operator = "notUnder"
	OpWithin   Operator = "within"
	OpNotWithin Operator = "notWithin"
	OpEmpty    Operator = "empty"
	OpNotEmpty Operator = "notEmpty"
	OpMatches  Operator = "matches"
)

// ValueKind classifies the shape of a leaf's value for validation.
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

// fieldSpec is the grammar half of a token definition: which operators a field
// allows and what value shape it expects. Compile strategies live in
// compile.go's registry, not here — vocabulary answers "is this leaf
// grammatical?", compile answers "what SQL is it?".
type fieldSpec struct {
	Operators []Operator
	Kind      ValueKind
	// Suggestable marks fields whose distinct values can be queried for
	// autocomplete (CompileDistinctValues).
	Suggestable bool
}

// vocabulary is the single grammar authority. Adding a field here without a
// matching compiler entry (or vice versa) fails the completeness test.
var vocabulary = map[Field]fieldSpec{
	FieldFilename: {
		Operators: []Operator{OpContains, OpStartsWith, OpEq, OpNeq},
		Kind:      KindText,
	},
	FieldFileType: {
		Operators: []Operator{OpIn, OpNotIn},
		Kind:      KindEnum,
	},
	FieldRating: {
		Operators: []Operator{OpEq, OpNeq, OpGte, OpLte, OpEmpty, OpNotEmpty},
		Kind:      KindNumeric,
	},
	FieldColorLabel: {
		Operators: []Operator{OpIn, OpNotIn, OpEmpty, OpNotEmpty},
		Kind:      KindEnum,
	},
	FieldFlag: {
		Operators: []Operator{OpIn, OpNotIn, OpEmpty, OpNotEmpty},
		Kind:      KindEnum,
	},
	FieldTag: {
		Operators: []Operator{OpHas, OpLacks, OpUnder, OpNotUnder, OpEmpty, OpNotEmpty},
		Kind:      KindTagReference,
	},
	FieldCapturedAt: {
		Operators: []Operator{OpWithin, OpNotWithin, OpEmpty, OpNotEmpty},
		Kind:      KindDateRange,
	},
	FieldIngestedAt: {
		Operators: []Operator{OpWithin, OpNotWithin},
		Kind:      KindDateRange,
	},
	FieldSource: {
		Operators: []Operator{OpIn, OpNotIn},
		Kind:      KindEntityReference,
	},
	FieldWidth: {
		Operators: []Operator{OpEq, OpGte, OpLte},
		Kind:      KindNumeric,
	},
	FieldHeight: {
		Operators: []Operator{OpEq, OpGte, OpLte},
		Kind:      KindNumeric,
	},
	FieldCameraMake: {
		Operators:   []Operator{OpEq, OpNeq, OpContains, OpEmpty, OpNotEmpty},
		Kind:        KindText,
		Suggestable: true,
	},
	FieldCameraModel: {
		Operators:   []Operator{OpEq, OpNeq, OpContains, OpEmpty, OpNotEmpty},
		Kind:        KindText,
		Suggestable: true,
	},
	FieldLensModel: {
		Operators:   []Operator{OpContains, OpStartsWith, OpEq, OpEmpty, OpNotEmpty},
		Kind:        KindText,
		Suggestable: true,
	},
	FieldTitle: {
		Operators: []Operator{OpContains, OpStartsWith, OpEq, OpEmpty, OpNotEmpty},
		Kind:      KindText,
	},
	FieldCaption: {
		Operators: []Operator{OpContains, OpStartsWith, OpEq, OpEmpty, OpNotEmpty},
		Kind:      KindText,
	},
	FieldCreator: {
		Operators:   []Operator{OpContains, OpStartsWith, OpEq, OpEmpty, OpNotEmpty},
		Kind:        KindText,
		Suggestable: true,
	},
	FieldCopyright: {
		Operators:   []Operator{OpContains, OpStartsWith, OpEq, OpEmpty, OpNotEmpty},
		Kind:        KindText,
		Suggestable: true,
	},
	FieldFileStatus: {
		Operators: []Operator{OpIn, OpNotIn},
		Kind:      KindEnum,
	},
	FieldText: {
		Operators: []Operator{OpMatches},
		Kind:      KindFreeText,
	},
}

// LookupField returns the grammar spec for a field, or false if unknown.
func LookupField(field Field) (fieldSpec, bool) {
	spec, ok := vocabulary[field]
	return spec, ok
}

// AllFields returns every registered field. Used by completeness tests and
// code generators.
func AllFields() []Field {
	fields := make([]Field, 0, len(vocabulary))
	for field := range vocabulary {
		fields = append(fields, field)
	}
	return fields
}
