//go:build !darwin

package app

// linkSystemLogDir is a no-op off macOS: only macOS has a Console.app-scanned
// log location (~/Library/Logs) worth bridging the app-home logs/ dir into. The
// Linux/Windows equivalents (if any) land with their platform passes.
func linkSystemLogDir(string) error { return nil }
