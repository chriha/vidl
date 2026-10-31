package ipc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fallbackPrefix starts the name of the temp-dir fallback directory.
const fallbackPrefix = "vidl-"

// checkSocketDir refuses a socket path inside the temp-dir fallback
// directory unless that directory is a real directory owned by the current
// user with mode 0700. Other paths and missing directories pass.
func checkSocketDir(path string) error {
	dir := filepath.Dir(path)
	if filepath.Dir(dir) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(dir), fallbackPrefix) {
		return nil
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check socket dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("socket dir %s is not a directory", dir)
	}
	if !dirOwnedByCurrentUser(info) {
		return fmt.Errorf("socket dir %s is owned by another user", dir)
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("socket dir %s has mode %04o, want 0700", dir, info.Mode().Perm())
	}
	return nil
}
