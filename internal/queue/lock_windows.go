//go:build windows

package queue

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// stillActive is the exit code GetExitCodeProcess reports for a running process.
const stillActive = 259

// acquireLock creates path exclusively and writes the process id into it. A
// lock file left behind by a process that is no longer running is removed
// and taken over.
func acquireLock(path string) (*Lock, error) {
	for attempt := 0; ; attempt++ {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if _, err := fmt.Fprint(f, os.Getpid()); err != nil {
				return nil, errors.Join(err, f.Close(), os.Remove(path))
			}
			return &Lock{f: f, path: path}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if attempt > 0 || holderAlive(path) {
			return nil, ErrLocked
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
}

// holderAlive reports whether the process named in the lock file runs. An
// unreadable or empty file counts as held: its owner may still be writing it.
func holderAlive(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return true
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED) // runs as another user
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return true
	}
	return code == stillActive
}

func (l *Lock) release() error {
	return errors.Join(l.f.Close(), os.Remove(l.path))
}
