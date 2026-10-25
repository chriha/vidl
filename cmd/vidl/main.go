// Command vidl is a terminal UI for downloading videos with yt-dlp.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/config"
	"github.com/chriha/vidl/internal/ipc"
	"github.com/chriha/vidl/internal/notify"
	"github.com/chriha/vidl/internal/queue"
	"github.com/chriha/vidl/internal/ui"
	"github.com/chriha/vidl/internal/ytdlp"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage:
  vidl [url...]                 open the UI and queue the given URLs
  vidl add [--paused] <url>...  queue URLs in the running vidl; --paused adds
                                them paused, to start later with s

A terminal UI for downloading videos with yt-dlp.

Only one vidl runs at a time. While it runs, "vidl <url>" and "vidl add <url>"
hand the URLs to it over a local socket (vidl.sock in the config dir) and
exit. With no vidl running, "vidl add" appends the URLs to the saved queue,
and they start the next time vidl opens.

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
	if err := run(os.Args[1:], os.Stdout); err != nil {
		if errors.Is(err, ipc.ErrRunning) {
			fmt.Fprintln(os.Stderr, err) // "vidl is already running"
		} else {
			fmt.Fprintln(os.Stderr, "vidl:", err)
		}
		os.Exit(1)
	}
}

// run starts the UI or hands args to a running vidl; messages for the user
// go to out.
func run(args []string, out io.Writer) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "add" {
		return add(dir, args[1:], out)
	}
	store := queue.FileStore{Path: filepath.Join(dir, "queue.json")}
	lock, err := store.Lock()
	if errors.Is(err, queue.ErrLocked) {
		if len(args) == 0 {
			return ipc.ErrRunning
		}
		return forwardToLockHolder(dir, store, ipc.Request{Add: args}, out)
	}
	if err != nil {
		return fmt.Errorf("lock %s: %w", store.LockPath(), err)
	}
	return errors.Join(runUI(dir, store, args), lock.Release())
}

// runUI runs the UI while this process holds the queue lock, serving
// `vidl add` on the socket for its lifetime.
func runUI(dir string, store queue.FileStore, urls []string) error {
	var missing []string
	for _, bin := range []string{"yt-dlp", "ffmpeg"} {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%v not found on PATH; %s", missing, installHint(runtime.GOOS))
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
	mgr, err := queue.New(cfg, runner, store)
	if err != nil {
		return fmt.Errorf("loading queue: %w", err)
	}
	addStartURLs(mgr, urls)

	p := tea.NewProgram(ui.New(mgr, runner, cfgPath).WithVersionCheck(runner.Version).WithNotifier(notify.System{}))
	stopServer := serveAdds(dir, p.Send)
	_, runErr := p.Run()
	return errors.Join(runErr, stopServer(), mgr.Close())
}

// addStartURLs queues the URLs passed on the command line, skipping the ones
// already downloaded or queued, and returns the number skipped.
func addStartURLs(mgr *queue.Manager, urls []string) (skipped int) {
	var reqs []queue.NewItem
	for _, u := range urls {
		reqs = append(reqs, queue.NewItem{URL: u})
	}
	mgr.Add(reqs...)
	return 0
}

// serveAdds listens for `vidl add` and hands the URLs to the UI through
// send. A socket that can't be opened is reported in the UI, which runs
// anyway. The returned func stops the server.
func serveAdds(dir string, send func(tea.Msg)) (stop func() error) {
	srv, err := ipc.Listen(ipc.SocketPath(dir), forwardToUI(send, handoffTimeout))
	if err != nil {
		// send blocks until the UI loop runs, which starts after this returns.
		go send(ui.ErrorMsg{Err: fmt.Errorf("vidl add is unavailable: %w", err)})
		return func() error { return nil }
	}
	srv.OnError = func(err error) { send(ui.ErrorMsg{Err: fmt.Errorf("vidl add: %w", err)}) }
	done := make(chan struct{})
	go func() {
		srv.Serve()
		close(done)
	}()
	return func() error {
		err := srv.Close()
		<-done
		return err
	}
}

// handoffTimeout bounds how long the socket handler waits for the UI loop;
// it stays below ipc.DefaultTimeout so the client still gets the reply.
const handoffTimeout = ipc.DefaultTimeout / 2

// forwardToUI returns the socket handler of the running instance: it passes
// the request to the UI loop through send and waits up to timeout for the count.
// send returns once the UI took the message or the program has ended.
func forwardToUI(send func(tea.Msg), timeout time.Duration) ipc.Handler {
	return func(req ipc.Request) (int, error) {
		added := make(chan int, 1)
		go send(ui.AddURLsMsg{URLs: req.Add, Paused: req.Paused, Added: added})
		select {
		case n := <-added:
			return n, nil
		case <-time.After(timeout):
			return 0, errors.New("the vidl UI did not respond")
		}
	}
}
