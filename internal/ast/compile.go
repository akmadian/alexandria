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
	column := spec.column(field)
	if column == "" {
		return Statement{}, fmt.Errorf("field %q has no column (virtual)", field)
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
	thumbnail_at, relative_path, size_bytes,
	duration_secs, camera_model`

// --- Internals ---

// compileFullWhere produces the complete WHERE clause including is_deleted=0,
// scope, and predicate tree.
func compileFullWhere(query Query, now time.Time) (string, []any, error) {
	parts := []string{"is_deleted = 0"}
	var args []any

	if query.Scope != nil && query.Scope.Kind != ScopeLibrary {
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
	case ScopeFolder:
		return compileFolderScope(scope)
	case ScopeCollection:
		return "id IN (SELECT asset_id FROM collection_assets WHERE collection_id = ?)", []any{scope.ID}, nil
	case ScopeTag:
		return "id IN (SELECT at.asset_id FROM asset_tags at JOIN tags t ON t.id = at.tag_id WHERE t.path GLOB (SELECT path FROM tags WHERE id = ?) || '*' AND at.removed_at IS NULL)", []any{scope.ID}, nil
	default:
		return "", nil, fmt.Errorf("unresolved scope kind %q", scope.Kind)
	}
}

// compileFolderScope narrows to one directory within a source. Path "" is the
// source root. Non-recursive means direct children only: paths under the
// folder with no further separator.
func compileFolderScope(scope *Scope) (string, []any, error) {
	if scope.Path == "" {
		if scope.Recursive {
			return "source_id = ?", []any{scope.SourceID}, nil
		}
		return "source_id = ? AND relative_path NOT LIKE '%/%'", []any{scope.SourceID}, nil
	}
	prefix := escapeLike(scope.Path) + "/"
	if scope.Recursive {
		return "source_id = ? AND relative_path LIKE ? ESCAPE '\\'",
			[]any{scope.SourceID, prefix + "%"}, nil
	}
	return "source_id = ? AND relative_path LIKE ? ESCAPE '\\' AND relative_path NOT LIKE ? ESCAPE '\\'",
		[]any{scope.SourceID, prefix + "%", prefix + "%/%"}, nil
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
		// NULL-negation policy (D-log 2026-07-10): negation includes absent.
		// A positive predicate over a NULL column yields SQL NULL, and a bare
		// NOT would propagate it (excluding the row). ifnull(…, 0) makes the
		// child two-valued first, so NOT is a true set complement.
		return "NOT ifnull((" + childSQL + "), 0)", childArgs, nil
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

// --- Leaf compilation (kind-dispatched) ---

// compileLeaf routes a leaf to its kind's compile strategy. The vocabulary is
// the only authority: kind picks the strategy, the spec supplies the column
// and nullability. There is no per-field compiler list to drift.
func compileLeaf(leaf Leaf, now time.Time) (string, []any, error) {
	spec, ok := LookupField(leaf.Field)
	if !ok {
		return "", nil, fmt.Errorf("no vocabulary entry for field %q", leaf.Field)
	}
	column := spec.column(leaf.Field)
	switch spec.Kind {
	case KindText:
		return compileTextColumn(column, spec.Nullable, leaf)
	case KindNumeric:
		return compileNumericColumn(column, spec.Nullable, leaf)
	case KindEnum:
		return compileEnumColumn(column, spec.Nullable, leaf)
	case KindDateRange:
		return compileDateColumn(column, spec.Nullable, leaf, now)
	case KindEntityReference:
		return compileEntityColumn(column, leaf)
	case KindTagReference:
		return compileTag(leaf)
	case KindFreeText:
		return compileFreeText(leaf)
	default:
		return "", nil, fmt.Errorf("no compile strategy for value kind %q", spec.Kind)
	}
}

// negationIncludesAbsent applies the NULL-negation policy (D-log 2026-07-10)
// to a leaf-level negative operator on a nullable column: "not equal to x"
// includes rows where the value is absent. SQL's three-valued logic would
// silently exclude them (`col != ?` is NULL for NULL col).
func negationIncludesAbsent(column, predicate string, nullable bool) string {
	if !nullable {
		return predicate
	}
	return "(" + predicate + " OR " + column + " IS NULL)"
}

// --- Compile strategies (one per value kind) ---

func compileTextColumn(column string, nullable bool, leaf Leaf) (string, []any, error) {
	switch leaf.Cmp {
	case OpEq:
		return column + " = ?", []any{leaf.Value}, nil
	case OpNeq:
		return negationIncludesAbsent(column, column+" != ?", nullable), []any{leaf.Value}, nil
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

func compileNumericColumn(column string, nullable bool, leaf Leaf) (string, []any, error) {
	switch leaf.Cmp {
	case OpEq:
		return column + " = ?", []any{leaf.Value}, nil
	case OpNeq:
		return negationIncludesAbsent(column, column+" != ?", nullable), []any{leaf.Value}, nil
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

// inPlaceholders builds the "(?,?,…)" list and arg slice for IN/NOT IN.
func inPlaceholders(values []string) (string, []any) {
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		args[i] = v
	}
	return "(" + strings.Join(placeholders, ",") + ")", args
}

func compileEnumColumn(column string, nullable bool, leaf Leaf) (string, []any, error) {
	switch leaf.Cmp {
	case OpIn:
		list, args := inPlaceholders(leaf.Value.([]string))
		return column + " IN " + list, args, nil
	case OpNotIn:
		list, args := inPlaceholders(leaf.Value.([]string))
		return negationIncludesAbsent(column, column+" NOT IN "+list, nullable), args, nil
	case OpEmpty:
		return column + " IS NULL", nil, nil
	case OpNotEmpty:
		return column + " IS NOT NULL", nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported enum operator %q", leaf.Cmp)
	}
}

func compileEntityColumn(column string, leaf Leaf) (string, []any, error) {
	switch leaf.Cmp {
	case OpIn:
		list, args := inPlaceholders(leaf.Value.([]string))
		return column + " IN " + list, args, nil
	case OpNotIn:
		list, args := inPlaceholders(leaf.Value.([]string))
		return column + " NOT IN " + list, args, nil
	default:
		return "", nil, fmt.Errorf("unsupported entity operator %q", leaf.Cmp)
	}
}

func compileTag(leaf Leaf) (string, []any, error) {
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

func compileDateColumn(column string, nullable bool, leaf Leaf, now time.Time) (string, []any, error) {
	switch leaf.Cmp {
	case OpWithin:
		dateValue := leaf.Value.(DateValue)
		start, end := dateValue.Resolve(now)
		return "(" + column + " >= ? AND " + column + " < ?)",
			[]any{formatTime(start), formatTime(end)}, nil
	case OpNotWithin:
		dateValue := leaf.Value.(DateValue)
		start, end := dateValue.Resolve(now)
		return negationIncludesAbsent(column, "("+column+" < ? OR "+column+" >= ?)", nullable),
			[]any{formatTime(start), formatTime(end)}, nil
	case OpEmpty:
		return column + " IS NULL", nil, nil
	case OpNotEmpty:
		return column + " IS NOT NULL", nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported date operator %q", leaf.Cmp)
	}
}

func compileFreeText(leaf Leaf) (string, []any, error) {
	text := leaf.Value.(string)
	return "id IN (SELECT asset_id FROM assets_fts WHERE assets_fts MATCH ?)",
		[]any{quoteFTS(text)}, nil
}

// --- ORDER BY ---

var sortFieldToSQL = map[SortField]string{
	SortCapturedAt: "COALESCE(captured_at, mtime)",
	SortIngestedAt: "ingested_at",
	SortRating:     "rating",
	SortFilename:   "filename",
	SortSize:       "size_bytes",
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
	// The id tiebreaker is ALWAYS ascending, regardless of direction — the
	// seam contract (seam/01 §Additions #4) and the mock both promise
	// `ORDER BY <field> <dir>, id ASC`; tied rows must not reorder when the
	// user flips sort direction.
	return column + " " + dir + ", id ASC"
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
