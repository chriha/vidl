package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"vidl/internal/queue"
)

var errNoFile = errors.New("no file recorded for this download")

// opener opens path with its default application, or with reveal shows it in
// the file manager.
type opener func(path string, reveal bool) error

// openedMsg is the outcome of opening a file.
type openedMsg struct{ err error }

// systemOpen uses open on macOS and xdg-open elsewhere, which can't select a
// file, so it opens the containing folder to reveal one.
func systemOpen(path string, reveal bool) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("not an absolute path: %s", path)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if reveal {
			cmd = exec.Command("open", "-R", path)
		} else {
			cmd = exec.Command("open", path)
		}
	case "windows":
		return errors.New("opening files is not supported on Windows")
	default:
		if reveal {
			path = filepath.Dir(path)
		}
		cmd = exec.Command("xdg-open", path)
	}
	// No output capture: an application started by xdg-open could inherit the
	// pipe and keep Run waiting until it exits.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(cmd.Path), err)
	}
	return nil
}

// openSelected opens (or reveals) the file of the selected finished download.
func (m *Model) openSelected(reveal bool) tea.Cmd {
	if m.tab != tabFinished {
		return nil
	}
	it, ok := m.selected()
	if !ok || it.Status != queue.Finished {
		return nil
	}
	if it.FilePath == "" {
		m.report(errNoFile)
		return nil
	}
	if _, err := os.Stat(it.FilePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("file not found: %s", it.FilePath)
		}
		m.report(err)
		return nil
	}
	open, path := m.open, it.FilePath
	return func() tea.Msg { return openedMsg{open(path, reveal)} }
}
