//go:build !windows

package ytdlp

import (
	"os/exec"
	"syscall"
)

// setInterruptible runs the command in its own process group and makes
// context cancellation send SIGINT to the whole group (yt-dlp and any ffmpeg
// children), giving yt-dlp a chance to exit cleanly.
func setInterruptible(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
}
