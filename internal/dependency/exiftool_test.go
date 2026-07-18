package dependency

import (
	"bufio"
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

// TestReadUntilMarker pins the transport's framing, especially the binary case:
// -b output (preview extraction) does not end with a newline, so the ready
// marker glues onto the payload's final bytes — the reader must match it as a
// suffix, split it off, and return the payload intact.
func TestReadUntilMarker(t *testing.T) {
	cases := []struct {
		name    string
		stream  string
		want    string
		wantErr bool
	}{
		{name: "marker on its own line", stream: "line one\nline two\n{ready1}\n", want: "line one\nline two\n"},
		{name: "marker glued to binary tail", stream: "\xff\xd8binary\xff\xd9{ready1}\n", want: "\xff\xd8binary\xff\xd9"},
		{name: "binary payload containing newlines", stream: "head\nmiddle\x00\ntail{ready1}\n", want: "head\nmiddle\x00\ntail"},
		{name: "empty payload", stream: "{ready1}\n", want: ""},
		{name: "eof before marker", stream: "partial output\n", want: "partial output\n", wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := readUntilMarker(bufio.NewReader(strings.NewReader(testCase.stream)), "{ready1}")
			if (err != nil) != testCase.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, testCase.wantErr)
			}
			if string(got) != testCase.want {
				t.Fatalf("payload = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestDiscover_MissingDegrades is the always-runnable half: a tool that isn't on
// PATH reports StateMissing without error, and StartExiftool refuses it. This is
// the D5 graceful-degradation contract, and it runs even on a machine with no
// exiftool installed.
func TestDiscover_MissingDegrades(t *testing.T) {
	status := Discover(ToolID("definitely-not-a-real-tool"), "")
	if status.State != StateMissing {
		t.Fatalf("unknown tool: want StateMissing, got %q", status.State)
	}
	if status.Available() {
		t.Fatal("missing tool must not report Available")
	}
	if _, err := StartExiftool(status, log.Default()); err == nil {
		t.Fatal("StartExiftool must refuse an unavailable tool")
	}
}

// TestExiftoolRoundTrip exercises the real daemon: version, then a metadata read.
// Skips when exiftool isn't installed — on this machine it validates the transport;
// in CI with exiftool present it guards the stay_open protocol against regressions.
func TestExiftoolRoundTrip(t *testing.T) {
	status := Discover(Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s) — skipping daemon round-trip", status.State)
	}

	daemon, err := StartExiftool(status, log.Default())
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	ctx := context.Background()

	// -ver over the daemon must match discovery's version.
	out, err := daemon.Execute(ctx, "-ver")
	if err != nil {
		t.Fatalf("execute -ver: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != status.Version {
		t.Fatalf("daemon -ver = %q, discovery = %q", got, status.Version)
	}

	// A second command on the same daemon must use a fresh marker and not bleed the
	// first response — reading its own file (JSON of this test file) proves both the
	// sequence counter and the merged-stream reader.
	out, err = daemon.Execute(ctx, "-json", "-FileType", "exiftool_test.go")
	if err != nil {
		t.Fatalf("execute -json: %v", err)
	}
	if !strings.Contains(string(out), "SourceFile") {
		t.Fatalf("expected JSON with SourceFile, got: %s", out)
	}
}
