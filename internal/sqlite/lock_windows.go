//go:build windows

package sqlite

import "os"

// ponytail: Windows single-instance lock is a placeholder — it holds the lock
// file open but does not enforce exclusivity. Real locking needs LockFileEx
// (golang.org/x/sys/windows). Windows is third-priority; wire this when the
// Windows build is actually exercised.
type instanceLock struct{ f *os.File }

func acquireLock(path string) (*instanceLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return &instanceLock{f: f}, nil
}

func (l *instanceLock) release() error { return l.f.Close() }
