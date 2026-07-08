// Package dependency supervises Alexandria's external command-line tools —
// discovering them, checking versions, and surviving their absence. Callers never
// touch os/exec; they ask this package to run a tool and get bytes or an error
// back, and they ask Status before offering a feature so a missing tool degrades
// gracefully rather than erroring (D5, impl/07).
//
// Scope note (ponytail): impl/07 designs a whole fleet — exiftool, ffmpeg/ffprobe,
// ghostscript, dcraw_emu — with user-consented downloads. This file ships only the
// exiftool descriptor and discovery, because impl/06 (XMP sync) is the only current
// consumer and no download flow exists yet. The other tools are one descriptor row
// each when their thumbnail features land; Fetch/checksum acquisition (NFR-6) is
// deferred until then. Extension is a data row, never new abstraction.
package dependency

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
)

// ToolID names an external tool. It is the key into the descriptor table.
type ToolID string

const Exiftool ToolID = "exiftool"

// Descriptor is the static knowledge about one tool: how to find it, how to ask
// its version, and the floor we require. Per-platform acquisition (download URL +
// pinned checksum) is intentionally absent until the consented-download flow ships.
type Descriptor struct {
	ID          ToolID
	Binaries    []string // candidate executable names, tried in order (PATH lookup)
	VersionArgs []string // args that print just the version, e.g. ["-ver"]
	MinVersion  string   // "major.minor" floor; "" means any version is accepted
}

// descriptors is the whole supported set — extension is adding a row here.
var descriptors = map[ToolID]Descriptor{
	Exiftool: {
		ID:          Exiftool,
		Binaries:    []string{"exiftool"},
		VersionArgs: []string{"-ver"},
		MinVersion:  "12.0", // stay_open + -@ predate this by years; a comfortable floor
	},
}

// State is the discovery verdict for a tool.
type State string

const (
	StateFound        State = "found"
	StateMissing      State = "missing"
	StateWrongVersion State = "wrong-version"
)

// Status is what a feature checks before offering itself. A StateMissing tool is
// not an error — the caller degrades (reports the feature unavailable) per D5.
type Status struct {
	State   State
	Path    string // resolved absolute path when found
	Version string // reported version when found
}

func (s Status) Available() bool { return s.State == StateFound }

// Discover resolves a tool: an explicit override path wins (machine.json's future
// job), otherwise PATH. It then runs the version check and compares against the
// descriptor floor. It never returns an error — the absence of a tool is a Status,
// not a failure (that is the whole point of the package).
//
// ponytail: the app-data tools dir tier (between override and PATH) is omitted —
// nothing writes to that dir until consented downloads exist, so it would only ever
// be empty. Add it in Fetch's change.
func Discover(id ToolID, override string) Status {
	descriptor, ok := descriptors[id]
	if !ok {
		return Status{State: StateMissing}
	}

	path := override
	if path == "" {
		for _, name := range descriptor.Binaries {
			if resolved, err := exec.LookPath(name); err == nil {
				path = resolved
				break
			}
		}
	}
	if path == "" {
		log.Debug("dependency: tool not found", "tool", id)
		return Status{State: StateMissing}
	}

	out, err := exec.Command(path, descriptor.VersionArgs...).Output()
	if err != nil {
		log.Warn("dependency: version check failed", "tool", id, "path", path, "err", err)
		return Status{State: StateMissing, Path: path}
	}
	version := strings.TrimSpace(string(out))

	if !versionAtLeast(version, descriptor.MinVersion) {
		log.Warn("dependency: tool below minimum version",
			"tool", id, "path", path, "version", version, "min", descriptor.MinVersion)
		return Status{State: StateWrongVersion, Path: path, Version: version}
	}

	log.Info("dependency: tool found", "tool", id, "path", path, "version", version)
	return Status{State: StateFound, Path: path, Version: version}
}

// versionAtLeast compares "major.minor" version strings numerically (exiftool
// reports "12.76"). An empty floor accepts anything; an unparseable version is
// treated as below the floor (fail closed).
func versionAtLeast(version, minimum string) bool {
	if minimum == "" {
		return true
	}
	haveMajor, haveMinor, ok := parseMajorMinor(version)
	if !ok {
		return false
	}
	wantMajor, wantMinor, ok := parseMajorMinor(minimum)
	if !ok {
		return true // an unparseable floor is a bug, not a reason to block the user
	}
	if haveMajor != wantMajor {
		return haveMajor > wantMajor
	}
	return haveMinor >= wantMinor
}

func parseMajorMinor(version string) (major, minor int, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(version), ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}
