// Command vidl is a terminal UI for downloading videos with yt-dlp.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/config"
	"github.com/chriha/vidl/internal/queue"
	"github.com/chriha/vidl/internal/ui"
	"github.com/chriha/vidl/internal/ytdlp"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: vidl [url...]

A terminal UI for downloading videos with yt-dlp. URLs given as arguments
are added to the download queue.

Options:
  -h, --help     show this help
  -v, --version  print the version
`

func main() {
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "-h", "--help":
			fmt.Print(usage)
			return
		case "-v", "--version":
			info, ok := debug.ReadBuildInfo()
			fmt.Println("vidl", resolveVersion(version, info, ok))
			return
		}
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vidl:", err)
		os.Exit(1)
	}
}

func run(urls []string) error {
	var missing []string
	for _, bin := range []string{"yt-dlp", "ffmpeg"} {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%v not found on PATH; %s", missing, installHint(runtime.GOOS))
	}

	dir, err := config.Dir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "config.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", cfgPath, err)
	}
	if _, err := os.Stat(cfgPath); errors.Is(err, fs.ErrNotExist) {
		// Write the defaults so they're easy to discover and edit.
		if err := config.Save(cfgPath, cfg); err != nil {
			return err
		}
	}

	runner := ytdlp.Runner{}
	mgr, err := queue.New(cfg, runner, queue.FileStore{Path: filepath.Join(dir, "queue.json")})
	if err != nil {
		return fmt.Errorf("loading queue: %w", err)
	}
	var reqs []queue.NewItem
	for _, u := range urls {
		reqs = append(reqs, queue.NewItem{URL: u})
	}
	mgr.Add(reqs...)

	_, runErr := tea.NewProgram(ui.New(mgr, runner, cfgPath).WithVersionCheck(runner.Version)).Run()
	closeErr := mgr.Close()
	return errors.Join(runErr, closeErr)
}
