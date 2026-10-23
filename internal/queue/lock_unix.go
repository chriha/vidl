//go:build !windows

package queue

import (
	"errors"
	"os"
	"syscall"
)

// acquireLock takes a non-blocking flock on path. The kernel drops it when
// the process exits, so a crash never leaves a stale lock. The file itself
// stays: removing it could let two processes lock different inodes.
func acquireLock(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return &Lock{f: f, path: path}, nil
}

func (l *Lock) release() error {
	return l.f.Close() // closing the last descriptor drops the flock
}
