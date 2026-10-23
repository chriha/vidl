//go:build !windows

package ui

import "os/exec"

// setCmdLine is only needed on Windows; openCommand sets cmdLine only there.
func setCmdLine(*exec.Cmd, string) {}
