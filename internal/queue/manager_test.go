package queue

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chriha/vidl/internal/config"
	"github.com/chriha/vidl/internal/ytdlp"
)

// fakeRunner blocks every download until the test finishes it via done(url).
type fakeRunner struct {
	mu      sync.Mutex
	results map[string]chan error
	parts   map[string][]string  // url -> files reported as being written
	files   map[string]string    // url -> final path, instead of /dl/<url>.mp4
	logs    map[string][]string  // url -> output lines printed before blocking
	jobs    map[string]ytdlp.Job // url -> most recent job
	started chan string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{results: map[string]chan error{}, parts: map[string][]string{}, files: map[string]string{}, logs: map[string][]string{}, jobs: map[string]ytdlp.Job{}, started: make(chan string, 100)}
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
	parts := f.parts[job.URL]
	file := f.files[job.URL]
	logs := f.logs[job.URL]
	f.jobs[job.URL] = job
	f.mu.Unlock()
	for _, line := range logs {
		onEvent(ytdlp.Event{Kind: ytdlp.EventLog, Line: line})
	}
	if file == "" {
		file = "/dl/" + job.URL + ".mp4"
	}
	onEvent(ytdlp.Event{Kind: ytdlp.EventInfo, Info: ytdlp.VideoInfo{Title: "title of " + job.URL}})
	ev := ytdlp.Event{Kind: ytdlp.EventProgress, Progress: ytdlp.Progress{Percent: 50, Speed: 1000}}
	ev.Files = parts
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

// job returns the most recent job of url.
func (f *fakeRunner) job(url string) ytdlp.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jobs[url]
}

// overwrites reports whether the most recent run of url asked yt-dlp to
// replace an existing file.
func (f *fakeRunner) overwrites(url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.jobs[url].Args(), "--force-overwrites")
}

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
	r.parts["a"] = []string{part}
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

func TestRequeueWaitsForFreeSlotAtItsPosition(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "queue.json")}
	m, r := newManager(t, 1, store)
	ids := add(m, "a", "b", "c")
	r.waitStarted(t, "a")
	m.Pause(ids[1])

	if err := m.Requeue(ids[1]); err != nil {
		t.Fatal(err)
	}
	if status(m, ids[1]) != Queued {
		t.Fatalf("b = %v, want queued", status(m, ids[1]))
	}
	r.assertNoStart(t) // the only slot is still taken
	st, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.Queue[1].ID != ids[1] || st.Queue[1].Status != Queued {
		t.Errorf("saved queue[1] = %s %v, want %s queued", st.Queue[1].ID, st.Queue[1].Status, ids[1])
	}

	r.finish("a", nil)
	r.waitStarted(t, "b") // b kept its place ahead of c
	r.assertNoStart(t)
}

func TestRequeueStartsWhenSlotFree(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	id := add(m, "a")[0]
	r.waitStarted(t, "a")
	m.Pause(id)
	waitStatus(t, m, id, Paused)

	if err := m.Requeue(id); err != nil {
		t.Fatal(err)
	}
	r.waitStarted(t, "a")
	waitStatus(t, m, id, Downloading)
}

func TestRequeueLeavesOtherStatusesAlone(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	finished, _ := finishedItem(t, m, r, "done")
	ids := add(m, "run", "wait", "fail", "cancel")
	r.waitStarted(t, "run")
	m.Start(ids[2])
	r.waitStarted(t, "fail")
	r.finish("fail", errors.New("boom"))
	waitStatus(t, m, ids[2], Failed)
	m.Cancel(ids[3])

	for _, tc := range []struct {
		name string
		id   string
		want Status
	}{
		{"downloading", ids[0], Downloading},
		{"queued", ids[1], Queued},
		{"failed", ids[2], Failed},
		{"cancelled", ids[3], Cancelled},
		{"finished", finished, Finished},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := m.Requeue(tc.id); err != nil {
				t.Fatalf("err = %v", err)
			}
			if got := status(m, tc.id); got != tc.want {
				t.Errorf("status = %v, want %v", got, tc.want)
			}
		})
	}
	r.assertNoStart(t)

	if err := m.Requeue("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: err = %v, want ErrNotFound", err)
	}
}

func TestCancelDeletesPartFiles(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	dir := t.TempDir()
	part := filepath.Join(dir, "weird [title].mp4.part")
	frag := part + "-Frag3"
	os.WriteFile(part, nil, 0o644)
	os.WriteFile(frag, nil, 0o644)
	r.parts["a"] = []string{part}
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
	if r.overwrites("a") {
		t.Error("retrying a failed download must not overwrite files")
	}
	r.finish("a", nil)
	waitStatus(t, m, id, Finished)
	if it, _ := m.Get(id); it.Error != "" {
		t.Errorf("error not cleared: %q", it.Error)
	}
}

// savedItem returns the item with id from the store's last saved state.
func savedItem(t *testing.T, m *Manager, store *MemStore, id string) Item {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range slices.Concat(store.State.Queue, store.State.History) {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("item %s not saved", id)
	return Item{}
}

func setLogs(r *fakeRunner, url string, lines ...string) {
	r.mu.Lock()
	r.logs[url] = lines
	r.mu.Unlock()
}

func TestFailedDownloadSavesLog(t *testing.T) {
	store := &MemStore{}
	m, r := newManager(t, 1, store)
	lines := []string{"[youtube] abc: Downloading webpage", "ERROR: [youtube] abc: HTTP Error 403"}
	setLogs(r, "a", lines...)
	id := add(m, "a")[0]
	r.waitStarted(t, "a")
	r.finish("a", errors.New("HTTP Error 403"))
	waitStatus(t, m, id, Failed)
	if got := savedItem(t, m, store, id).Log; !equal(got, lines) {
		t.Errorf("saved log = %q, want %q", got, lines)
	}

	setLogs(r, "a")
	m.Start(id)
	r.waitStarted(t, "a")
	if got := savedItem(t, m, store, id).Log; got != nil {
		t.Errorf("saved log after retry = %q, want none", got)
	}
}

func TestFailedDownloadLogSurvivesRestart(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "queue.json")}
	m, r := newManager(t, 1, store)
	lines := []string{"line 1", "ERROR: line 2"}
	setLogs(r, "a", lines...)
	id := add(m, "a")[0]
	r.waitStarted(t, "a")
	r.finish("a", errors.New("boom"))
	waitStatus(t, m, id, Failed)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	m2, _ := newManager(t, 1, store)
	if it, _ := m2.Get(id); !equal(it.Log, lines) {
		t.Errorf("log after restart = %q, want %q", it.Log, lines)
	}
}

func TestOnlyFailedDownloadsSaveLog(t *testing.T) {
	store := &MemStore{}
	m, r := newManager(t, 1, store)
	for _, u := range []string{"f", "p", "q", "d"} {
		setLogs(r, u, "output of "+u)
	}
	ids := add(m, "f", "p", "q", "d")
	r.waitStarted(t, "f")
	r.finish("f", nil)
	waitStatus(t, m, ids[0], Finished)
	r.waitStarted(t, "p")
	m.Pause(ids[1])
	r.waitStarted(t, "q")
	m.Pause(ids[2])
	r.waitStarted(t, "d")
	m.Requeue(ids[2]) // waits: d holds the only slot

	tests := []struct {
		name string
		id   string
		want Status
	}{
		{"finished", ids[0], Finished},
		{"paused", ids[1], Paused},
		{"queued", ids[2], Queued},
		{"downloading", ids[3], Downloading},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if it, _ := m.Get(tt.id); it.Status != tt.want || len(it.Log) == 0 {
				t.Fatalf("setup: status %v with %d log lines, want %v with output", it.Status, len(it.Log), tt.want)
			}
			if got := savedItem(t, m, store, tt.id).Log; got != nil {
				t.Errorf("saved log = %q, want none", got)
			}
		})
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
	r.parts["a"] = []string{part}
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

func TestEstimatedSizePersists(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "queue.json")}
	want := State{Queue: []Item{{ID: "a", URL: "a", EstimatedSize: 432013312}}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Queue) != 1 || got.Queue[0].EstimatedSize != 432013312 {
		t.Errorf("queue = %+v", got.Queue)
	}
}

func TestLoadWithoutEstimatedSize(t *testing.T) {
	got, err := FileStore{Path: "testdata/queue_legacy.json"}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Queue) != 1 || got.Queue[0].URL != "a" || got.Queue[0].EstimatedSize != 0 {
		t.Errorf("queue = %+v", got.Queue)
	}
}

// finishedItem downloads url to a real 1234-byte file and returns its id and path.
func finishedItem(t *testing.T, m *Manager, r *fakeRunner, url string) (id, path string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), url+".mp4")
	if err := os.WriteFile(path, make([]byte, 1234), 0o644); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.files[url] = path
	r.mu.Unlock()
	id = add(m, url)[0]
	r.waitStarted(t, url)
	r.finish(url, nil)
	waitStatus(t, m, id, Finished)
	return id, path
}

func TestRedownloadFinished(t *testing.T) {
	m, r := newManager(t, 1, &MemStore{})
	id, path := finishedItem(t, m, r, "a")
	if r.overwrites("a") {
		t.Error("first download must not overwrite files")
	}

	if err := m.Start(id); err != nil {
		t.Fatal(err)
	}
	r.waitStarted(t, "a")
	if !r.overwrites("a") {
		t.Error("re-download must pass --force-overwrites")
	}
	q, h := m.Snapshot()
	if len(q) != 1 || q[0].ID != id || q[0].Status != Downloading || len(h) != 0 {
		t.Fatalf("while re-downloading: queue = %+v, history = %+v", q, h)
	}
	if q[0].FilePath != path || q[0].Size != 1234 {
		t.Errorf("previous file lost while re-downloading: path %q, size %d", q[0].FilePath, q[0].Size)
	}

	r.finish("a", nil)
	waitStatus(t, m, id, Finished)
	if q, h := m.Snapshot(); len(q) != 0 || len(h) != 1 || h[0].ID != id {
		t.Errorf("after re-download: queue = %+v, history = %+v", q, h)
	}
}

func TestRedownloadStopped(t *testing.T) {
	tests := []struct {
		name string
		stop func(m *Manager, r *fakeRunner, id string)
		want Status
	}{
		{"failed", func(_ *Manager, r *fakeRunner, _ string) { r.finish("a", errors.New("HTTP Error 403")) }, Failed},
		{"cancelled", func(m *Manager, _ *fakeRunner, id string) { m.Cancel(id) }, Cancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, r := newManager(t, 1, &MemStore{})
			id, path := finishedItem(t, m, r, "a")
			part := path + ".part"
			if err := os.WriteFile(part, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			// A single-stream download reports its final path as being
			// written, which is the previous download's file. It comes first
			// so that it would be gone by the time the part file is.
			r.mu.Lock()
			r.parts["a"] = []string{path, part}
			r.mu.Unlock()

			m.Start(id)
			r.waitStarted(t, "a")
			tt.stop(m, r, id)
			waitStatus(t, m, id, tt.want)
			if tt.want == Cancelled {
				waitFor(t, "part file deletion", func() bool {
					_, err := os.Stat(part)
					return errors.Is(err, os.ErrNotExist)
				})
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("previous file deleted: %v", err)
			}
			q, h := m.Snapshot()
			if len(q) != 1 || q[0].ID != id || len(h) != 0 {
				t.Fatalf("queue = %+v, history = %+v", q, h)
			}
			if q[0].FilePath != path || q[0].Size != 1234 {
				t.Errorf("previous file forgotten: path %q, size %d", q[0].FilePath, q[0].Size)
			}

			m.Start(id)
			r.waitStarted(t, "a")
			if !r.overwrites("a") {
				t.Error("retrying a re-download must still overwrite")
			}
		})
	}
}

func TestRedownloadAcrossRestart(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "queue.json")}
	m, r := newManager(t, 1, store)
	id, _ := finishedItem(t, m, r, "a")
	m.Start(id)
	r.waitStarted(t, "a")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	m2, r2 := newManager(t, 1, store)
	r2.waitStarted(t, "a") // the interrupted re-download resumes
	if !r2.overwrites("a") {
		t.Error("resumed re-download must pass --force-overwrites")
	}
	r2.finish("a", nil)
	waitStatus(t, m2, id, Finished)
	if q, h := m2.Snapshot(); len(q) != 0 || len(h) != 1 || h[0].ID != id {
		t.Errorf("queue = %+v, history = %+v", q, h)
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

func TestJobUsesSponsorBlockSetting(t *testing.T) {
	for _, categories := range []string{"", "sponsor,selfpromo"} {
		t.Run(categories, func(t *testing.T) {
			m, r := newManager(t, 1, &MemStore{})
			cfg := m.Config()
			cfg.SponsorBlockRemove = categories
			m.SetConfig(cfg)
			add(m, "a")
			r.waitStarted(t, "a")
			if got := r.job("a").SponsorBlockRemove; got != categories {
				t.Errorf("job SponsorBlockRemove = %q, want %q", got, categories)
			}
		})
	}
}

func TestJobUsesItemSponsorBlock(t *testing.T) {
	on, off := true, false
	tests := []struct {
		name    string
		setting string
		item    *bool
		want    string
	}{
		{"no override follows the setting", "sponsor,intro", nil, "sponsor,intro"},
		{"turned off", "sponsor,intro", &off, ""},
		{"left on", "sponsor,intro", &on, "sponsor,intro"},
		{"on without a setting removes nothing", "", &on, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, r := newManager(t, 1, &MemStore{})
			cfg := m.Config()
			cfg.SponsorBlockRemove = tt.setting
			m.SetConfig(cfg)
			m.Add(NewItem{URL: "a", SponsorBlock: tt.item})
			r.waitStarted(t, "a")
			if got := r.job("a").SponsorBlockRemove; got != tt.want {
				t.Errorf("job SponsorBlockRemove = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSponsorBlockOverrideSurvivesRestart(t *testing.T) {
	off := false
	store := FileStore{Path: filepath.Join(t.TempDir(), "queue.json")}
	m, r := newManager(t, 1, store)
	id := m.Add(NewItem{URL: "a", SponsorBlock: &off})[0]
	r.waitStarted(t, "a")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	// The resumed run started before the setting is set, so pause and start
	// again to get a job built with it.
	m2, r2 := newManager(t, 1, store)
	r2.waitStarted(t, "a")
	cfg := m2.Config()
	cfg.SponsorBlockRemove = "sponsor"
	m2.SetConfig(cfg)
	m2.Pause(id)
	waitStatus(t, m2, id, Paused)
	m2.Start(id)
	r2.waitStarted(t, "a")
	if got := r2.job("a").SponsorBlockRemove; got != "" {
		t.Errorf("job after restart removes %q, want nothing", got)
	}
}

// overrides are the per-download settings a job was run with.
type overrides struct {
	Template  string
	Subtitles bool
	Clip      ytdlp.Clip
}

func jobOverrides(j ytdlp.Job) overrides {
	return overrides{j.Template, j.EmbedSubtitles, j.Clip}
}

// subtitleSettings returns m's settings with embed_subtitles on, so that
// tests can tell an override from the setting in both directions.
func subtitleSettings(m *Manager) config.Config {
	cfg := m.Config()
	cfg.EmbedSubtitles = true
	return cfg
}

func TestJobUsesItemOverrides(t *testing.T) {
	on, off := true, false
	clip := ytdlp.Clip{Start: 90 * time.Second, End: 2 * time.Minute}
	def := config.Default().FilenameTemplate
	tests := []struct {
		name string
		req  NewItem
		want overrides
	}{
		{"no overrides follow the settings", NewItem{}, overrides{def, true, ytdlp.Clip{}}},
		{"template", NewItem{Template: "%(id)s.%(ext)s"}, overrides{"%(id)s.%(ext)s", true, ytdlp.Clip{}}},
		{"subtitles off", NewItem{Subtitles: &off}, overrides{def, false, ytdlp.Clip{}}},
		{"subtitles on", NewItem{Subtitles: &on}, overrides{def, true, ytdlp.Clip{}}},
		{"clip", NewItem{Clip: clip}, overrides{def, true, clip}},
		{"open-ended clip", NewItem{Clip: ytdlp.Clip{Start: time.Minute}}, overrides{def, true, ytdlp.Clip{Start: time.Minute}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, r := newManager(t, 1, &MemStore{})
			m.SetConfig(subtitleSettings(m))
			tt.req.URL = "a"
			m.Add(tt.req)
			r.waitStarted(t, "a")
			if got := jobOverrides(r.job("a")); got != tt.want {
				t.Errorf("job = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOverridesKeptAcrossPauseAndRetry(t *testing.T) {
	on := true
	want := overrides{"%(id)s.%(ext)s", true, ytdlp.Clip{Start: time.Second, End: time.Minute}}
	m, r := newManager(t, 1, &MemStore{})
	id := m.Add(NewItem{URL: "a", Template: want.Template, Subtitles: &on, Clip: want.Clip})[0]
	r.waitStarted(t, "a")

	m.Pause(id)
	waitStatus(t, m, id, Paused)
	m.Start(id)
	r.waitStarted(t, "a")
	if got := jobOverrides(r.job("a")); got != want {
		t.Errorf("resumed job = %+v, want %+v", got, want)
	}

	r.finish("a", errors.New("HTTP Error 403"))
	waitStatus(t, m, id, Failed)
	m.Start(id)
	r.waitStarted(t, "a")
	if got := jobOverrides(r.job("a")); got != want {
		t.Errorf("retried job = %+v, want %+v", got, want)
	}
}

func TestOverridesSurviveRestart(t *testing.T) {
	off := false
	want := overrides{"%(id)s.%(ext)s", false, ytdlp.Clip{Start: 1500 * time.Millisecond}}
	store := FileStore{Path: filepath.Join(t.TempDir(), "queue.json")}
	m, r := newManager(t, 1, store)
	m.Add(NewItem{URL: "a", Template: want.Template, Subtitles: &off, Clip: want.Clip})
	r.waitStarted(t, "a")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	m2, r2 := newManager(t, 1, store)
	m2.SetConfig(subtitleSettings(m2))
	r2.waitStarted(t, "a")
	if got := jobOverrides(r2.job("a")); got != want {
		t.Errorf("job after restart = %+v, want %+v", got, want)
	}
}

func TestLegacyQueueFileFollowsSettings(t *testing.T) {
	legacy, err := os.ReadFile("testdata/queue_legacy.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	store := FileStore{Path: path}
	m, r := newManager(t, 1, store)
	cfg := subtitleSettings(m)
	cfg.FilenameTemplate = "%(title)s.%(ext)s"
	m.SetConfig(cfg)
	const id = "0123456789abcdef"
	if it, ok := m.Get(id); !ok || it.Status != Paused {
		t.Fatalf("legacy item = %+v, %v", it, ok)
	}

	m.Start(id)
	r.waitStarted(t, "a")
	want := overrides{cfg.FilenameTemplate, true, ytdlp.Clip{}}
	if got := jobOverrides(r.job("a")); got != want {
		t.Errorf("job = %+v, want %+v", got, want)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"clip_start", "clip_end", "template", "subtitles", "sponsorblock"} {
		if strings.Contains(string(saved), `"`+key+`"`) {
			t.Errorf("item without overrides saved with %q:\n%s", key, saved)
		}
	}
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
