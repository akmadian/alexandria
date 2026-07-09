package settings

import "path"

// The D18 ignore list lives in Settings.IgnorePatterns (seeded from the defaults
// in DefaultSettings on first run, then user-editable), so ALL the matching lives
// here too — settings is the single owner. Consumers (importer SCAN, watcher
// intake) hold a Settings value and call these methods directly; they carry no
// ignore logic of their own. There are TWO D18 chokepoints — SCAN (needs the
// matched pattern, via MatchIgnore, for the per-pattern skip tally) and the watcher
// intake (a bool, via Ignored, so a .tmp save-storm never even enters the debouncer).
//
// The zero Settings is the null object: no patterns → MatchIgnore returns "" for
// everything, so a bare consumer ignores nothing without any nil-guarding.

// MatchIgnore returns the ignore pattern that name (a base file/dir name) matches,
// or "" if none. A directory hit prunes the whole subtree at SCAN; a file hit is
// tallied by pattern and skipped without a row. Malformed patterns (path.Match
// only errors on bad syntax, which our defaults never have) are non-matches.
func (s *Settings) MatchIgnore(name string) string {
	for _, pattern := range s.IgnorePatterns {
		if matched, err := path.Match(pattern, name); err == nil && matched {
			return pattern
		}
	}
	return ""
}

// Ignored reports whether name matches the ignore list.
func (s *Settings) Ignored(name string) bool { return s.MatchIgnore(name) != "" }
