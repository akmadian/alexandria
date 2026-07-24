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

// ScopeKind identifies what the scope selects. The alphabet is the
// Scope vocabulary (C1, docs/frontend-architecture.md): library = everything; folder = a directory
// subtree within a volume; collection/tag = membership.
type ScopeKind string

const (
	ScopeLibrary    ScopeKind = "library"
	ScopeFolder     ScopeKind = "folder"
	ScopeCollection ScopeKind = "collection"
	ScopeTag        ScopeKind = "tag"
)

// Scope narrows the query to a specific container. Kind decides which fields
// are meaningful: collection/tag carry ID; folder carries VolumeID + Path
// (+ Recursive); library carries nothing.
type Scope struct {
	Kind ScopeKind
	ID   string // collection/tag only
	// Folder scope: the volume and the volume-relative directory path within it.
	// Path "" means the volume root. Recursive false = direct children only.
	VolumeID  string
	Path      string
	Recursive bool
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

// SortField names the logical sort axis. Members reuse the TokenField
// spellings (capturedAt, ingestedAt, …) so the sort vocabulary and the filter
// vocabulary are one language, not two.
type SortField string

const (
	SortCapturedAt SortField = "capturedAt"
	SortIngestedAt SortField = "ingestedAt"
	SortRating     SortField = "rating"
	SortFilename   SortField = "filename"
	SortSize       SortField = "size"
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
