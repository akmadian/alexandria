package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// linkSystemLogDir bridges the app-home logs/ dir into ~/Library/Logs, the
// location Console.app scans for its Log Reports. Because the one-directory app
// home lives at ~/.alexandria (not under ~/Library), Console would not find the
// logs on its own; a symlink at ~/Library/Logs/Alexandria makes them browsable
// there while keeping the single-directory layout.
func linkSystemLogDir(logsDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}
	link := filepath.Join(home, "Library", "Logs", "Alexandria")
	return symlinkForce(logsDir, link)
}
