package importer

import "path"

// ignorePatterns are the baked-in ignore-list defaults (D18). Matched against a
// file or directory's base name via path.Match. A directory hit prunes the whole
// subtree at SCAN; a file hit is tallied (by pattern) and skipped without a row.
//
// ponytail: baked defaults only for v1. The live, per-catalog editable list
// lives in settings KV — wire it in when the settings service lands; this slice
// becomes the seed/reset-to-defaults value.
var ignorePatterns = []string{
	".DS_Store", "._*", ".Spotlight-V100", ".Trashes", ".fseventsd", // macOS
	"Thumbs.db", "desktop.ini", // Windows
	"@eaDir", ".AppleDouble", // NAS
	"*.tmp", "*.temp", "*.part", "*.crdownload", "*.download", // in-flight writes
}

// matchIgnore returns the pattern that name matched, or "" if none. Malformed
// patterns (path.Match only errors on bad syntax, which our constants never
// have) are treated as non-matches.
func matchIgnore(name string) string {
	for _, pattern := range ignorePatterns {
		if matched, err := path.Match(pattern, name); err == nil && matched {
			return pattern
		}
	}
	return ""
}
