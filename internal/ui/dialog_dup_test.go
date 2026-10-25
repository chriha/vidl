package ui

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chriha/vidl/internal/config"
	"github.com/chriha/vidl/internal/queue"
	"github.com/chriha/vidl/internal/ytdlp"
)

// dupDialogModel opens the dialog on a manager whose history holds a finished
// download of video f1 and whose queue holds video q1.
func dupDialogModel(t *testing.T, p *fakeProber) (Model, *queue.Manager) {
	t.Helper()
	cfg := config.Default()
	cfg.Concurrency = 1
	cfg.DownloadDir = t.TempDir()
	st := &queue.MemStore{State: queue.State{History: []queue.Item{{
		ID: "h1", URL: "https://youtube.test/watch?v=f1", Title: "Finished one", Extractor: "Youtube", VideoID: "f1",
		Status: queue.Finished, FilePath: "/videos/Finished one.mp4", AddedAt: time.Now(), FinishedAt: time.Now(),
	}}}}
	mgr, err := queue.New(cfg, &fakeRunner{}, st)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Close() })
	mgr.Add(queue.NewItem{URL: "https://youtube.test/watch?v=q1", Title: "Queued one", Extractor: "Youtube", VideoID: "q1"})
	m := New(mgr, p, filepath.Join(t.TempDir(), "config.toml"))
	m.clipboard = func() (string, error) { return "", errors.New("no clipboard") }
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	return press(t, m, "n"), mgr
}

func probeURL(t *testing.T, m Model, url string) Model {
	t.Helper()
	m.dialog.input.SetValue(url)
	m, res := submit(t, m)
	return update(t, m, res)
}

func queueLen(t *testing.T, mgr *queue.Manager) int {
	t.Helper()
	q, _ := mgr.Snapshot()
	return len(q)
}

var dupProber = &fakeProber{infos: map[string]*ytdlp.Info{
	// Short links of the finished and the queued video, and a new one.
	"https://yt.test/f1":  {ID: "f1", Extractor: "Youtube", Title: "Finished one", URL: "https://youtube.test/watch?v=f1"},
	"https://yt.test/q1":  {ID: "q1", Extractor: "Youtube", Title: "Queued one", URL: "https://youtube.test/watch?v=q1"},
	"https://yt.test/new": {ID: "n1", Extractor: "Youtube", Title: "New one", URL: "https://youtube.test/watch?v=n1"},
	"https://yt.test/pl": {Title: "List", Extractor: "YoutubeTab", IsPlaylist: true, Entries: []ytdlp.Entry{
		{ID: "f1", Extractor: "Youtube", Title: "Finished one", URL: "https://youtube.test/watch?v=f1"},
		{ID: "n1", Extractor: "Youtube", Title: "New one", URL: "https://youtube.test/watch?v=n1"},
		{ID: "q1", Extractor: "Youtube", Title: "Queued one", URL: "https://youtube.test/watch?v=q1"},
	}},
}}

func TestDialogDuplicateConfirmStep(t *testing.T) {
	tests := []struct {
		name, url, wantTitle string
		wantFinished         bool
	}{
		{"finished in history", "https://yt.test/f1", "Finished one", true},
		{"active in the queue", "https://yt.test/q1", "Queued one", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := dupDialogModel(t, dupProber)
			m = probeURL(t, m, tt.url)
			d := m.dialog
			if d.step != stepDuplicate {
				t.Fatalf("step = %d, want duplicate (err = %v)", d.step, d.err)
			}
			if d.dup == nil || d.dup.Title != tt.wantTitle || (d.dup.Status == queue.Finished) != tt.wantFinished {
				t.Errorf("dup = %+v, want %q (finished %v)", d.dup, tt.wantTitle, tt.wantFinished)
			}
		})
	}
}

func TestDialogDuplicateAddAnyway(t *testing.T) {
	m, mgr := dupDialogModel(t, dupProber)
	m = probeURL(t, m, "https://yt.test/f1")
	m = press(t, m, "enter")
	if m.dialog == nil || m.dialog.step != stepDetails {
		t.Fatalf("enter didn't continue to the details step")
	}
	m = press(t, m, "enter")
	if m.dialog != nil {
		t.Fatalf("dialog still open: err = %v", m.dialog.err)
	}
	if n := queueLen(t, mgr); n != 2 {
		t.Errorf("queue has %d items, want 2", n)
	}
}

func TestDialogDuplicateEscReturnsToInput(t *testing.T) {
	m, mgr := dupDialogModel(t, dupProber)
	m = probeURL(t, m, "https://yt.test/f1")
	m = press(t, m, "esc")
	if m.dialog == nil || m.dialog.step != stepInput {
		t.Fatal("esc didn't return to the URL input")
	}
	if !m.dialog.input.Focused() {
		t.Error("URL input not focused")
	}
	if n := queueLen(t, mgr); n != 1 {
		t.Errorf("queue has %d items, want 1", n)
	}
}

func TestDialogNonDuplicateSkipsConfirm(t *testing.T) {
	m, _ := dupDialogModel(t, dupProber)
	m = probeURL(t, m, "https://yt.test/new")
	if m.dialog.step != stepDetails {
		t.Errorf("step = %d, want details", m.dialog.step)
	}
}

func TestDialogPlaylistDuplicatesUnchecked(t *testing.T) {
	m, mgr := dupDialogModel(t, dupProber)
	m = probeURL(t, m, "https://yt.test/pl")
	d := m.dialog
	if d.step != stepEntries {
		t.Fatalf("step = %d, want entries (err = %v)", d.step, d.err)
	}
	if want := []bool{false, true, false}; !slices.Equal(d.checked, want) {
		t.Errorf("checked = %v, want %v", d.checked, want)
	}
	if len(d.entryDups) != 3 || d.entryDups[0] == "" || d.entryDups[1] != "" || d.entryDups[2] == "" ||
		d.entryDups[0] == d.entryDups[2] {
		t.Errorf("entryDups = %q, want distinct markers on entries 1 and 3", d.entryDups)
	}
	// A duplicate can still be checked and added.
	m = press(t, m, "space", "enter", "enter")
	if m.dialog != nil {
		t.Fatalf("dialog still open: err = %v", m.dialog.err)
	}
	if n := queueLen(t, mgr); n != 3 {
		t.Errorf("queue has %d items, want 3", n)
	}
}

func TestDialogBatchSkipsDuplicates(t *testing.T) {
	m, mgr := dupDialogModel(t, dupProber)
	m = update(t, m, tea.PasteMsg{Content: "https://youtube.test/watch?v=f1 https://a.test/new https://youtube.test/watch?v=q1"})
	d := m.dialog
	if d.step != stepBatch {
		t.Fatalf("step = %d, want batch", d.step)
	}
	if want := []bool{true, false, true}; !slices.Equal(d.batchDups, want) {
		t.Errorf("batchDups = %v, want %v", d.batchDups, want)
	}
	m = press(t, m, "enter")
	if m.dialog != nil {
		t.Fatalf("dialog still open: err = %v", m.dialog.err)
	}
	q, _ := mgr.Snapshot()
	if len(q) != 2 || q[1].URL != "https://a.test/new" {
		t.Errorf("queue = %+v, want q1 and https://a.test/new", q)
	}
}

func TestDialogBatchAllDuplicates(t *testing.T) {
	m, mgr := dupDialogModel(t, dupProber)
	m = update(t, m, tea.PasteMsg{Content: "https://youtube.test/watch?v=f1 https://youtube.test/watch?v=q1"})
	m = press(t, m, "enter")
	if m.dialog == nil || m.dialog.step != stepBatch {
		t.Fatal("dialog closed or left the batch step")
	}
	if !errors.Is(m.dialog.err, errAllDuplicates) {
		t.Errorf("err = %v, want %v", m.dialog.err, errAllDuplicates)
	}
	if n := queueLen(t, mgr); n != 1 {
		t.Errorf("queue has %d items, want 1", n)
	}
}
