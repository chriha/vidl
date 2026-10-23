// Command vidl is a terminal UI for downloading videos with yt-dlp.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"vidl/internal/config"
	"vidl/internal/queue"
	"vidl/internal/ui"
	"vidl/internal/ytdlp"
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
			fmt.Println("vidl", version)
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
		return fmt.Errorf("%v not found on PATH; install with: brew install yt-dlp ffmpeg", missing)
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

	mgr, err := queue.New(cfg, ytdlp.Runner{}, queue.FileStore{Path: filepath.Join(dir, "queue.json")})
	if err != nil {
		return fmt.Errorf("loading queue: %w", err)
	}
	var reqs []queue.NewItem
	for _, u := range urls {
		reqs = append(reqs, queue.NewItem{URL: u})
	}
	mgr.Add(reqs...)

	_, runErr := tea.NewProgram(ui.New(mgr)).Run()
	closeErr := mgr.Close()
	return errors.Join(runErr, closeErr)
}
