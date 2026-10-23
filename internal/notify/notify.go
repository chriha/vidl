// Package notify sends desktop notifications through the platform's own
// notifier: osascript on macOS, notify-send on Linux and other Unixes, and
// PowerShell's built-in toast API on Windows.
package notify

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"
)

// ErrUnavailable means the platform's notifier binary is not installed.
var ErrUnavailable = errors.New("desktop notifier not found")

// Notifier sends one desktop notification.
type Notifier interface {
	Notify(title, body string) error
}

// Cmd is a notifier command line. Title and body only ever appear as whole
// arguments or environment values, never inside script text, because they
// come from the internet.
type Cmd struct {
	Name string
	Args []string
	Env  []string // extra KEY=VALUE pairs on top of the process environment
}

const (
	envTitle = "VIDL_NOTIFY_TITLE"
	envBody  = "VIDL_NOTIFY_BODY"
)

// macScript reads title and body from argv.
var macScript = []string{
	"on run argv",
	"display notification (item 2 of argv) with title (item 1 of argv)",
	"end run",
}

// windowsAppID is PowerShell's own AppUserModelID; vidl has none registered.
const windowsAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// windowsScript reads title and body from the environment and creates text
// nodes from them, so they are never parsed as script or XML.
var windowsScript = strings.Join([]string{
	"$ErrorActionPreference = 'Stop'",
	"$m = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]",
	"$x = $m::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)",
	"$t = $x.GetElementsByTagName('text')",
	"[void]$t.Item(0).AppendChild($x.CreateTextNode($env:" + envTitle + "))",
	"[void]$t.Item(1).AppendChild($x.CreateTextNode($env:" + envBody + "))",
	"$m::CreateToastNotifier('" + windowsAppID + "').Show([Windows.UI.Notifications.ToastNotification]::new($x))",
}, "\n")

// Build returns the notifier command for goos (a runtime.GOOS value).
func Build(goos, title, body string) Cmd {
	switch goos {
	case "darwin":
		var args []string
		for _, line := range macScript {
			args = append(args, "-e", line)
		}
		return Cmd{Name: "osascript", Args: append(args, "--", title, body)}
	case "windows":
		return Cmd{
			Name: "powershell",
			Args: []string{"-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(windowsScript)},
			Env:  []string{envTitle + "=" + title, envBody + "=" + body},
		}
	default:
		return Cmd{Name: "notify-send", Args: []string{"--app-name=vidl", "--", title, body}}
	}
}

// encodePowerShell encodes script for -EncodedCommand (base64 of UTF-16LE),
// which avoids Windows command-line quoting entirely.
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// System sends notifications with the current platform's notifier.
type System struct{}

// Notify shows a notification and waits for the notifier to exit. A missing
// notifier binary returns an error wrapping ErrUnavailable.
func (System) Notify(title, body string) error {
	c := Build(runtime.GOOS, title, body)
	path, err := exec.LookPath(c.Name)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnavailable, c.Name)
	}
	cmd := exec.Command(path, c.Args...)
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s: %w: %s", c.Name, err, msg)
		}
		return fmt.Errorf("%s: %w", c.Name, err)
	}
	return nil
}
