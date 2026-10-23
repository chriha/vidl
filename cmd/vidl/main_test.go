package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/config"
	"github.com/chriha/vidl/internal/ipc"
	"github.com/chriha/vidl/internal/queue"
	"github.com/chriha/vidl/internal/ui"
)

// testEnv isolates the config dir under a short temp path (macOS limits
// socket paths to 104 bytes) and empties PATH, so run can never start the UI.
func testEnv(t *testing.T) (dir string, store queue.FileStore) {
	t.Helper()
	base, err := os.MkdirTemp("", "vidl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("PATH", t.TempDir())
	dir, err = config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	return dir, queue.FileStore{Path: filepath.Join(dir, "queue.json")}
}

// holdLock plays a running instance that holds the queue lock.
func holdLock(t *testing.T, store queue.FileStore) {
	t.Helper()
	l, err := store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Release() })
}

// fakeServer plays a running instance's socket and records what it receives.
type fakeServer struct {
	mu   sync.Mutex
	urls []string
}

func (f *fakeServer) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.urls)
}

func serve(t *testing.T, dir string) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	srv, err := ipc.Listen(ipc.SocketPath(dir), func(urls []string) (int, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.urls = append(f.urls, urls...)
		return len(urls), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { srv.Serve(); close(done) }()
	t.Cleanup(func() { srv.Close(); <-done })
	return f
}

func TestRunForwardsToRunningInstance(t *testing.T) {
	tests := []struct {
		name string
		args []string
		urls []string
		out  string
	}{
		{"vidl url", []string{"https://a", "https://b"}, []string{"https://a", "https://b"}, "Added 2 downloads to the running vidl\n"},
		{"vidl add", []string{"add", "https://a"}, []string{"https://a"}, "Added 1 download to the running vidl\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, store := testEnv(t)
			holdLock(t, store)
			srv := serve(t, dir)

			var out bytes.Buffer
			if err := run(tt.args, &out); err != nil {
				t.Fatalf("run: %v", err)
			}
			if !reflect.DeepEqual(srv.got(), tt.urls) {
				t.Errorf("server got %q, want %q", srv.got(), tt.urls)
			}
			if out.String() != tt.out {
				t.Errorf("output = %q, want %q", out.String(), tt.out)
			}
			if _, err := os.Stat(store.Path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("queue.json touched while an instance runs: %v", err)
			}
		})
	}
}

func TestRunWithoutURLsRefusesSecondInstance(t *testing.T) {
	dir, store := testEnv(t)
	holdLock(t, store)
	srv := serve(t, dir)

	err := run(nil, &bytes.Buffer{})
	if !errors.Is(err, ipc.ErrRunning) {
		t.Fatalf("run = %v, want ErrRunning", err)
	}
	if got := srv.got(); len(got) != 0 {
		t.Errorf("server got %q, want nothing", got)
	}
}

func TestRunFailsWhenLockHolderDoesNotAnswer(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"vidl url", []string{"https://a"}},
		{"vidl add", []string{"add", "https://a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, store := testEnv(t)
			holdLock(t, store)
			before := queue.State{History: []queue.Item{{ID: "h1", URL: "https://old", Status: queue.Finished}}}
			if err := store.Save(before); err != nil {
				t.Fatal(err)
			}

			var out bytes.Buffer
			err := run(tt.args, &out)
			if err == nil || errors.Is(err, ipc.ErrRunning) {
				t.Fatalf("run = %v, want an error about the unresponsive instance", err)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want none", out.String())
			}
			after, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Errorf("queue.json changed: %+v", after)
			}
		})
	}
}

func TestAddWithoutInstanceAppendsToQueueFile(t *testing.T) {
	_, store := testEnv(t)
	before := queue.State{
		Queue:   []queue.Item{{ID: "q1", URL: "https://queued", Status: queue.Queued, Resolution: "720"}},
		History: []queue.Item{{ID: "h1", URL: "https://old", Status: queue.Finished}},
	}
	if err := store.Save(before); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run([]string{"add", "https://a", "https://b"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "Queued 2 downloads; they start when vidl runs\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.History, before.History) || len(after.Queue) != 3 || !reflect.DeepEqual(after.Queue[0], before.Queue[0]) {
		t.Fatalf("state = %+v, want the existing items followed by two new ones", after)
	}
	for i, url := range []string{"https://a", "https://b"} {
		if it := after.Queue[i+1]; it.URL != url || it.Status != queue.Queued {
			t.Errorf("new item %d = %+v, want %s queued", i, it, url)
		}
	}
	// The instance lock was released again, so a vidl can start.
	l, err := store.Lock()
	if err != nil {
		t.Fatalf("lock after add: %v", err)
	}
	l.Release()
}

func TestAddWithoutURLsIsAUsageError(t *testing.T) {
	_, store := testEnv(t)
	var out bytes.Buffer
	if err := run([]string{"add"}, &out); err == nil {
		t.Fatal("run(add) = nil, want a usage error")
	}
	if _, err := os.Stat(store.Path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("queue.json written: %v", err)
	}
}

func TestForwardToUI(t *testing.T) {
	t.Run("returns the count from the UI", func(t *testing.T) {
		var sent []string
		h := forwardToUI(func(msg tea.Msg) {
			m := msg.(ui.AddURLsMsg)
			sent = m.URLs
			m.Added <- len(m.URLs)
		}, time.Second)
		n, err := h([]string{"https://a", "https://b"})
		if err != nil || n != 2 {
			t.Fatalf("handler = %d, %v; want 2, nil", n, err)
		}
		if !reflect.DeepEqual(sent, []string{"https://a", "https://b"}) {
			t.Errorf("sent %q", sent)
		}
	})
	t.Run("fails when the UI does not answer", func(t *testing.T) {
		h := forwardToUI(func(tea.Msg) {}, 10*time.Millisecond)
		if n, err := h([]string{"https://a"}); err == nil || n != 0 {
			t.Fatalf("handler = %d, %v; want 0 and an error", n, err)
		}
	})
}
