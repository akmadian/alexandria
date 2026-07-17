package main

import (
	"testing"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/enrichment"
)

// TestCellState pins the task-22 matrix cell precedence: n/a beats everything
// (unknown or inapplicable type), then running > done > failed > pending —
// running is happening now, and a present artifact always outranks a stale
// failure (D28: a failed state never outlives the artifact that supersedes it).
func TestCellState(t *testing.T) {
	applicable := &enrichment.JobDefinition{Kind: "kind", Applicable: func(assettype.Handler) bool { return true }}
	inapplicable := &enrichment.JobDefinition{Kind: "kind", Applicable: func(assettype.Handler) bool { return false }}

	testCases := []struct {
		name       string
		definition *enrichment.JobDefinition
		known      bool
		present    bool
		running    bool
		exhausted  bool
		want       string
	}{
		{"unknown type is na", applicable, false, true, true, true, "na"},
		{"inapplicable kind is na", inapplicable, true, true, true, true, "na"},
		{"running wins over everything", applicable, true, true, true, true, "running"},
		{"present artifact wins over failure", applicable, true, true, false, true, "done"},
		{"exhausted reads failed", applicable, true, false, false, true, "failed"},
		{"nothing yet reads pending", applicable, true, false, false, false, "pending"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := cellState(testCase.definition, assettype.Handler{}, testCase.known,
				testCase.present, testCase.running, testCase.exhausted)
			if got != testCase.want {
				t.Fatalf("cellState() = %q, want %q", got, testCase.want)
			}
		})
	}
}
