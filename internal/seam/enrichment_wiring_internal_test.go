package seam

import (
	"testing"

	"github.com/akmadian/alexandria/internal/enrichment"
)

// *enrichment.Engine is the production implementer of the seam's enrichment
// interfaces, but no host wires it yet (task 21 is contract + capability, the
// binding lands with the app host). These compile-time assertions are the only
// link keeping the engine's method set and these interfaces from drifting apart
// in the meantime — a rename on either side fails the build here, not silently at
// the future composition root.
var (
	_ enrichmentView       = (*enrichment.Engine)(nil)
	_ enrichmentController = (*enrichment.Engine)(nil)
)

// TestEnrichmentWiring_Compiles is a placeholder so the assertions above live in a
// test file (kept out of the production build) yet are still checked by go test /
// go vet. The assertions do the work; a failure is a compile error above this line.
func TestEnrichmentWiring_Compiles(_ *testing.T) {}
