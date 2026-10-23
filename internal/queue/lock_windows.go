//go:build windows

package queue

import (
	"errors"
	"os"
	"syscall"
)

// errSharingViolation is Windows' ERROR_SHARING_VIOLATION.
const errSharingViolation = syscall.Errno(32)

// acquireLock opens path without sharing, so no other process can open it
// while the handle is held. Windows closes the handle when the process
// exits, so a crash never leaves a stale lock.
func acquireLock(path string) (*Lock, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if errors.Is(err, errSharingViolation) {
		return nil, ErrLocked
	}
	if err != nil {
		return nil, err
	}
	return &Lock{f: os.NewFile(uintptr(h), path)}, nil
}

func (l *Lock) release() error {
	return l.f.Close()
}
