package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/logging"
)

// SetupLogging routes the default logger to a fresh timestamped file under the
// app home's logs/ dir, tee'd with stderr, and (on platforms that have one)
// surfaces that dir in the OS log location so a native log reader can browse it.
// A Finder-launched .app has no console, so the file is the only place its logs
// survive. One file per run keeps each session self-contained.
//
// The file stays open for the process lifetime and is intentionally never
// closed: charmbracelet/log and os.File writes are unbuffered (each line is a
// direct syscall), so there is nothing to flush and the OS reclaims the fd at
// exit — which also sidesteps the "defer close never runs after log.Fatalf" trap.
//
// ponytail: per-run files, no pruning, Debug level. Add retention (keep the last
// N runs) and a settings-driven level with the impl/11 settings consumer + impl/12
// startup sequence. Richer filtering (`log stream`, structured Console
// predicates) is an os_log cgo bridge — deferred until flat files fall short.
func SetupLogging() error {
	dir, err := LogsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	name := fmt.Sprintf("alexandria-%s.log", time.Now().Format("2006-01-02-150405"))
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	log.SetDefault(logging.New(io.MultiWriter(os.Stderr, file)))
	log.Info("logging to file", "path", path)

	// Bridging the logs into the OS reader is a convenience, never load-bearing —
	// file logging already works, so a failure here is a warning, not fatal.
	if err := linkSystemLogDir(dir); err != nil {
		log.Warn("could not surface logs in the system log location", "err", err)
	}
	return nil
}

// symlinkForce makes link a symlink to target, replacing an existing symlink
// that points elsewhere. It is idempotent (an already-correct link is left
// alone) and safe (it refuses to touch link if a real file or directory sits
// there — never clobber user data). The platform log-dir bridge is built on it.
func symlinkForce(target, link string) error {
	info, err := os.Lstat(link)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		if current, _ := os.Readlink(link); current == target {
			return nil // already points where we want
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove stale symlink %s: %w", link, err)
		}
	case err == nil:
		return fmt.Errorf("%s exists and is not a symlink; leaving it untouched", link)
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}
	return nil
}
