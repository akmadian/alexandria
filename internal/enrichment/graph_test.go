package enrichment_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/enrichment"
)

// TestRenderGraphDOT_RasterEdges: the raster sub-graph shows thumbnail feeding
// each cheap signal — the acceptance edge set (D28 commitment #2).
func TestRenderGraphDOT_RasterEdges(t *testing.T) {
	dot := enrichment.RenderGraphDOT(enrichment.Definitions(nil, nil), assettype.All())
	if !strings.HasPrefix(dot, "digraph enrichment {") {
		t.Fatalf("not a digraph:\n%s", dot)
	}
	for _, signal := range []string{"sharpness", "clipping", "phash"} {
		// nodes are namespaced by group (g<N>_<kind>); assert the thumbnail→signal
		// edge exists regardless of which group index the raster set landed in.
		if !strings.Contains(dot, "thumbnail\" -> \"g") || !strings.Contains(dot, "_"+signal+"\";") {
			t.Errorf("missing thumbnail → %s edge:\n%s", signal, dot)
		}
	}
	// The label carries the domain vocabulary, not a generic node dump.
	if !strings.Contains(dot, "image") || !strings.Contains(dot, "jpg") {
		t.Errorf("label missing file-type/extension vocabulary:\n%s", dot)
	}
}

// TestRenderGraphASCII_Tree: the terminal fallback roots at thumbnail and lists
// the signals as its children.
func TestRenderGraphASCII_Tree(t *testing.T) {
	ascii := enrichment.RenderGraphASCII(enrichment.Definitions(nil, nil), assettype.All())
	if !strings.Contains(ascii, "thumbnail") {
		t.Fatalf("no thumbnail root:\n%s", ascii)
	}
	for _, signal := range []string{"sharpness", "clipping", "phash"} {
		if !strings.Contains(ascii, signal) {
			t.Errorf("ascii missing %s:\n%s", signal, ascii)
		}
	}
}

// TestRenderGraphDOT_GraphvizClean pipes the DOT through `dot -Tsvg` and asserts
// it renders without warnings — the acceptance's real check. Skips when graphviz
// is not installed (CI without it), so the structural tests above stand alone.
func TestRenderGraphDOT_GraphvizClean(t *testing.T) {
	dotBinary, err := exec.LookPath("dot")
	if err != nil {
		t.Skip("graphviz `dot` not installed; skipping the render check")
	}
	dot := enrichment.RenderGraphDOT(enrichment.Definitions(nil, nil), assettype.All())
	command := exec.Command(dotBinary, "-Tsvg")
	command.Stdin = strings.NewReader(dot)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("dot -Tsvg failed: %v\nstderr: %s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("dot emitted warnings:\n%s", stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("<svg")) {
		t.Errorf("no svg output produced")
	}
}
