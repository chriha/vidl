package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"vidl/internal/config"
	"vidl/internal/queue"
	"vidl/internal/ytdlp"
)

// fakeRunner fails URLs starting with "fail", and blocks the rest until
// cancelled or released with finish. Every run first logs fakeLogLines lines.
type fakeRunner struct {
	mu   sync.Mutex
	done map[string]chan struct{}
}

const fakeLogLines = 30

func (f *fakeRunner) ch(url string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done == nil {
		f.done = map[string]chan struct{}{}
	}
	if f.done[url] == nil {
		f.done[url] = make(chan struct{})
	}
	return f.done[url]
}

func (f *fakeRunner) finish(url string) { close(f.ch(url)) }

func (f *fakeRunner) Run(ctx context.Context, job ytdlp.Job, onEvent func(ytdlp.Event)) error {
	for i := range fakeLogLines {
		onEvent(ytdlp.Event{Kind: ytdlp.EventLog, Line: fmt.Sprintf("%s line %d", job.URL, i+1)})
	}
	if strings.HasPrefix(job.URL, "fail") {
		return errors.New("boom")
	}
	onEvent(ytdlp.Event{Kind: ytdlp.EventProgress, Progress: ytdlp.Progress{Percent: 42, Speed: 2048, ETA: time.Minute}})
	select {
	case <-f.ch(job.URL):
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func newTestModel(t *testing.T, concurrency int, urls ...string) (Model, *queue.Manager, *fakeRunner) {
	t.Helper()
	cfg := config.Default()
	cfg.Concurrency = concurrency
	cfg.DownloadDir = t.TempDir()
	r := &fakeRunner{}
	mgr, err := queue.New(cfg, r, &queue.MemStore{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Close() })
	var reqs []queue.NewItem
	for _, u := range urls {
		reqs = append(reqs, queue.NewItem{URL: u, Title: u})
	}
	mgr.Add(reqs...)
	m := New(mgr, &fakeProber{})
	m.clipboard = func() (string, error) { return "", errors.New("no clipboard") }
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 12})
	return m, mgr, r
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case "ctrl+d", "ctrl+u":
			msg = tea.KeyPressMsg{Code: rune(k[5]), Mod: tea.ModCtrl}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		m = update(t, m, msg)
	}
	return m
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

func selectedURL(m Model) string {
	it, _ := m.selected()
	return it.URL
}

func TestCursorFollowsItemAcrossRefresh(t *testing.T) {
	m, mgr, r := newTestModel(t, 1, "a", "b", "c")
	waitFor(t, "a running", func() bool { it, _ := mgr.Get(m.queue[0].ID); return it.Status == queue.Downloading })
	m = press(t, m, "j", "j")
	if got := selectedURL(m); got != "c" {
		t.Fatalf("selected %q, want c", got)
	}
	r.finish("a")
	waitFor(t, "a finished", func() bool { _, h := mgr.Snapshot(); return len(h) == 1 })
	m = update(t, m, changedMsg{})
	if got := selectedURL(m); got != "c" || m.cursor[tabDownloads] != 1 {
		t.Errorf("selected %q at %d, want c at 1", got, m.cursor[tabDownloads])
	}
}

func TestMoveKeepsCursorOnItem(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a", "b", "c")
	m = press(t, m, "G", "u")
	if m.err != nil {
		t.Fatal(m.err)
	}
	if got := selectedURL(m); got != "c" || m.cursor[tabDownloads] != 1 {
		t.Errorf("selected %q at %d, want c at 1", got, m.cursor[tabDownloads])
	}
	m = press(t, m, "d")
	if got := selectedURL(m); got != "c" || m.cursor[tabDownloads] != 2 {
		t.Errorf("selected %q at %d, want c at 2", got, m.cursor[tabDownloads])
	}
}

func TestRemoveNeedsConfirmation(t *testing.T) {
	m, mgr, _ := newTestModel(t, 1, "a", "b")
	m = press(t, m, "j", "r")
	if m.confirm == nil {
		t.Fatal("no confirmation dialog")
	}
	m = press(t, m, "esc")
	if m.confirm != nil || len(m.queue) != 2 {
		t.Fatalf("esc: confirm = %v, %d items", m.confirm, len(m.queue))
	}
	m = press(t, m, "r", "j", "y")
	if m.confirm != nil || m.err != nil {
		t.Fatalf("confirm = %v, err = %v", m.confirm, m.err)
	}
	q, _ := mgr.Snapshot()
	if len(q) != 1 || q[0].URL != "a" {
		t.Errorf("queue = %+v", q)
	}
}

func TestRetryOnlyFailed(t *testing.T) {
	m, mgr, _ := newTestModel(t, 1, "fail-a", "b")
	var failed string
	for _, it := range m.queue {
		if it.URL == "fail-a" {
			failed = it.ID
		}
	}
	waitFor(t, "fail-a failed", func() bool { it, _ := mgr.Get(failed); return it.Status == queue.Failed })
	m = update(t, m, changedMsg{})
	m = selectURL(t, m, "b")
	m = press(t, m, "R")
	if !errors.Is(m.err, errNotRetryable) {
		t.Errorf("retry running: err = %v", m.err)
	}
	m = selectURL(t, m, "fail-a")
	m = press(t, m, "R")
	if m.err != nil {
		t.Errorf("retry failed: err = %v", m.err)
	}
}

func selectURL(t *testing.T, m Model, url string) Model {
	t.Helper()
	m = press(t, m, "g")
	for range m.items() {
		if selectedURL(m) == url {
			return m
		}
		m = press(t, m, "j")
	}
	t.Fatalf("%q not in list", url)
	return m
}

func TestHalfPageAndScrolling(t *testing.T) {
	var urls []string
	for i := range 30 {
		urls = append(urls, string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	m, _, _ := newTestModel(t, 1, urls...)
	rows := m.tableRows()
	m = press(t, m, "ctrl+d")
	if got := m.cursor[tabDownloads]; got != rows/2 {
		t.Errorf("ctrl+d: cursor = %d, want %d", got, rows/2)
	}
	m = press(t, m, "G")
	if cur, off := m.cursor[tabDownloads], m.offset[tabDownloads]; cur != 29 || off != 30-rows {
		t.Errorf("G: cursor %d offset %d, want 29 and %d", cur, off, 30-rows)
	}
	m = press(t, m, "ctrl+u", "g")
	if m.offset[tabDownloads] != 0 {
		t.Errorf("g: offset = %d", m.offset[tabDownloads])
	}
	for _, line := range strings.Split(m.render(), "\n") {
		if w := ansi.StringWidth(line); w > m.width {
			t.Errorf("line wider than terminal (%d): %q", w, line)
		}
	}
	if h := strings.Count(m.render(), "\n") + 1; h > m.height {
		t.Errorf("render height %d > %d", h, m.height)
	}
}
