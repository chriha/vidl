package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/chriha/vidl/internal/config"
	"github.com/chriha/vidl/internal/ipc"
	"github.com/chriha/vidl/internal/queue"
)

// add implements `vidl add`: it hands urls to the running vidl, or appends
// them to queue.json when none runs.
func add(dir string, urls []string, out io.Writer) error {
	if len(urls) == 0 {
		return errors.New("usage: vidl add <url> [<url>...]")
	}
	err := forward(dir, urls, out)
	if !errors.Is(err, ipc.ErrNoServer) {
		return err
	}

	cfgPath := filepath.Join(dir, "config.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", cfgPath, err)
	}
	store := queue.FileStore{Path: filepath.Join(dir, "queue.json")}
	reqs := make([]queue.NewItem, len(urls))
	for i, u := range urls {
		reqs[i] = queue.NewItem{URL: u}
	}
	ids, err := store.AppendQueued(cfg, reqs...)
	if errors.Is(err, queue.ErrLocked) {
		return notAnswering(dir, store)
	}
	if len(ids) > 0 {
		fmt.Fprintf(out, "Queued %s; they start when vidl runs\n", downloads(len(ids)))
	}
	if err != nil {
		return fmt.Errorf("queue downloads in %s: %w", store.Path, err)
	}
	return nil
}

// forward hands urls to the running vidl and reports how many it added. It
// fails with ipc.ErrNoServer when nothing listens on the socket.
func forward(dir string, urls []string, out io.Writer) error {
	n, err := ipc.Send(ipc.SocketPath(dir), urls, ipc.DefaultTimeout)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Added %s to the running vidl\n", downloads(n))
	return nil
}

// notAnswering is the error for a lock held by a vidl whose socket is gone.
func notAnswering(dir string, store queue.FileStore) error {
	return fmt.Errorf("a running vidl holds %s but does not answer on %s", store.LockPath(), ipc.SocketPath(dir))
}

func downloads(n int) string {
	if n == 1 {
		return "1 download"
	}
	return fmt.Sprintf("%d downloads", n)
}
