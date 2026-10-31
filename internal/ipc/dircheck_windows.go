//go:build windows

package ipc

import "os"

// dirOwnedByCurrentUser is always true: Windows has no Unix owner ids, and
// os.TempDir() is per-user there.
func dirOwnedByCurrentUser(info os.FileInfo) bool {
	return true
}
