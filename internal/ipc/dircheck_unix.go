//go:build !windows

package ipc

import "os"

// currentUID is a seam so tests can simulate a directory owned by someone else.
var currentUID = os.Getuid

// dirOwnedByCurrentUser reports whether info belongs to the current user.
func dirOwnedByCurrentUser(info os.FileInfo) bool {
	return true
}
