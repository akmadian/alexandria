package enrichment

import (
	"fmt"
	"strings"

	"github.com/akmadian/alexandria/internal/assettype"
)

// The registry is the whole graph (D28 commitment #1) — but it is stored FLAT,
// one row per kind, and the graph people want to see is a hierarchy per asset
// type (thumbnail → {sharpness, clipping, phash}). This file is the presentation
// over the flat rows (commitment #2): `cmd/dev jobs graph` renders it as DOT (for
// graphviz) and ASCII (for a terminal). Pure functions over the definitions and
// the assettype table — no engine, no I/O — so they render from Definitions(nil,
// nil) and test with fakes.

// graphGroup is one distinct sub-graph: the set of kinds that apply, plus the
// extensions (and their file-type labels) that share it. Asset types with the
// same applicable-kind set collapse into one group so the output is N distinct
// graphs, not one identical cluster per extension.
type graphGroup struct {
	extensions []string
	fileTypes  []string
	kinds      []string // in registry order
}

// groupByApplicability buckets asset types by their applicable-kind set. Types
// with no applicable kind (no thumbnail generator → no enrichment) are skipped.
func groupByApplicability(definitions []JobDefinition, handlers []assettype.Handler) []graphGroup {
	var groups []graphGroup
	bySignature := make(map[string]int) // signature → index into groups
	for _, handler := range handlers {
		var kinds []string
		for index := range definitions {
			if definitions[index].Applicable(handler) {
				kinds = append(kinds, definitions[index].Kind)
			}
		}
		if len(kinds) == 0 {
			continue
		}
		signature := strings.Join(kinds, "\x00")
		groupIndex, seen := bySignature[signature]
		if !seen {
			groupIndex = len(groups)
			bySignature[signature] = groupIndex
			groups = append(groups, graphGroup{kinds: kinds})
		}
		group := &groups[groupIndex]
		group.extensions = append(group.extensions, handler.Ext)
		group.fileTypes = appendDistinct(group.fileTypes, string(handler.Type))
	}
	return groups
}

// edges returns the group's prerequisite edges (prerequisite → dependent),
// restricted to kinds present in the group. roots returns the group's kinds with
// no in-group prerequisite (the graph's entry points).
func (g graphGroup) edges(definitions []JobDefinition) [][2]string {
	inGroup := make(map[string]bool, len(g.kinds))
	for _, kind := range g.kinds {
		inGroup[kind] = true
	}
	byKind := indexByKind(definitions)
	var out [][2]string
	for _, kind := range g.kinds {
		for _, prerequisite := range byKind[kind].Prerequisites {
			if inGroup[prerequisite] {
				out = append(out, [2]string{prerequisite, kind})
			}
		}
	}
	return out
}

func (g graphGroup) roots(definitions []JobDefinition) []string {
	inGroup := make(map[string]bool, len(g.kinds))
	for _, kind := range g.kinds {
		inGroup[kind] = true
	}
	byKind := indexByKind(definitions)
	var roots []string
	for _, kind := range g.kinds {
		hasParent := false
		for _, prerequisite := range byKind[kind].Prerequisites {
			if inGroup[prerequisite] {
				hasParent = true
				break
			}
		}
		if !hasParent {
			roots = append(roots, kind)
		}
	}
	return roots
}

// label names the group by its file types and covered extensions, e.g.
// "image, raw (jpg jpeg png cr2 …)".
func (g graphGroup) label() string {
	return fmt.Sprintf("%s (%s)", strings.Join(g.fileTypes, ", "), strings.Join(g.extensions, " "))
}

// RenderGraphDOT renders the registry as a graphviz digraph — one cluster per
// distinct sub-graph, node ids namespaced by group so shared kind names
// ("thumbnail") stay independent between clusters. Renders under `dot -Tsvg`
// without warnings.
func RenderGraphDOT(definitions []JobDefinition, handlers []assettype.Handler) string {
	var builder strings.Builder
	builder.WriteString("digraph enrichment {\n")
	builder.WriteString("  rankdir=LR;\n")
	builder.WriteString("  node [shape=box];\n")
	for groupIndex, group := range groupByApplicability(definitions, handlers) {
		fmt.Fprintf(&builder, "  subgraph cluster_%d {\n", groupIndex)
		fmt.Fprintf(&builder, "    label=%q;\n", group.label())
		for _, kind := range group.kinds {
			fmt.Fprintf(&builder, "    %q [label=%q];\n", nodeID(groupIndex, kind), kind)
		}
		for _, edge := range group.edges(definitions) {
			fmt.Fprintf(&builder, "    %q -> %q;\n", nodeID(groupIndex, edge[0]), nodeID(groupIndex, edge[1]))
		}
		builder.WriteString("  }\n")
	}
	builder.WriteString("}\n")
	return builder.String()
}

// RenderGraphASCII renders the registry as an indented tree per distinct
// sub-graph — the terminal fallback when graphviz is not installed.
func RenderGraphASCII(definitions []JobDefinition, handlers []assettype.Handler) string {
	dependents := dependentsByKind(definitions)
	var builder strings.Builder
	builder.WriteString("enrichment job graph\n")
	for _, group := range groupByApplicability(definitions, handlers) {
		fmt.Fprintf(&builder, "\n%s\n", group.label())
		inGroup := make(map[string]bool, len(group.kinds))
		for _, kind := range group.kinds {
			inGroup[kind] = true
		}
		for _, root := range group.roots(definitions) {
			builder.WriteString(root + "\n")
			writeASCIIChildren(&builder, root, "", inGroup, dependents)
		}
	}
	return builder.String()
}

// writeASCIIChildren prints a node's in-group dependents as a ├─/└─ tree; the
// caller has already printed the node's own name. prefix is the accumulated
// continuation columns of the ancestors. The registry graph is a small DAG; a
// kind reachable by two parents prints under each, which is fine for a
// legibility view. No cycle guard: the prerequisite graph is acyclic by
// construction — every engine build runs Validate (topo-sort, D28 commitment #3),
// so a cyclic registry fails the suite long before it could reach this renderer.
func writeASCIIChildren(builder *strings.Builder, kind, prefix string, inGroup map[string]bool, dependents map[string][]string) {
	var children []string
	for _, dependent := range dependents[kind] {
		if inGroup[dependent] {
			children = append(children, dependent)
		}
	}
	for index, child := range children {
		branch, spacer := "├─ ", "│  "
		if index == len(children)-1 {
			branch, spacer = "└─ ", "   "
		}
		fmt.Fprintf(builder, "%s%s%s\n", prefix, branch, child)
		writeASCIIChildren(builder, child, prefix+spacer, inGroup, dependents)
	}
}

func nodeID(groupIndex int, kind string) string { return fmt.Sprintf("g%d_%s", groupIndex, kind) }

func indexByKind(definitions []JobDefinition) map[string]*JobDefinition {
	byKind := make(map[string]*JobDefinition, len(definitions))
	for index := range definitions {
		byKind[definitions[index].Kind] = &definitions[index]
	}
	return byKind
}

// dependentsByKind is the reverse edge table (prerequisite → dependents), the
// same relation the engine builds at construction, recomputed here so the graph
// renderer stays a pure function of the definitions.
func dependentsByKind(definitions []JobDefinition) map[string][]string {
	dependents := make(map[string][]string, len(definitions))
	for index := range definitions {
		for _, prerequisite := range definitions[index].Prerequisites {
			dependents[prerequisite] = append(dependents[prerequisite], definitions[index].Kind)
		}
	}
	return dependents
}

func appendDistinct(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
