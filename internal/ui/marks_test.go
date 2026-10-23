package ui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
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
