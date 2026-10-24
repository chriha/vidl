package queue

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/chriha/vidl/internal/config"
	"github.com/chriha/vidl/internal/ytdlp"
)

func tempStore(t *testing.T) FileStore {
	t.Helper()
	return FileStore{Path: filepath.Join(t.TempDir(), "queue.json")}
}

func lock(t *testing.T, s FileStore) *Lock {
	t.Helper()
	l, err := s.Lock()
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	t.Cleanup(func() {
		if err := l.Release(); err != nil {
			t.Errorf("Release: %v", err)
		}
	})
	return l
}

func TestLockIsExclusive(t *testing.T) {
	s := tempStore(t)
	l, err := s.Lock()
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}

	if _, err := s.Lock(); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Lock = %v, want ErrLocked", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	lock(t, s)
}

func TestLockCreatesConfigDir(t *testing.T) {
	s := FileStore{Path: filepath.Join(t.TempDir(), "new", "queue.json")}
	lock(t, s)
	if _, err := os.Stat(s.LockPath()); err != nil {
		t.Errorf("lock file: %v", err)
	}
}

// fixtureState has one queued item and one finished item with most fields set.
func fixtureState() State {
	added := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	sub := true
	return State{
		Queue: []Item{{
			ID: "0000000000000001", URL: "https://example.com/a", Title: "A", Format: ytdlp.MP3,
			Resolution: "best", Status: Paused, Progress: ytdlp.Progress{Percent: 40},
			PartFiles: []string{"/dl/A.mp3.part"}, AddedAt: added, StartedAt: added.Add(time.Minute),
			ClipStart: time.Second, Template: "%(id)s.%(ext)s", Subtitles: &sub,
		}},
		History: []Item{{
			ID: "0000000000000002", URL: "https://example.com/b", Title: "B", Format: ytdlp.MP4,
			Resolution: "720", Status: Finished, FilePath: "/dl/B.mp4", Size: 1234,
			AddedAt: added, StartedAt: added, FinishedAt: added.Add(time.Hour),
		}},
	}
}

func TestAppendQueuedKeepsPausedRequest(t *testing.T) {
	s := tempStore(t)
	ids, err := s.AppendQueued(appendCfg(), NewItem{URL: "https://example.com/p", Paused: true}, NewItem{URL: "https://example.com/q"})
	if err != nil {
		t.Fatalf("AppendQueued: %v", err)
	}
	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Queue) != 2 || st.Queue[0].ID != ids[0] || st.Queue[1].ID != ids[1] {
		t.Fatalf("queue = %+v, want the two new items", st.Queue)
	}
	if st.Queue[0].Status != Paused || st.Queue[1].Status != Queued {
		t.Errorf("statuses = %v, %v; want paused, queued", st.Queue[0].Status, st.Queue[1].Status)
	}
}

func appendCfg() config.Config {
	cfg := config.Default()
	cfg.DefaultFormat = ytdlp.MKV
	cfg.DefaultResolution = "1080"
	return cfg
}

func TestAppendQueuedKeepsExistingItems(t *testing.T) {
	s := tempStore(t)
	before := fixtureState()
	if err := s.Save(before); err != nil {
		t.Fatal(err)
	}

	ids, err := s.AppendQueued(appendCfg(), NewItem{URL: "https://example.com/c"}, NewItem{URL: "https://example.com/d", Format: ytdlp.MP4, Resolution: "480"})
	if err != nil {
		t.Fatalf("AppendQueued: %v", err)
	}
	if len(ids) != 2 || ids[0] == ids[1] || ids[0] == "" {
		t.Fatalf("ids = %q, want two distinct ids", ids)
	}

	after, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.History, before.History) {
		t.Errorf("history changed:\n got %+v\nwant %+v", after.History, before.History)
	}
	if len(after.Queue) != 3 || !reflect.DeepEqual(after.Queue[0], before.Queue[0]) {
		t.Fatalf("queue = %+v, want the existing item followed by two new ones", after.Queue)
	}
	tests := []struct {
		got        Item
		id, url    string
		format     ytdlp.Format
		resolution string
	}{
		{after.Queue[1], ids[0], "https://example.com/c", ytdlp.MKV, "1080"},
		{after.Queue[2], ids[1], "https://example.com/d", ytdlp.MP4, "480"},
	}
	for _, tt := range tests {
		it := tt.got
		if it.ID != tt.id || it.URL != tt.url || it.Status != Queued || it.Format != tt.format || it.Resolution != tt.resolution || it.AddedAt.IsZero() {
			t.Errorf("new item = %+v, want id %s, url %s, queued, %v %s", it, tt.id, tt.url, tt.format, tt.resolution)
		}
	}
}

func TestAppendQueuedCreatesMissingFile(t *testing.T) {
	s := FileStore{Path: filepath.Join(t.TempDir(), "new", "queue.json")}
	ids, err := s.AppendQueued(appendCfg(), NewItem{URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("AppendQueued: %v", err)
	}
	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Queue) != 1 || st.Queue[0].ID != ids[0] || len(st.History) != 0 {
		t.Errorf("state = %+v", st)
	}
}

func TestAppendQueuedLoadsInManager(t *testing.T) {
	s := tempStore(t)
	if err := s.Save(fixtureState()); err != nil {
		t.Fatal(err)
	}
	ids, err := s.AppendQueued(appendCfg(), NewItem{URL: "c"})
	if err != nil {
		t.Fatalf("AppendQueued: %v", err)
	}

	m, r := newManager(t, 1, s)
	r.waitStarted(t, "c")
	queue, history := m.Snapshot()
	if len(queue) != 2 || len(history) != 1 {
		t.Fatalf("queue = %+v, history = %+v", queue, history)
	}
	if it, ok := m.Get(ids[0]); !ok || it.URL != "c" || it.Format != ytdlp.MKV {
		t.Errorf("appended item = %+v, %v", it, ok)
	}
}

func TestAppendQueuedRefusesWhileLocked(t *testing.T) {
	s := tempStore(t)
	if err := s.Save(fixtureState()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	lock(t, s)

	if _, err := s.AppendQueued(appendCfg(), NewItem{URL: "c"}); !errors.Is(err, ErrLocked) {
		t.Fatalf("AppendQueued = %v, want ErrLocked", err)
	}
	after, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("queue file changed while locked:\n%s", after)
	}
}

func TestAppendQueuedKeepsCorruptFile(t *testing.T) {
	s := tempStore(t)
	const corrupt = `{"queue": [`
	if err := os.WriteFile(s.Path, []byte(corrupt), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AppendQueued(appendCfg(), NewItem{URL: "c"}); err == nil {
		t.Fatal("AppendQueued on a corrupt file succeeded")
	}
	after, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != corrupt {
		t.Errorf("corrupt file overwritten with:\n%s", after)
	}
	lock(t, s) // the failed append released the lock
}
