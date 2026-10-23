package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/queue"
)

var errNoFile = errors.New("no file recorded for this download")

// opener opens path with its default application, or with reveal shows it in
// the file manager.
type opener func(path string, reveal bool) error

// openedMsg is the outcome of opening a file.
type openedMsg struct{ err error }

// openSpec is the command that opens or reveals a file.
type openSpec struct {
	name string
	args []string
	// cmdLine, when set, is the verbatim Windows command line used instead
	// of quoting args (see setCmdLine).
	cmdLine string
	// ignoreExit treats a non-zero exit status as success.
	ignoreExit bool
}

// openCommand chooses the command for goos: open on macOS, rundll32 and
// explorer on Windows, xdg-open elsewhere. xdg-open can't select a file, so
// it opens the containing folder to reveal one.
func openCommand(goos, path string, reveal bool) openSpec {
	switch goos {
	case "darwin":
		if reveal {
			return openSpec{name: "open", args: []string{"-R", path}}
		}
		return openSpec{name: "open", args: []string{path}}
	case "windows":
		if reveal {
			// Explorer misparses /select, when Go quotes the argument and
			// exits with status 1 even on success.
			return openSpec{
				name:       "explorer.exe",
				cmdLine:    `explorer.exe /select,"` + path + `"`,
				ignoreExit: true,
			}
		}
		// Unlike "cmd /c start", rundll32 doesn't re-parse the path.
		return openSpec{name: "rundll32.exe", args: []string{"url.dll,FileProtocolHandler", path}}
	default:
		if reveal {
			path = filepath.Dir(path)
		}
		return openSpec{name: "xdg-open", args: []string{path}}
	}
}

// systemOpen opens path with its default application, or with reveal shows it
// in the file manager.
func systemOpen(path string, reveal bool) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("not an absolute path: %s", path)
	}
	return run(openCommand(runtime.GOOS, path, reveal))
}

// run executes spec and waits for it to finish.
func run(spec openSpec) error {
	cmd := exec.Command(spec.name, spec.args...)
	if spec.cmdLine != "" {
		setCmdLine(cmd, spec.cmdLine)
	}
	// No output capture: an application started by xdg-open could inherit the
	// pipe and keep Run waiting until it exits.
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && spec.ignoreExit && errors.As(err, &exit) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", spec.name, err)
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

// browser opens url in the default web browser.
type browser func(url string) error

// errNotWebURL reports a URL that isn't an absolute http(s) URL.
func errNotWebURL(raw string) error { return fmt.Errorf("not a web URL: %s", raw) }

// webURL validates that raw is an absolute http or https URL with a host.
func webURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errNotWebURL(raw)
	}
	return nil
}

// systemBrowse opens an http(s) URL in the default web browser, passing it
// as a single argument and never through a shell.
func systemBrowse(raw string) error {
	if err := webURL(raw); err != nil {
		return err
	}
	return run(openCommand(runtime.GOOS, raw, false))
}

// browseSelected opens the source URL of the selected download, whatever its
// status. Marks are ignored.
func (m *Model) browseSelected() tea.Cmd {
	if m.tab != tabDownloads && m.tab != tabFinished {
		return nil
	}
	it, ok := m.selected()
	if !ok {
		return nil
	}
	if err := webURL(it.URL); err != nil {
		m.report(err)
		return nil
	}
	browse, raw := m.browse, it.URL
	return func() tea.Msg { return openedMsg{browse(raw)} }
}
