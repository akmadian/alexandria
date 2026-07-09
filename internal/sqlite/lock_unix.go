//go:build !windows

package sqlite

import (
	"errors"
	"os"
	"syscall"

	"github.com/akmadian/alexandria/internal/domain"
)

// instanceLock is an advisory whole-catalog lock. flock is tied to the open file
// description and released automatically when the process dies, so a crashed
// instance never leaves a stale lock (unlike an O_EXCL sentinel file).
type instanceLock struct{ f *os.File }

func acquireLock(path string) (*instanceLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, &domain.CatalogLockedError{Path: path}
		}
		return nil, err
	}
	return &instanceLock{f: file}, nil
}

func (l *instanceLock) release() error {
	// Best-effort unlock; closing the fd releases the lock regardless.
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
