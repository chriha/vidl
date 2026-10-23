//go:build windows

package ytdlp

import "os/exec"

// setInterruptible keeps the default behaviour (kill on cancel); Windows has
// no SIGINT equivalent for child processes.
func setInterruptible(cmd *exec.Cmd) {}
