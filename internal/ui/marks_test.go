package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/queue"
)

// markedURLs returns the URLs of the marked items on tab t, in list order.
func markedURLs(m Model, t tab) []string {
	all := m.queue
	if t == tabFinished {
		all = m.history
	}
	var out []string
	for _, it := range all {
		if m.marks[t][it.ID] {
			out = append(out, it.URL)
		}
	}
	return out
}

func TestSpaceTogglesMarkAndAdvances(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a", "b", "c")
	m = press(t, m, "space")
	if got := markedURLs(m, tabDownloads); !slices.Equal(got, []string{"a"}) {
		t.Errorf("marked = %v, want [a]", got)
	}
	if m.cursor[tabDownloads] != 1 {
		t.Errorf("cursor = %d, want 1", m.cursor[tabDownloads])
	}
	m = press(t, m, "space", "k", "k", "space")
	if got := markedURLs(m, tabDownloads); !slices.Equal(got, []string{"b"}) {
		t.Errorf("after unmarking a: marked = %v, want [b]", got)
	}
}

func TestSpaceOnLastRowStays(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a", "b")
	m = press(t, m, "G", "space")
	if got := markedURLs(m, tabDownloads); !slices.Equal(got, []string{"b"}) {
		t.Errorf("marked = %v, want [b]", got)
	}
	if m.cursor[tabDownloads] != 1 {
		t.Errorf("cursor = %d, want 1", m.cursor[tabDownloads])
	}
}

func TestMarkAllMarksVisibleRows(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "apple", "banana", "avocado")
	m = press(t, m, "/", "a", "v", "enter", "V")
	if got := markedURLs(m, tabDownloads); !slices.Equal(got, []string{"avocado"}) {
		t.Fatalf("filtered V: marked = %v, want [avocado]", got)
	}
	m = press(t, m, "esc", "esc", "V")
	if got := markedURLs(m, tabDownloads); !slices.Equal(got, []string{"apple", "banana", "avocado"}) {
		t.Fatalf("V with some marked: marked = %v, want all", got)
	}
	if m = press(t, m, "V"); len(m.marks[tabDownloads]) != 0 {
		t.Errorf("V with all marked: marked = %v, want none", markedURLs(m, tabDownloads))
	}
}

func TestMarksDropWhenItemsLeaveTheTab(t *testing.T) {
	m, mgr, _ := newTestModel(t, 1, "a", "b", "c")
	m = press(t, m, "V")
	if err := mgr.Remove(m.queue[1].ID); err != nil {
		t.Fatal(err)
	}
	m = update(t, m, changedMsg{})
	if got := markedURLs(m, tabDownloads); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("marked = %v, want [a c]", got)
	}
	if n := len(m.marks[tabDownloads]); n != 2 {
		t.Errorf("%d marks kept, want 2", n)
	}
}

func TestMarksKeptWhileFilteredOut(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "apple", "banana")
	m = press(t, m, "space", "/", "b", "enter")
	if got := markedURLs(m, tabDownloads); !slices.Equal(got, []string{"apple"}) {
		t.Errorf("marked = %v, want [apple] kept behind the filter", got)
	}
}

func TestMarksArePerTab(t *testing.T) {
	m, _, r := newTestModel(t, 1, "a", "b")
	r.finish("a")
	waitFor(t, "a finished", func() bool {
		_, h := m.mgr.Snapshot()
		return len(h) == 1
	})
	m = update(t, m, changedMsg{})
	m = press(t, m, "space", "2")
	if n := len(m.marks[tabFinished]); n != 0 {
		t.Fatalf("finished tab has %d marks before marking", n)
	}
	m = press(t, m, "space")
	if got := markedURLs(m, tabFinished); !slices.Equal(got, []string{"a"}) {
		t.Errorf("finished marked = %v, want [a]", got)
	}
	if got := markedURLs(m, tabDownloads); !slices.Equal(got, []string{"b"}) {
		t.Errorf("downloads marked = %v, want [b]", got)
	}
}

func TestNoMarksOnSettings(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a")
	m = press(t, m, "3", "space", "V")
	for tb, ms := range m.marks {
		if len(ms) != 0 {
			t.Errorf("tab %d has marks after space/V on Settings", tb)
		}
	}
}

func TestEscClearsMarksBeforeFilters(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "apple", "banana")
	m = press(t, m, "/", "a", "enter", "space", "enter")
	m = press(t, m, "esc")
	if m.detail || len(m.marks[tabDownloads]) != 1 || m.filter[tabDownloads] != "a" {
		t.Fatalf("first esc: detail = %v, marks = %d, filter = %q", m.detail, len(m.marks[tabDownloads]), m.filter[tabDownloads])
	}
	m = press(t, m, "esc")
	if len(m.marks[tabDownloads]) != 0 || m.filter[tabDownloads] != "a" {
		t.Fatalf("second esc: marks = %d, filter = %q", len(m.marks[tabDownloads]), m.filter[tabDownloads])
	}
	if m = press(t, m, "esc"); m.filter[tabDownloads] != "" {
		t.Errorf("third esc: filter = %q", m.filter[tabDownloads])
	}
}

func TestMarkedRowsRenderMarker(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a", "b")
	m = press(t, m, "space")
	lines := strings.Split(ansi.Strip(m.tableView(downloadColumns, downloadCells, "")), "\n")
	if len(lines) < 3 {
		t.Fatalf("table has %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[1], markGlyph) {
		t.Errorf("marked row %q doesn't start with %q", lines[1], markGlyph)
	}
	if strings.HasPrefix(lines[2], markGlyph) {
		t.Errorf("unmarked row %q starts with %q", lines[2], markGlyph)
	}
	if !strings.Contains(ansi.Strip(m.statusLine()), "1 marked") {
		t.Errorf("status line %q doesn't count the marks", ansi.Strip(m.statusLine()))
	}
}

func TestMarkHelp(t *testing.T) {
	keys := newKeyMap()
	// has reports whether bindings has k for marking (Settings uses space to toggle).
	has := func(bindings []key.Binding, k string) bool {
		return slices.ContainsFunc(bindings, func(b key.Binding) bool {
			return slices.Contains(b.Keys(), k) && strings.HasPrefix(b.Help().Desc, "mark")
		})
	}
	for _, tt := range []struct {
		tab  tab
		want bool
	}{{tabDownloads, true}, {tabFinished, true}, {tabSettings, false}} {
		k := tabKeys{keyMap: keys, tab: tt.tab}
		for _, name := range []string{"space", "V"} {
			if got := has(k.ShortHelp(), name); got != tt.want {
				t.Errorf("tab %d short help has %s = %v, want %v", tt.tab, name, got, tt.want)
			}
			if got := has(slices.Concat(k.FullHelp()...), name); got != tt.want {
				t.Errorf("tab %d full help has %s = %v, want %v", tt.tab, name, got, tt.want)
			}
		}
	}
	k := tabKeys{keyMap: keys, tab: tabDownloads, marked: true}
	if !slices.ContainsFunc(k.escKeys(), func(b key.Binding) bool { return b.Help().Desc == "clear marks" }) {
		t.Error("esc help doesn't mention clearing marks while rows are marked")
	}
}

// markURLs marks the downloads with the given URLs on the current tab.
func markURLs(t *testing.T, m Model, urls ...string) Model {
	t.Helper()
	for _, u := range urls {
		m = press(t, selectURL(t, m, u), "space")
	}
	return m
}

// statusOf returns the current status of the download with url.
func statusOf(t *testing.T, mgr *queue.Manager, url string) queue.Status {
	t.Helper()
	q, h := mgr.Snapshot()
	for _, it := range slices.Concat(q, h) {
		if it.URL == url {
			return it.Status
		}
	}
	t.Fatalf("%q not found", url)
	return 0
}

// newBulkModel returns a model whose downloads a and c are running, fail-b
// failed, d is paused, e queued and f cancelled.
func newBulkModel(t *testing.T) (Model, *queue.Manager) {
	t.Helper()
	m, mgr, _ := newTestModel(t, 2, "a", "fail-b", "c", "d", "e", "f")
	waitFor(t, "fail-b failed and c running", func() bool {
		return statusOf(t, mgr, "fail-b") == queue.Failed && statusOf(t, mgr, "c") == queue.Downloading
	})
	q, _ := mgr.Snapshot()
	for _, it := range q {
		var err error
		switch it.URL {
		case "d":
			err = mgr.Pause(it.ID)
		case "f":
			err = mgr.Cancel(it.ID)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return update(t, m, changedMsg{}), mgr
}

func TestBulkActionsApplyToSupportingMarkedRows(t *testing.T) {
	tests := []struct {
		name   string
		marked []string
		key    string
		toast  string
		want   map[string]queue.Status // final statuses
	}{
		{
			name: "start", marked: []string{"a", "fail-b", "d"}, key: "s",
			toast: "started 2 · skipped 1",
			want:  map[string]queue.Status{"a": queue.Downloading, "d": queue.Downloading},
		},
		{
			name: "pause and requeue", marked: []string{"a", "fail-b", "d", "e"}, key: "p",
			toast: "paused 2 · requeued 1 · skipped 1",
			want:  map[string]queue.Status{"a": queue.Paused, "e": queue.Paused, "fail-b": queue.Failed},
		},
		{
			name: "cancel", marked: []string{"d", "e", "f"}, key: "c",
			toast: "cancelled 2 · skipped 1",
			want:  map[string]queue.Status{"d": queue.Cancelled, "e": queue.Cancelled, "f": queue.Cancelled},
		},
		{
			name: "retry", marked: []string{"fail-b", "e", "f"}, key: "R",
			toast: "retried 2 · skipped 1",
			want:  map[string]queue.Status{"f": queue.Downloading},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, mgr := newBulkModel(t)
			m = press(t, markURLs(t, m, tt.marked...), tt.key)
			if m.toast.err != nil {
				t.Fatalf("toast error: %v", m.toast.err)
			}
			if m.toast.text != tt.toast {
				t.Errorf("toast = %q, want %q", m.toast.text, tt.toast)
			}
			for url, want := range tt.want {
				waitFor(t, fmt.Sprintf("%s %v", url, want), func() bool { return statusOf(t, mgr, url) == want })
			}
		})
	}
}

func TestBulkActionReportsManagerErrors(t *testing.T) {
	m, mgr := newBulkModel(t)
	m = markURLs(t, m, "e", "d")
	// Remove d behind the model's back, so pausing it fails.
	q, _ := mgr.Snapshot()
	for _, it := range q {
		if it.URL == "d" {
			if err := mgr.Remove(it.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	m = press(t, m, "p")
	if !errors.Is(m.toast.err, queue.ErrNotFound) {
		t.Fatalf("toast error = %v, want %v", m.toast.err, queue.ErrNotFound)
	}
	if !strings.Contains(m.toast.text, "paused 1 · failed 1") {
		t.Errorf("toast = %q, want the counts", m.toast.text)
	}
	if got := statusOf(t, mgr, "e"); got != queue.Paused {
		t.Errorf("e = %v, want paused", got)
	}
}

func TestBulkRemoveAsksOnce(t *testing.T) {
	tests := []struct {
		name   string
		marked []string
		note   string
		left   []string // URLs left in the queue
	}{
		{name: "running", marked: []string{"a", "e"}, note: "Partial files will be deleted.", left: []string{"c", "fail-b", "d", "f"}},
		{name: "waiting", marked: []string{"e", "f"}, note: "", left: []string{"a", "c", "fail-b", "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, mgr := newBulkModel(t)
			m = press(t, markURLs(t, m, tt.marked...), "r")
			if m.confirm == nil {
				t.Fatal("no confirmation dialog")
			}
			if m.confirm.question != "Remove 2 downloads?" || m.confirm.note != tt.note {
				t.Errorf("confirm = %q / %q, want %q / %q", m.confirm.question, m.confirm.note, "Remove 2 downloads?", tt.note)
			}
			m = press(t, m, "y")
			if m.confirm != nil || m.toast.err != nil || m.toast.text != "removed 2" {
				t.Fatalf("confirm = %v, toast = %q (%v)", m.confirm, m.toast.text, m.toast.err)
			}
			q, _ := mgr.Snapshot()
			var got []string
			for _, it := range q {
				got = append(got, it.URL)
			}
			slices.Sort(got)
			want := slices.Sorted(slices.Values(tt.left))
			if !slices.Equal(got, want) {
				t.Errorf("queue = %v, want %v", got, want)
			}
			if n := m.markCount(); n != 0 {
				t.Errorf("%d marks left after removing the marked rows", n)
			}
		})
	}
}

func TestBulkRemoveOnFinishedKeepsFiles(t *testing.T) {
	m, mgr, r := newTestModel(t, 3, "a", "b", "c")
	m = finishAll(t, m, mgr, r, "a", "b", "c")
	m = press(t, m, "2")
	m = press(t, markURLs(t, m, "a", "c"), "r")
	if m.confirm == nil || m.confirm.note != "The downloaded files are kept." {
		t.Fatalf("confirm = %+v", m.confirm)
	}
	m = press(t, m, "y")
	if _, h := mgr.Snapshot(); len(h) != 1 || h[0].URL != "b" {
		t.Errorf("history = %+v, want only b", h)
	}
	for _, u := range []string{"a", "c"} {
		if _, err := os.Stat(filepath.Join(mgr.Config().DownloadDir, u+".mp4")); err != nil {
			t.Errorf("file of %s deleted: %v", u, err)
		}
	}
}
