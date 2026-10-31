package main

import (
	"bytes"
	"context"
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
	"github.com/chriha/vidl/internal/ytdlp"
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
	mu     sync.Mutex
	urls   []string
	paused []bool // Paused of each request
}

func (f *fakeServer) gotPaused() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.paused)
}

func (f *fakeServer) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.urls)
}

func serve(t *testing.T, dir string) *fakeServer {
	t.Helper()
	f, stop, err := startServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	return f
}

// startServer listens on dir's socket; stop shuts it down.
func startServer(dir string) (f *fakeServer, stop func(), err error) {
	f = &fakeServer{}
	srv, err := ipc.Listen(ipc.SocketPath(dir), func(req ipc.Request) (int, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.urls = append(f.urls, req.Add...)
		f.paused = append(f.paused, req.Paused)
		return len(req.Add), nil
	})
	if err != nil {
		return nil, nil, err
	}
	done := make(chan struct{})
	go func() { srv.Serve(); close(done) }()
	return f, func() { srv.Close(); <-done }, nil
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
		{"vidl add --paused", []string{"add", "--paused", "https://a"}, []string{"https://a"}, "Added 1 download to the running vidl, paused\n"},
		{"vidl add url --paused", []string{"add", "https://a", "https://b", "--paused"}, []string{"https://a", "https://b"}, "Added 2 downloads to the running vidl, paused\n"},
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
			wantPaused := []bool{slices.Contains(tt.args, "--paused")}
			if got := srv.gotPaused(); !reflect.DeepEqual(got, wantPaused) {
				t.Errorf("request paused = %v, want %v", got, wantPaused)
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

func TestAddPausedWithoutInstanceAppendsPaused(t *testing.T) {
	_, store := testEnv(t)

	var out bytes.Buffer
	if err := run([]string{"add", "--paused", "https://a", "https://b"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "Added 2 downloads paused; start them in vidl\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Queue) != 2 {
		t.Fatalf("queue = %+v, want two items", after.Queue)
	}
	for i, url := range []string{"https://a", "https://b"} {
		if it := after.Queue[i]; it.URL != url || it.Status != queue.Paused {
			t.Errorf("item %d = %+v, want %s paused", i, it, url)
		}
	}
}

func TestAddUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no URLs", []string{"add"}},
		{"help flag", []string{"add", "--help"}},
		{"flag among URLs", []string{"add", "https://a", "-h"}},
		{"only --paused", []string{"add", "--paused"}},
		{"unknown flag with --paused", []string{"add", "--paused", "--start", "https://a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, store := testEnv(t)
			var out bytes.Buffer
			if err := run(tt.args, &out); err == nil {
				t.Fatalf("run(%q) = nil, want a usage error", tt.args)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want none", out.String())
			}
			if _, err := os.Stat(store.Path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("queue.json written: %v", err)
			}
		})
	}
}

func TestAddWaitsForStartingInstance(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"vidl url", []string{"https://a"}},
		{"vidl add", []string{"add", "https://a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, store := testEnv(t)
			holdLock(t, store)
			type started struct {
				f    *fakeServer
				stop func()
				err  error
			}
			listening := make(chan started, 1)
			// The instance holds the lock but listens only after the client's
			// first attempt, as during startup.
			time.AfterFunc(100*time.Millisecond, func() {
				f, stop, err := startServer(dir)
				listening <- started{f, stop, err}
			})

			var out bytes.Buffer
			runErr := run(tt.args, &out)
			srv := <-listening
			if srv.err != nil {
				t.Fatal(srv.err)
			}
			t.Cleanup(srv.stop)
			if runErr != nil {
				t.Fatalf("run: %v", runErr)
			}
			if got := srv.f.got(); !reflect.DeepEqual(got, []string{"https://a"}) {
				t.Errorf("server got %q", got)
			}
			if want := "Added 1 download to the running vidl\n"; out.String() != want {
				t.Errorf("output = %q, want %q", out.String(), want)
			}
		})
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
		n, err := h(ipc.Request{Add: []string{"https://a", "https://b"}})
		if err != nil || n != 2 {
			t.Fatalf("handler = %d, %v; want 2, nil", n, err)
		}
		if !reflect.DeepEqual(sent, []string{"https://a", "https://b"}) {
			t.Errorf("sent %q", sent)
		}
	})
	t.Run("passes paused to the UI", func(t *testing.T) {
		var sent ui.AddURLsMsg
		h := forwardToUI(func(msg tea.Msg) {
			sent = msg.(ui.AddURLsMsg)
			sent.Added <- len(sent.URLs)
		}, time.Second)
		if _, err := h(ipc.Request{Add: []string{"https://a"}, Paused: true}); err != nil {
			t.Fatal(err)
		}
		if !sent.Paused || !reflect.DeepEqual(sent.URLs, []string{"https://a"}) {
			t.Errorf("sent %+v, want https://a paused", sent)
		}
	})
	t.Run("fails when the UI does not answer", func(t *testing.T) {
		var sent ui.AddURLsMsg
		h := forwardToUI(func(msg tea.Msg) { sent = msg.(ui.AddURLsMsg) }, 10*time.Millisecond)
		if n, err := h(ipc.Request{Add: []string{"https://a"}}); err == nil || n != 0 {
			t.Fatalf("handler = %d, %v; want 0 and an error", n, err)
		}
		if sent.Claim.Take() {
			t.Error("a late UI could still take the request after the timeout")
		}
	})
}

// idleRunner never downloads; size estimates fail.
type idleRunner struct{}

func (idleRunner) Run(context.Context, ytdlp.Job, func(ytdlp.Event)) error {
	return errors.New("not downloading in tests")
}

func (idleRunner) Estimate(context.Context, string, ytdlp.Format, string, string) (*ytdlp.Estimate, error) {
	return nil, errors.New("no estimates in tests")
}

func TestAddStartURLsSkipsDuplicates(t *testing.T) {
	tests := []struct {
		name        string
		urls        []string
		wantQueue   []string
		wantSkipped int
	}{
		{"no urls", nil, []string{"https://a"}, 0},
		{"new only", []string{"https://b"}, []string{"https://a", "https://b"}, 0},
		{"duplicate skipped", []string{"https://a", "https://b"}, []string{"https://a", "https://b"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Concurrency = 0
			cfg.DownloadDir = t.TempDir()
			mgr, err := queue.New(cfg, idleRunner{}, &queue.MemStore{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { mgr.Close() })
			mgr.Add(queue.NewItem{URL: "https://a"})

			if got := addStartURLs(mgr, tt.urls); got != tt.wantSkipped {
				t.Errorf("skipped = %d, want %d", got, tt.wantSkipped)
			}
			q, _ := mgr.Snapshot()
			var got []string
			for _, it := range q {
				got = append(got, it.URL)
			}
			if !slices.Equal(got, tt.wantQueue) {
				t.Errorf("queue = %q, want %q", got, tt.wantQueue)
			}
		})
	}
}
