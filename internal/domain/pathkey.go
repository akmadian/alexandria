package domain

import "golang.org/x/text/unicode/norm"

// PathKey returns the comparison form of a path or filename: Unicode NFC.
// macOS hands out decomposed names (NFD — "é" as 'e' + combining accent) while
// other sources compose them, so byte comparison misses visually identical
// names and mints phantom identities (the D20 trust-breaker).
//
// The rule is "compare keys, open bytes": PathKey is for equality, matching,
// and dedup ONLY. It is one-way — on-disk bytes are the truth for file I/O and
// must never be replaced by the normalized form (on Linux the original bytes
// are the only name that opens the file). No case folding: distinct-case names
// are genuinely distinct files on case-sensitive filesystems.
func PathKey(path string) string {
	return norm.NFC.String(path)
}
