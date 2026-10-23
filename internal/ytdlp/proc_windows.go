//go:build windows

package ytdlp

import (
	"os/exec"
	"strconv"
	"syscall"
)

// ctrlBreakEvent is CTRL_BREAK_EVENT; CTRL_C_EVENT can't target a process
// group other than the caller's.
const ctrlBreakEvent = 1

var generateConsoleCtrlEvent = syscall.NewLazyDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent")

// setInterruptible runs the command in its own process group and makes
// context cancellation send CTRL_BREAK to the whole group (yt-dlp and any
// ffmpeg children), which stops them without deleting partial files. Without
// a shared console the event can't be delivered, so the process tree is
// killed instead.
func setInterruptible(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		if ok, _, _ := generateConsoleCtrlEvent.Call(ctrlBreakEvent, uintptr(pid)); ok != 0 {
			return nil
		}
		if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run(); err == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
}
