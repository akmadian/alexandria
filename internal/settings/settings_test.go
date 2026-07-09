package settings

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

func testLogger() *log.Logger { return log.New(io.Discard) }

// waitFor polls cond until true or the deadline — hot-reload is async (watch +
// debounce), so tests can't assert synchronously.
func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	got := loadJSON(filepath.Join(t.TempDir(), "nope.json"), DefaultSettings(), testLogger())
	if !reflect.DeepEqual(got, DefaultSettings()) {
		t.Fatalf("missing file should yield defaults, got %+v", got)
	}
}

func TestLoadMalformedQuarantinesAndReverts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadJSON(path, DefaultSettings(), testLogger())
	if !reflect.DeepEqual(got, DefaultSettings()) {
		t.Fatalf("malformed file should revert to defaults, got %+v", got)
	}
	// Original content preserved as a sibling, not deleted.
	matches, _ := filepath.Glob(path + ".invalid-*")
	if len(matches) != 1 {
		t.Fatalf("expected exactly one quarantined sibling, found %d", len(matches))
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("original path should be gone (renamed to quarantine)")
	}
}

func TestBadFieldDoesNotZeroGoodFields(t *testing.T) {
	// Valid JSON, one bad numeric field — must clamp that field only.
	m := DefaultMachine()
	m.Workers.Ingest.Hash = -3 // bad
	m.Workers.Ingest.Extract = 5
	m.Workers.Ingest.Thumb = 6
	got := sanitizeMachine(m, testLogger())
	if got.Workers.Ingest.Hash != DefaultMachine().Workers.Ingest.Hash {
		t.Fatalf("bad hash count should clamp to default, got %d", got.Workers.Ingest.Hash)
	}
	if got.Workers.Ingest.Extract != 5 || got.Workers.Ingest.Thumb != 6 {
		t.Fatal("good fields must survive a single bad field")
	}
}

func TestSaveRoundTrips(t *testing.T) {
	for _, testCase := range []string{"settings", "machine", "keybindings"} {
		t.Run(testCase, func(t *testing.T) {
			dir := t.TempDir()
			switch testCase {
			case "settings":
				c, _ := OpenSettings(dir, testLogger())
				defer c.Close()
				want := DefaultSettings()
				want.ThumbnailQuality = 70
				want.IgnorePatterns = []string{"*.tmp", ".DS_Store"}
				if err := c.Save(want); err != nil {
					t.Fatal(err)
				}
				got := loadJSON(filepath.Join(dir, "settings.json"), DefaultSettings(), testLogger())
				if got.ThumbnailQuality != 70 || len(got.IgnorePatterns) != 2 {
					t.Fatalf("settings round-trip mismatch: %+v", got)
				}
			case "machine":
				c, _ := OpenMachine(dir, testLogger())
				defer c.Close()
				want := DefaultMachine()
				want.Workers.Ingest.Thumb = 8
				if err := c.Save(want); err != nil {
					t.Fatal(err)
				}
				got := loadJSON(filepath.Join(dir, "machine.json"), DefaultMachine(), testLogger())
				if got.Workers.Ingest.Thumb != 8 {
					t.Fatalf("machine round-trip mismatch: %+v", got)
				}
			case "keybindings":
				c, _ := OpenKeybindings(dir, testLogger())
				defer c.Close()
				if err := c.Save(Keybindings{"nextAsset": "j"}); err != nil {
					t.Fatal(err)
				}
				got := loadJSON(filepath.Join(dir, "keybindings.json"), Keybindings{}, testLogger())
				if got["nextAsset"] != "j" {
					t.Fatalf("keybindings round-trip mismatch: %+v", got)
				}
			}
		})
	}
}

func TestHotReloadValidExternalEdit(t *testing.T) {
	dir := t.TempDir()
	c, _ := OpenSettings(dir, testLogger())
	defer c.Close()

	fired := make(chan Settings, 1)
	c.OnChange(func(s Settings) { fired <- s })

	// External write (NOT through Save), valid and changed.
	external := DefaultSettings()
	external.ThumbnailQuality = 42
	writeJSON(t, filepath.Join(dir, "settings.json"), external)

	select {
	case s := <-fired:
		if s.ThumbnailQuality != 42 {
			t.Fatalf("OnChange fired with wrong value: %+v", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hot-reload did not fire OnChange for a valid external edit")
	}
	if c.Get().ThumbnailQuality != 42 {
		t.Fatal("cache not updated after external edit")
	}
}

func TestHotReloadInvalidKeepsPreviousAndDoesNotQuarantine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	c, _ := OpenSettings(dir, testLogger())
	defer c.Close()
	if err := c.Save(func() Settings { s := DefaultSettings(); s.ThumbnailQuality = 55; return s }()); err != nil {
		t.Fatal(err)
	}

	c.OnChange(func(Settings) { t.Error("OnChange must not fire for an invalid external edit") })

	if err := os.WriteFile(path, []byte("{ broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Give the watch+debounce time to (not) act.
	time.Sleep(reloadDebounce + 500*time.Millisecond)

	if c.Get().ThumbnailQuality != 55 {
		t.Fatal("live reload of invalid JSON must keep previous values")
	}
	if matches, _ := filepath.Glob(path + ".invalid-*"); len(matches) != 0 {
		t.Fatal("live reload must NOT quarantine — file left as the user has it")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("the (broken) file must remain readable/editable in place")
	}
}

func TestDebounceCollapsesRapidWrites(t *testing.T) {
	dir := t.TempDir()
	c, _ := OpenSettings(dir, testLogger())
	defer c.Close()

	var reloads atomic.Int32
	c.OnChange(func(Settings) { reloads.Add(1) })

	// Several rapid distinct writes within one debounce window.
	for i := 1; i <= 5; i++ {
		s := DefaultSettings()
		s.ThumbnailQuality = 10 + i
		writeJSON(t, filepath.Join(dir, "settings.json"), s)
		time.Sleep(30 * time.Millisecond)
	}
	waitFor(t, func() bool { return c.Get().ThumbnailQuality == 15 })
	// The burst collapsed: far fewer than 5 reloads (ideally 1).
	if n := reloads.Load(); n == 0 || n > 2 {
		t.Fatalf("expected the burst to debounce to ~1 reload, got %d", n)
	}
}

// writeJSON writes v as indented JSON directly (bypassing Save), simulating an
// external editor. Not atomic on purpose — exercises the raw-write path.
func writeJSON[T any](t *testing.T, path string, v T) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
