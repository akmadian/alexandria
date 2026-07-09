package ast

import (
	"fmt"
	"strings"
	"time"
)

// Statement is the compiled SQL output: parameterized query text and its
// bound arguments. Values NEVER appear in SQL — only in Args.
type Statement struct {
	SQL  string
	Args []any
}

// --- Public compile family (all pure; `now` always a parameter) ---

// CompileSelect produces SELECT <AssetRow columns> … ORDER BY … LIMIT/OFFSET.
func CompileSelect(query Query, arrangement Arrangement, page Page, now time.Time) (Statement, error) {
	if err := Validate(query); err != nil {
		return Statement{}, err
	}
	where, args, err := compileFullWhere(query, now)
	if err != nil {
		return Statement{}, err
	}
	orderBy := compileOrderBy(arrangement)

	sql := "SELECT " + assetRowColumns + " FROM assets WHERE " + where + " ORDER BY " + orderBy
	if page.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", page.Limit)
		if page.Offset > 0 {
			sql += fmt.Sprintf(" OFFSET %d", page.Offset)
		}
	}
	return Statement{SQL: sql, Args: args}, nil
}

// CompileCount produces SELECT COUNT(*) for the total matching the query.
func CompileCount(query Query, now time.Time) (Statement, error) {
	if err := Validate(query); err != nil {
		return Statement{}, err
	}
	where, args, err := compileFullWhere(query, now)
	if err != nil {
		return Statement{}, err
	}
	return Statement{
		SQL:  "SELECT COUNT(*) FROM assets WHERE " + where,
		Args: args,
	}, nil
}

// CompileIDSlice produces an ids-only window over the compiled ordering
// (range-selection materialization).
func CompileIDSlice(query Query, arrangement Arrangement, fromIndex, toIndex int, now time.Time) (Statement, error) {
	if err := Validate(query); err != nil {
		return Statement{}, err
	}
	where, args, err := compileFullWhere(query, now)
	if err != nil {
		return Statement{}, err
	}
	orderBy := compileOrderBy(arrangement)

	limit := toIndex - fromIndex
	sql := "SELECT id FROM assets WHERE " + where + " ORDER BY " + orderBy +
		fmt.Sprintf(" LIMIT %d OFFSET %d", limit, fromIndex)
	return Statement{SQL: sql, Args: args}, nil
}

// CompileIndexOf produces the 0-based position of one asset in the compiled
// ordering. ROW_NUMBER() OVER (ORDER BY …) in a subquery, WHERE id = ?.
func CompileIndexOf(query Query, arrangement Arrangement, id string, now time.Time) (Statement, error) {
	if err := Validate(query); err != nil {
		return Statement{}, err
	}
	where, args, err := compileFullWhere(query, now)
	if err != nil {
		return Statement{}, err
	}
	orderBy := compileOrderBy(arrangement)

	sql := "SELECT position FROM (" +
		"SELECT id, ROW_NUMBER() OVER (ORDER BY " + orderBy + ") - 1 AS position " +
		"FROM assets WHERE " + where +
		") WHERE id = ?"
	args = append(args, id)
	return Statement{SQL: sql, Args: args}, nil
}

// CompileWhere produces a WHERE fragment + args for query-shaped update targets.
// exceptIDs compiles to AND id NOT IN (…) in the SAME statement.
func CompileWhere(query Query, exceptIDs []string, now time.Time) (Statement, error) {
	if err := Validate(query); err != nil {
		return Statement{}, err
	}
	where, args, err := compileFullWhere(query, now)
	if err != nil {
		return Statement{}, err
	}
	if len(exceptIDs) > 0 {
		placeholders := make([]string, len(exceptIDs))
		for i, id := range exceptIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where += " AND id NOT IN (" + strings.Join(placeholders, ",") + ")"
	}
	return Statement{SQL: where, Args: args}, nil
}

// CompileDistinctValues produces a SELECT DISTINCT for suggestable fields
// (autocomplete values).
func CompileDistinctValues(field Field) (Statement, error) {
	spec, ok := LookupField(field)
	if !ok {
		return Statement{}, &ErrUnknownField{Field: field}
	}
	if !spec.Suggestable {
		return Statement{}, fmt.Errorf("field %q is not suggestable", field)
	}
	column, ok := fieldToColumn[field]
	if !ok {
		return Statement{}, fmt.Errorf("no column mapping for field %q", field)
	}
	sql := fmt.Sprintf(
		"SELECT DISTINCT %s FROM assets WHERE is_deleted = 0 AND %s IS NOT NULL ORDER BY %s",
		column, column, column)
	return Statement{SQL: sql, Args: nil}, nil
}

// MergeScope composes a stored smart-collection query into an outer query by
// AND-ing their predicates. The caller resolves the smart collection's stored
// AST first, then merges it with the user's ad-hoc filter.
func MergeScope(outer Query, storedWhere Node) Query {
	merged := Query{
		Version: outer.Version,
		Scope:   outer.Scope,
	}
	switch {
	case outer.Where == nil && storedWhere == nil:
		// nothing
	case outer.Where == nil:
		merged.Where = storedWhere
	case storedWhere == nil:
		merged.Where = outer.Where
	default:
		merged.Where = Group{
			Op:       GroupAnd,
			Children: []Node{storedWhere, outer.Where},
		}
	}
	return merged
}

// --- Asset row columns (the slim grid-card projection) ---

const assetRowColumns = `id, source_id, filename, file_type, file_status,
	rating, color_label, flag,
	width, height, captured_at, ingested_at,
	thumbnail_at, relative_path, size_bytes`

// --- Internals ---

// compileFullWhere produces the complete WHERE clause including is_deleted=0,
// scope, and predicate tree.
func compileFullWhere(query Query, now time.Time) (string, []any, error) {
	parts := []string{"is_deleted = 0"}
	var args []any

	if query.Scope != nil && query.Scope.Kind != ScopeAll {
		scopeSQL, scopeArgs, err := compileScope(query.Scope)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, scopeSQL)
		args = append(args, scopeArgs...)
	}

	if query.Where != nil {
		nodeSQL, nodeArgs, err := compileNode(query.Where, now)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, nodeSQL)
		args = append(args, nodeArgs...)
	}

	return strings.Join(parts, " AND "), args, nil
}

func compileScope(scope *Scope) (string, []any, error) {
	switch scope.Kind {
	case ScopeSource:
		return "source_id = ?", []any{scope.ID}, nil
	case ScopeCollection:
		return "id IN (SELECT asset_id FROM collection_assets WHERE collection_id = ?)", []any{scope.ID}, nil
	case ScopeTag:
		return "id IN (SELECT at.asset_id FROM asset_tags at JOIN tags t ON t.id = at.tag_id WHERE t.path GLOB (SELECT path FROM tags WHERE id = ?) || '*' AND at.removed_at IS NULL)", []any{scope.ID}, nil
	default:
		return "", nil, fmt.Errorf("unresolved scope kind %q", scope.Kind)
	}
}

func compileNode(node Node, now time.Time) (string, []any, error) {
	switch v := node.(type) {
	case Group:
		return compileGroup(v, now)
	case Leaf:
		return compileLeaf(v, now)
	default:
		panic(fmt.Sprintf("unknown node type %T", node))
	}
}

func compileGroup(group Group, now time.Time) (string, []any, error) {
	switch group.Op {
	case GroupNot:
		childSQL, childArgs, err := compileNode(group.Children[0], now)
		if err != nil {
			return "", nil, err
		}
		return "NOT (" + childSQL + ")", childArgs, nil
	case GroupAnd, GroupOr:
		parts := make([]string, len(group.Children))
		var args []any
		for i, child := range group.Children {
			childSQL, childArgs, err := compileNode(child, now)
			if err != nil {
				return "", nil, err
			}
			parts[i] = childSQL
			args = append(args, childArgs...)
		}
		joiner := " AND "
		if group.Op == GroupOr {
			joiner = " OR "
		}
		return "(" + strings.Join(parts, joiner) + ")", args, nil
	default:
		return "", nil, fmt.Errorf("unknown group op %q", group.Op)
	}
}

// --- Per-field compiler registry (C10) ---

type fieldCompiler func(leaf Leaf, now time.Time) (string, []any, error)

var compilerRegistry = map[Field]fieldCompiler{
	FieldFilename:    compileTextColumn("filename"),
	FieldFileType:    compileEnumColumn("file_type"),
	FieldRating:      compileNumericColumn("rating"),
	FieldColorLabel:  compileEnumColumn("color_label"),
	FieldFlag:        compileEnumColumn("flag"),
	FieldTag:         compileTag,
	FieldCapturedAt:  compileDateColumn("captured_at"),
	FieldIngestedAt:  compileDateColumn("ingested_at"),
	FieldSource:      compileEntityColumn("source_id"),
	FieldWidth:       compileNumericColumn("width"),
	FieldHeight:      compileNumericColumn("height"),
	FieldCameraMake:  compileTextColumn("camera_make"),
	FieldCameraModel: compileTextColumn("camera_model"),
	FieldLensModel:   compileTextColumn("lens_model"),
	FieldTitle:       compileTextColumn("title"),
	FieldCaption:     compileTextColumn("caption"),
	FieldCreator:     compileTextColumn("creator"),
	FieldCopyright:   compileTextColumn("copyright"),
	FieldFileStatus:  compileEnumColumn("file_status"),
	FieldText:        compileFreeText,
}

// fieldToColumn maps fields to their SQL column name for direct-column
// operations (distinct values, etc.).
var fieldToColumn = map[Field]string{
	FieldFilename:    "filename",
	FieldFileType:    "file_type",
	FieldRating:      "rating",
	FieldColorLabel:  "color_label",
	FieldFlag:        "flag",
	FieldCapturedAt:  "captured_at",
	FieldIngestedAt:  "ingested_at",
	FieldSource:      "source_id",
	FieldWidth:       "width",
	FieldHeight:      "height",
	FieldCameraMake:  "camera_make",
	FieldCameraModel: "camera_model",
	FieldLensModel:   "lens_model",
	FieldTitle:       "title",
	FieldCaption:     "caption",
	FieldCreator:     "creator",
	FieldCopyright:   "copyright",
	FieldFileStatus:  "file_status",
}

func compileLeaf(leaf Leaf, now time.Time) (string, []any, error) {
	compiler, ok := compilerRegistry[leaf.Field]
	if !ok {
		return "", nil, fmt.Errorf("no compiler for field %q", leaf.Field)
	}
	return compiler(leaf, now)
}

// --- Compiler strategies ---

func compileTextColumn(column string) fieldCompiler {
	return func(leaf Leaf, _ time.Time) (string, []any, error) {
		switch leaf.Cmp {
		case OpEq:
			return column + " = ?", []any{leaf.Value}, nil
		case OpNeq:
			return column + " != ?", []any{leaf.Value}, nil
		case OpContains:
			return column + " LIKE ? ESCAPE '\\'", []any{"%" + escapeLike(leaf.Value.(string)) + "%"}, nil
		case OpStartsWith:
			return column + " LIKE ? ESCAPE '\\'", []any{escapeLike(leaf.Value.(string)) + "%"}, nil
		case OpEmpty:
			return column + " IS NULL", nil, nil
		case OpNotEmpty:
			return column + " IS NOT NULL", nil, nil
		default:
			return "", nil, fmt.Errorf("unsupported text operator %q", leaf.Cmp)
		}
	}
}

func compileNumericColumn(column string) fieldCompiler {
	return func(leaf Leaf, _ time.Time) (string, []any, error) {
		switch leaf.Cmp {
		case OpEq:
			return column + " = ?", []any{leaf.Value}, nil
		case OpNeq:
			return column + " != ?", []any{leaf.Value}, nil
		case OpGte:
			return column + " >= ?", []any{leaf.Value}, nil
		case OpLte:
			return column + " <= ?", []any{leaf.Value}, nil
		case OpEmpty:
			return column + " IS NULL", nil, nil
		case OpNotEmpty:
			return column + " IS NOT NULL", nil, nil
		default:
			return "", nil, fmt.Errorf("unsupported numeric operator %q", leaf.Cmp)
		}
	}
}

func compileEnumColumn(column string) fieldCompiler {
	return func(leaf Leaf, _ time.Time) (string, []any, error) {
		switch leaf.Cmp {
		case OpIn:
			values := leaf.Value.([]string)
			placeholders := make([]string, len(values))
			args := make([]any, len(values))
			for i, v := range values {
				placeholders[i] = "?"
				args[i] = v
			}
			return column + " IN (" + strings.Join(placeholders, ",") + ")", args, nil
		case OpNotIn:
			values := leaf.Value.([]string)
			placeholders := make([]string, len(values))
			args := make([]any, len(values))
			for i, v := range values {
				placeholders[i] = "?"
				args[i] = v
			}
			return "(" + column + " IS NULL OR " + column + " NOT IN (" + strings.Join(placeholders, ",") + "))", args, nil
		case OpEmpty:
			return column + " IS NULL", nil, nil
		case OpNotEmpty:
			return column + " IS NOT NULL", nil, nil
		default:
			return "", nil, fmt.Errorf("unsupported enum operator %q", leaf.Cmp)
		}
	}
}

func compileEntityColumn(column string) fieldCompiler {
	return func(leaf Leaf, _ time.Time) (string, []any, error) {
		switch leaf.Cmp {
		case OpIn:
			values := leaf.Value.([]string)
			placeholders := make([]string, len(values))
			args := make([]any, len(values))
			for i, v := range values {
				placeholders[i] = "?"
				args[i] = v
			}
			return column + " IN (" + strings.Join(placeholders, ",") + ")", args, nil
		case OpNotIn:
			values := leaf.Value.([]string)
			placeholders := make([]string, len(values))
			args := make([]any, len(values))
			for i, v := range values {
				placeholders[i] = "?"
				args[i] = v
			}
			return column + " NOT IN (" + strings.Join(placeholders, ",") + ")", args, nil
		default:
			return "", nil, fmt.Errorf("unsupported entity operator %q", leaf.Cmp)
		}
	}
}

func compileTag(leaf Leaf, _ time.Time) (string, []any, error) {
	switch leaf.Cmp {
	case OpHas:
		return "EXISTS (SELECT 1 FROM asset_tags WHERE asset_id = assets.id AND tag_id = ? AND removed_at IS NULL)", []any{leaf.Value}, nil
	case OpLacks:
		return "NOT EXISTS (SELECT 1 FROM asset_tags WHERE asset_id = assets.id AND tag_id = ? AND removed_at IS NULL)", []any{leaf.Value}, nil
	case OpUnder:
		return "EXISTS (SELECT 1 FROM asset_tags at JOIN tags t ON t.id = at.tag_id WHERE at.asset_id = assets.id AND t.path GLOB (SELECT path FROM tags WHERE id = ?) || '*' AND at.removed_at IS NULL)", []any{leaf.Value}, nil
	case OpNotUnder:
		return "NOT EXISTS (SELECT 1 FROM asset_tags at JOIN tags t ON t.id = at.tag_id WHERE at.asset_id = assets.id AND t.path GLOB (SELECT path FROM tags WHERE id = ?) || '*' AND at.removed_at IS NULL)", []any{leaf.Value}, nil
	case OpEmpty:
		return "NOT EXISTS (SELECT 1 FROM asset_tags WHERE asset_id = assets.id AND removed_at IS NULL)", nil, nil
	case OpNotEmpty:
		return "EXISTS (SELECT 1 FROM asset_tags WHERE asset_id = assets.id AND removed_at IS NULL)", nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported tag operator %q", leaf.Cmp)
	}
}

func compileDateColumn(column string) fieldCompiler {
	return func(leaf Leaf, now time.Time) (string, []any, error) {
		switch leaf.Cmp {
		case OpWithin:
			dateValue := leaf.Value.(DateValue)
			start, end := dateValue.Resolve(now)
			return column + " >= ? AND " + column + " < ?",
				[]any{formatTime(start), formatTime(end)}, nil
		case OpNotWithin:
			dateValue := leaf.Value.(DateValue)
			start, end := dateValue.Resolve(now)
			return "(" + column + " < ? OR " + column + " >= ?)",
				[]any{formatTime(start), formatTime(end)}, nil
		case OpEmpty:
			return column + " IS NULL", nil, nil
		case OpNotEmpty:
			return column + " IS NOT NULL", nil, nil
		default:
			return "", nil, fmt.Errorf("unsupported date operator %q", leaf.Cmp)
		}
	}
}

func compileFreeText(leaf Leaf, _ time.Time) (string, []any, error) {
	text := leaf.Value.(string)
	return "id IN (SELECT asset_id FROM assets_fts WHERE assets_fts MATCH ?)",
		[]any{quoteFTS(text)}, nil
}

// --- ORDER BY ---

var sortFieldToSQL = map[SortField]string{
	SortCaptured: "COALESCE(captured_at, mtime)",
	SortAdded:    "ingested_at",
	SortRating:   "rating",
	SortFilename: "filename",
	SortSize:     "size_bytes",
}

func compileOrderBy(arrangement Arrangement) string {
	column := sortFieldToSQL[arrangement.SortField]
	if column == "" {
		column = "ingested_at"
	}
	dir := "DESC"
	if arrangement.SortDir == SortAsc {
		dir = "ASC"
	}
	// Always append id as deterministic tiebreaker.
	return column + " " + dir + ", id " + dir
}

// --- Helpers ---

// escapeLike escapes LIKE metacharacters so user input is literal.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// quoteFTS wraps user input for FTS5 MATCH: double-quotes the entire input so
// metacharacters (*, ", AND, OR, NOT, NEAR) are literals. Internal
// double-quotes are escaped by doubling.
func quoteFTS(s string) string {
	escaped := strings.ReplaceAll(s, `"`, `""`)
	return `"` + escaped + `"`
}

// formatTime matches the sqlite package's RFC3339 format for date comparisons.
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
