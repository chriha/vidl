package queue

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"vidl/internal/config"
	"vidl/internal/ytdlp"
)

// fakeRunner blocks every download until the test finishes it via done(url).
type fakeRunner struct {
	mu      sync.Mutex
	results map[string]chan error
	parts   map[string]string // url -> part file reported as being written
	files   map[string]string // url -> final path, instead of /dl/<url>.mp4
	started chan string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{results: map[string]chan error{}, parts: map[string]string{}, files: map[string]string{}, started: make(chan string, 100)}
}

func (f *fakeRunner) result(url string) chan error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.results[url] == nil {
		f.results[url] = make(chan error, 1)
	}
	return f.results[url]
}

func (f *fakeRunner) Run(ctx context.Context, job ytdlp.Job, onEvent func(ytdlp.Event)) error {
	res := f.result(job.URL)
	f.mu.Lock()
	part := f.parts[job.URL]
	file := f.files[job.URL]
	f.mu.Unlock()
	if file == "" {
		file = "/dl/" + job.URL + ".mp4"
	}
	onEvent(ytdlp.Event{Kind: ytdlp.EventInfo, Info: ytdlp.VideoInfo{Title: "title of " + job.URL}})
	ev := ytdlp.Event{Kind: ytdlp.EventProgress, Progress: ytdlp.Progress{Percent: 50, Speed: 1000}}
	if part != "" {
		ev.Files = []string{part}
	}
	onEvent(ev)
	f.started <- job.URL
	select {
	case err := <-res:
		if err == nil {
			onEvent(ytdlp.Event{Kind: ytdlp.EventFile, Path: file})
		}
		return err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (f *fakeRunner) finish(url string, err error) { f.result(url) <- err }

// waitStarted waits until exactly the given downloads have started, in any
// order, since concurrent downloads start in parallel goroutines.
func (f *fakeRunner) waitStarted(t *testing.T, urls ...string) {
	t.Helper()
	want := map[string]bool{}
	for _, u := range urls {
		want[u] = true
	}
	for range urls {
		select {
		case got := <-f.started:
			if !want[got] {
				t.Fatalf("started %q, want one of %v", got, urls)
			}
			delete(want, got)
		case <-time.After(2 * time.Second):
			t.Fatalf("%v did not start", want)
		}
	}
}

func (f *fakeRunner) assertNoStart(t *testing.T) {
	t.Helper()
	select {
	case got := <-f.started:
		t.Fatalf("unexpected start of %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func newManager(t *testing.T, concurrency int, store Store) (*Manager, *fakeRunner) {
	t.Helper()
	cfg := config.Default()
	cfg.Concurrency = concurrency
	cfg.DownloadDir = t.TempDir()
	r := newFakeRunner()
	m, err := New(cfg, r, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m, r
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func status(m *Manager, id string) Status {
	it, _ := m.Get(id)
	return it.Status
}

func waitStatus(t *testing.T, m *Manager, id string, want Status) {
	t.Helper()
	waitFor(t, "status "+want.String(), func() bool { return status(m, id) == want })
}

func add(m *Manager, urls ...string) []string {
	var reqs []NewItem
	for _, u := range urls {
		reqs = append(reqs, NewItem{URL: u})
	}
	return m.Add(reqs...)
}

func TestConcurrencyLimitAndAutoStart(t *testing.T) {
	m, r := newManager(t, 2, &MemStore{})
	ids := add(m, "a", "b", "c")
	r.waitStarted(t, "a", "b")
	r.assertNoStart(t)
	if status(m, ids[2]) != Queued {
		t.Fatalf("c = %v", status(m, ids[2]))
	}

	r.finish("a", nil)
	r.waitStarted(t, "c")
	waitStatus(t, m, ids[0], Finished)

	_, history := m.Snapshot()
	if len(history) != 1 || history[0].FilePath != "/dl/a.mp4" || history[0].Title != "title of a" {
		t.Errorf("history = %+v", history)
	}
}

func TestFinishedRecordsFileSize(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	path := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(path, make([]byte, 1234), 0o644); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.files["a"] = path
	r.mu.Unlock()
	id := add(m, "a")[0]
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitStatus(t, m, id, Finished)
	if it, _ := m.Get(id); it.Size != 1234 {
		t.Errorf("size = %d, want 1234", it.Size)
	}
}

func TestAddUsesConfigDefaults(t *testing.T) {
	m, _ := newManager(t, 1, &MemStore{})
	it, _ := m.Get(add(m, "a")[0])
	if it.Format != ytdlp.MP4 || it.Resolution != "1080" {
		t.Errorf("format = %q, resolution = %q", it.Format, it.Resolution)
	}
}

func TestPauseAndResumeKeepsPartFiles(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	part := filepath.Join(t.TempDir(), "a.part")
	os.WriteFile(part, nil, 0o644)
	r.parts["a"] = part
	ids := add(m, "a", "b")
	r.waitStarted(t, "a")

	m.Pause(ids[0])
	waitStatus(t, m, ids[0], Paused)
	r.waitStarted(t, "b") // the free slot goes to the next item
	if _, err := os.Stat(part); err != nil {
		t.Errorf("part file deleted on pause: %v", err)
	}
	if it, _ := m.Get(ids[0]); it.Progress.Speed != 0 || it.Progress.Percent != 50 {
		t.Errorf("progress after pause = %+v", it.Progress)
	}

	// Start forces the download even though the only slot is taken.
	m.Start(ids[0])
	r.waitStarted(t, "a")
	if status(m, ids[0]) != Downloading || status(m, ids[1]) != Downloading {
		t.Errorf("statuses = %v, %v", status(m, ids[0]), status(m, ids[1]))
	}
}

func TestCancelDeletesPartFiles(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	dir := t.TempDir()
	part := filepath.Join(dir, "weird [title].mp4.part")
	frag := part + "-Frag3"
	os.WriteFile(part, nil, 0o644)
	os.WriteFile(frag, nil, 0o644)
	r.parts["a"] = part
	id := add(m, "a")[0]
	r.waitStarted(t, "a")

	m.Cancel(id)
	waitStatus(t, m, id, Cancelled)
	// Files are deleted right after the status change, outside the lock.
	for _, f := range []string{part, frag} {
		waitFor(t, filepath.Base(f)+" deletion", func() bool {
			_, err := os.Stat(f)
			return errors.Is(err, os.ErrNotExist)
		})
	}
}

func TestCancelQueuedItem(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	ids := add(m, "a", "b")
	r.waitStarted(t, "a")
	m.Cancel(ids[1])
	if status(m, ids[1]) != Cancelled {
		t.Fatalf("b = %v", status(m, ids[1]))
	}
	r.finish("a", nil)
	r.assertNoStart(t)
}

func TestFailureAndRetry(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	id := add(m, "a")[0]
	r.waitStarted(t, "a")
	r.finish("a", errors.New("HTTP Error 403"))
	waitStatus(t, m, id, Failed)
	if it, _ := m.Get(id); it.Error != "HTTP Error 403" {
		t.Errorf("error = %q", it.Error)
	}

	m.Start(id)
	r.waitStarted(t, "a")
	r.finish("a", nil)
	waitStatus(t, m, id, Finished)
	if it, _ := m.Get(id); it.Error != "" {
		t.Errorf("error not cleared: %q", it.Error)
	}
}

func TestMove(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	ids := add(m, "a", "b", "c", "d")
	r.waitStarted(t, "a")
	order := func() []string {
		q, _ := m.Snapshot()
		var urls []string
		for _, it := range q {
			urls = append(urls, it.URL)
		}
		return urls
	}

	m.Move(ids[3], -1) // d above c
	m.Move(ids[1], -1) // b can't move above the running a
	m.Move(ids[0], 1)  // running a can't move
	m.Move(ids[1], 1)  // b below d
	if got, want := order(), []string{"a", "d", "b", "c"}; !equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	r.finish("a", nil)
	r.waitStarted(t, "d") // the new top of the queue starts next
}

func TestRemove(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	part := filepath.Join(t.TempDir(), "a.part")
	os.WriteFile(part, nil, 0o644)
	r.parts["a"] = part
	ids := add(m, "a", "b")
	r.waitStarted(t, "a")

	if err := m.Remove(ids[0]); err != nil {
		t.Fatal(err)
	}
	r.waitStarted(t, "b")
	waitFor(t, "part file deletion", func() bool {
		_, err := os.Stat(part)
		return errors.Is(err, os.ErrNotExist)
	})
	if _, ok := m.Get(ids[0]); ok {
		t.Error("removed item still present")
	}

	r.finish("b", nil)
	waitStatus(t, m, ids[1], Finished)
	m.Remove(ids[1])
	if _, h := m.Snapshot(); len(h) != 0 {
		t.Errorf("history = %+v", h)
	}
	if err := m.Remove("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "queue.json")}
	m, r := newManager(t, 1, store)
	ids := add(m, "a", "b", "c")
	r.waitStarted(t, "a")
	r.finish("a", nil)
	r.waitStarted(t, "b")
	m.Pause(ids[2])
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	m2, r2 := newManager(t, 1, store)
	r2.waitStarted(t, "b") // interrupted download resumes
	r2.assertNoStart(t)
	if status(m2, ids[2]) != Paused {
		t.Errorf("c = %v", status(m2, ids[2]))
	}
	_, history := m2.Snapshot()
	if len(history) != 1 || history[0].ID != ids[0] {
		t.Errorf("history = %+v", history)
	}
}

func TestSetConfigRaisesConcurrency(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	add(m, "a", "b")
	r.waitStarted(t, "a")
	r.assertNoStart(t)
	cfg := m.Config()
	cfg.Concurrency = 2
	m.SetConfig(cfg)
	r.waitStarted(t, "b")
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
