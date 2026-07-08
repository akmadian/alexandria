package xmp

import (
	"context"
	"testing"

	"github.com/akmadian/alexandria/internal/dependency"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/charmbracelet/log"
)

// TestReadLightroomSidecar is the impl/06 acceptance round-trip (read half): an
// LrC-authored sidecar parses into rating/label/keywords/caption/title. It uses the
// real exiftool daemon, so it skips when exiftool isn't installed.
func TestReadLightroomSidecar(t *testing.T) {
	status := dependency.Discover(dependency.Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s)", status.State)
	}
	daemon, err := dependency.StartExiftool(status, log.Default())
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	fields, err := Read(context.Background(), daemon, "testdata/lightroom.xmp")
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}

	if fields.Rating == nil || *fields.Rating != 4 {
		t.Errorf("Rating = %v, want 4", fields.Rating)
	}
	if fields.Label != "Rot" {
		t.Errorf("Label = %q, want raw %q", fields.Label, "Rot")
	}
	if fields.Title != "Sunrise over Tokyo" {
		t.Errorf("Title = %q", fields.Title)
	}
	if fields.Caption != "Shot from the hotel roof at dawn." {
		t.Errorf("Caption = %q", fields.Caption)
	}
	if len(fields.Tags) != 3 || fields.Tags[0] != "Travel" || fields.Tags[2] != "Tokyo" {
		t.Errorf("Tags = %v, want [Travel Japan Tokyo]", fields.Tags)
	}
	if len(fields.Hierarchical) != 1 || fields.Hierarchical[0] != "Travel|Japan|Tokyo" {
		t.Errorf("Hierarchical = %v", fields.Hierarchical)
	}

	// The German label normalizes to canonical red; the raw string above is what
	// gets preserved for round-trip when a label is unknown.
	if label, ok := NormalizeLabel(fields.Label); !ok || label != domain.ColorLabelRed {
		t.Errorf("NormalizeLabel(%q) = %q, %v; want red, true", fields.Label, label, ok)
	}
}

func TestNormalizeLabel(t *testing.T) {
	cases := []struct {
		raw  string
		want domain.ColorLabel
		ok   bool
	}{
		{"Red", domain.ColorLabelRed, true},
		{"rot", domain.ColorLabelRed, true},       // German
		{"Rouge", domain.ColorLabelRed, true},     // French
		{"  BLEU  ", domain.ColorLabelBlue, true}, // French, padded + upper
		{"青", domain.ColorLabelBlue, true},        // Japanese
		{"viola", domain.ColorLabelPurple, true},  // Italian
		{"Krypton", "", false},                    // unknown → preserved by caller
		{"", "", false},                           // empty
	}
	for _, tc := range cases {
		label, ok := NormalizeLabel(tc.raw)
		if ok != tc.ok || label != tc.want {
			t.Errorf("NormalizeLabel(%q) = %q,%v; want %q,%v", tc.raw, label, ok, tc.want, tc.ok)
		}
	}
}

// TestDecodeExiftoolJSON checks the loose-typing coercions without a daemon:
// single-value list tags arrive as a bare string, ratings may be numeric strings,
// and leading warnings before the JSON array are tolerated.
func TestDecodeExiftoolJSON(t *testing.T) {
	raw := []byte(`Warning: minor issue
[{
  "SourceFile": "a.xmp",
  "Rating": "5",
  "Label": "Blue",
  "Subject": "Solo",
  "HierarchicalSubject": ["A|B", "C"],
  "Description": "cap"
}]`)
	fields, err := decodeExiftoolJSON(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fields.Rating == nil || *fields.Rating != 5 {
		t.Errorf("Rating = %v, want 5 (from numeric string)", fields.Rating)
	}
	if len(fields.Tags) != 1 || fields.Tags[0] != "Solo" {
		t.Errorf("Tags = %v, want [Solo] (single string coerced to list)", fields.Tags)
	}
	if len(fields.Hierarchical) != 2 {
		t.Errorf("Hierarchical = %v, want 2", fields.Hierarchical)
	}
}

func TestDecide(t *testing.T) {
	cases := []struct {
		name   string
		state  SyncState
		policy ConflictPolicy
		want   Action
	}{
		{"quiet", SyncState{}, PolicyXMPWins, ActionNoop},
		{"inbound", SyncState{SidecarChanged: true}, PolicyXMPWins, ActionApplyInbound},
		{"outbound-enabled", SyncState{CatalogChanged: true, WriteBackEnabled: true}, PolicyXMPWins, ActionWriteOutbound},
		{"outbound-disabled", SyncState{CatalogChanged: true}, PolicyXMPWins, ActionNoop},
		{"conflict-xmp", SyncState{SidecarChanged: true, CatalogChanged: true}, PolicyXMPWins, ActionConflict},
		{"conflict-catalog-writeback", SyncState{SidecarChanged: true, CatalogChanged: true, WriteBackEnabled: true}, PolicyCatalogWins, ActionConflict},
		{"conflict-catalog-no-writeback", SyncState{SidecarChanged: true, CatalogChanged: true}, PolicyCatalogWins, ActionNoop},
	}
	for _, tc := range cases {
		if got := Decide(tc.state, tc.policy); got != tc.want {
			t.Errorf("%s: Decide = %q, want %q", tc.name, got, tc.want)
		}
	}
}
