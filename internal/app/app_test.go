package app_test

import (
	"os"
	"path/filepath"
	"testing"

	app "github.com/akmadian/alexandria/internal/app"
)

func TestHome_EnvOverride(t *testing.T) {
	t.Setenv("ALEXANDRIA_HOME", "/tmp/custom-home")
	got, err := app.Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if got != "/tmp/custom-home" {
		t.Fatalf("got %q, want /tmp/custom-home", got)
	}
}

func TestHome_DefaultUnderUserHome(t *testing.T) {
	t.Setenv("ALEXANDRIA_HOME", "")
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no user home dir: %v", err)
	}
	got, err := app.Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if want := filepath.Join(userHome, ".alexandria"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLogsAndCatalogDir_UnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ALEXANDRIA_HOME", home)
	t.Setenv("ALEXANDRIA_CATALOG", "")

	logs, err := app.LogsDir()
	if err != nil {
		t.Fatalf("LogsDir: %v", err)
	}
	if want := filepath.Join(home, "logs"); logs != want {
		t.Fatalf("LogsDir = %q, want %q", logs, want)
	}

	catalog, err := app.CatalogDir()
	if err != nil {
		t.Fatalf("CatalogDir: %v", err)
	}
	if want := filepath.Join(home, "catalog"); catalog != want {
		t.Fatalf("CatalogDir = %q, want %q", catalog, want)
	}
}

func TestCatalogDir_EnvOverrideWins(t *testing.T) {
	t.Setenv("ALEXANDRIA_HOME", t.TempDir())
	t.Setenv("ALEXANDRIA_CATALOG", "/mnt/photos/catalog")
	got, err := app.CatalogDir()
	if err != nil {
		t.Fatalf("CatalogDir: %v", err)
	}
	if got != "/mnt/photos/catalog" {
		t.Fatalf("got %q, want /mnt/photos/catalog", got)
	}
}
