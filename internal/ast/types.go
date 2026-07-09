package ast

// Version is the query AST version. Forward-only migrations ride catalog
// migrations; the JSON wire form always carries this so the caller can detect
// a query from a newer app version (C6).
const Version = 1

// Query is the complete query specification: scope (WHERE you're looking),
// predicate tree (WHAT you're filtering), arrangement (HOW it's ordered),
// and page (WHICH slice you want).
type Query struct {
	Version int
	Scope   *Scope // nil = everything
	Where   Node   // nil = no predicate (scope-only browse)
}

// ScopeKind identifies what the scope selects.
type ScopeKind string

const (
	ScopeAll        ScopeKind = "all"
	ScopeCollection ScopeKind = "collection"
	ScopeSource     ScopeKind = "source"
	ScopeTag        ScopeKind = "tag"
)

// Scope narrows the query to a specific container.
type Scope struct {
	Kind ScopeKind
	ID   string // empty for ScopeAll
}

// Node is the sealed predicate-tree interface. Only Group and Leaf implement
// it (unexported marker method) — the compiler's type switch is exhaustive by
// construction.
type Node interface{ isNode() }

// GroupOp is a boolean tree operator.
type GroupOp string

const (
	GroupAnd GroupOp = "and"
	GroupOr  GroupOp = "or"
	GroupNot GroupOp = "not"
)

// Group is a boolean combinator node in the predicate tree.
type Group struct {
	Op       GroupOp
	Children []Node
}

func (Group) isNode() {}

// Leaf is a single predicate: field × operator × value.
type Leaf struct {
	Field Field
	Cmp   Operator
	Value any // wire-typed: string | float64 | bool | []string | DateValue — validated per value kind
}

func (Leaf) isNode() {}

// SortField names the logical sort axis.
type SortField string

const (
	SortCaptured SortField = "captured"
	SortAdded    SortField = "added"
	SortRating   SortField = "rating"
	SortFilename SortField = "filename"
	SortSize     SortField = "size"
)

// SortDir is ascending or descending.
type SortDir string

const (
	SortAsc  SortDir = "asc"
	SortDesc SortDir = "desc"
)

// Arrangement controls order and (future) sectioning. C1/C4: arrangement
// decides order, never adds/removes assets.
type Arrangement struct {
	SortField SortField
	SortDir   SortDir
}

// Page is the offset/limit pagination slice.
type Page struct {
	Limit  int
	Offset int
}
