// Package app resolves where Alexandria keeps its data, wires up logging, and
// carries the app host's webkit-free helpers (today: the asset-server thumbnail
// middleware, the seam's binary channel). The app home is one directory (default
// ~/.alexandria) holding the session logs and the default catalog — everything
// in one place, easy to find, copy, and back up. It is a LEAF; the app host and
// dev harness call in. Kept out of the root Wails package so it stays testable
// without the gtk/webkit toolchain.
package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// Home is the directory Alexandria owns for this user: session logs (logs/) and
// the default catalog (catalog/). Overridable via ALEXANDRIA_HOME; defaults to
// ~/.alexandria.
func Home() (string, error) {
	if home := os.Getenv("ALEXANDRIA_HOME"); home != "" {
		return home, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(userHome, ".alexandria"), nil
}

// LogsDir is where session logs go: <home>/logs. App-level (logs span catalogs),
// so it lives beside the default catalog, never inside one.
func LogsDir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "logs"), nil
}

// CatalogDir resolves the catalog to open. The catalog is a relocatable,
// self-contained directory (db + thumbnails + settings) — ALEXANDRIA_CATALOG
// points at one anywhere (e.g. beside the photos on an external drive); the
// default is <home>/catalog. impl/12 grows the picker + recent-catalog list.
func CatalogDir() (string, error) {
	if dir := os.Getenv("ALEXANDRIA_CATALOG"); dir != "" {
		return dir, nil
	}
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "catalog"), nil
}
