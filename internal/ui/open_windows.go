package ui

import (
	"os/exec"
	"syscall"
)

// setCmdLine makes cmd run with line verbatim instead of Go's argument quoting.
func setCmdLine(cmd *exec.Cmd, line string) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
}
