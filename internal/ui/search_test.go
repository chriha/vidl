package ui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/queue"
)

func urls(items []queue.Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.URL)
	}
	return out
}

func TestFilterItems(t *testing.T) {
	items := []queue.Item{
		{URL: "https://a.example/1", Title: "Me at the Zoo"},
		{URL: "https://b.example/zoo"},
		{URL: "https://c.example/3", Title: "Cats"},
	}
	tests := []struct {
		query string
		want  []string
	}{
		{"", urls(items)},
		{"  ", urls(items)},
		{"ZOO", []string{"https://a.example/1", "https://b.example/zoo"}},
		{"cats", []string{"https://c.example/3"}},
		{"c.example", []string{"https://c.example/3"}},
		{"dogs", nil},
	}
	for _, tt := range tests {
		if got := urls(filterItems(items, tt.query)); !slices.Equal(got, tt.want) {
			t.Errorf("filter %q = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestSearchFiltersAsYouType(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "apple", "banana", "avocado")
	m = press(t, m, "/", "a", "v")
	if m.search == nil {
		t.Fatal("search input closed")
	}
	if got := urls(m.items()); !slices.Equal(got, []string{"avocado"}) {
		t.Errorf("items = %v, want [avocado]", got)
	}
	if got := selectedURL(m); got != "avocado" {
		t.Errorf("selected %q, want avocado", got)
	}
}

func TestSearchEnterKeepsEscClears(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "apple", "banana", "avocado")
	m = press(t, m, "/", "b", "enter")
	if m.search != nil || m.filter[tabDownloads] != "b" {
		t.Fatalf("enter: search open = %v, filter = %q", m.search != nil, m.filter[tabDownloads])
	}
	if got := urls(m.items()); !slices.Equal(got, []string{"banana"}) {
		t.Errorf("items = %v, want [banana]", got)
	}
	if got := urls(m.tabItems(tabFinished)); got != nil {
		t.Errorf("finished tab filtered too: %v", got)
	}

	m = press(t, m, "/", "esc")
	if m.search != nil || m.filter[tabDownloads] != "" || len(m.items()) != 3 {
		t.Errorf("esc in input: filter = %q, %d items", m.filter[tabDownloads], len(m.items()))
	}

	m = press(t, m, "/", "b", "enter", "esc")
	if m.filter[tabDownloads] != "" {
		t.Errorf("esc after enter: filter = %q", m.filter[tabDownloads])
	}
}

func TestEscClosesDetailBeforeFilter(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "apple", "banana")
	m = press(t, m, "/", "b", "enter", "enter", "esc")
	if m.detail || m.filter[tabDownloads] != "b" {
		t.Fatalf("first esc: detail = %v, filter = %q", m.detail, m.filter[tabDownloads])
	}
	if m = press(t, m, "esc"); m.filter[tabDownloads] != "" {
		t.Errorf("second esc: filter = %q", m.filter[tabDownloads])
	}
}

func TestActionsApplyToFilteredSelection(t *testing.T) {
	m, mgr, _ := newTestModel(t, 1, "apple", "banana", "avocado")
	m = press(t, m, "/", "v", "enter", "r", "y")
	if m.toast.err != nil {
		t.Fatal(m.toast.err)
	}
	q, _ := mgr.Snapshot()
	if got := urls(q); !slices.Equal(got, []string{"apple", "banana"}) {
		t.Errorf("queue = %v, want avocado removed", got)
	}
}

func TestSearchOnlyOnTables(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a")
	if m = press(t, m, "3", "/"); m.search != nil {
		t.Error("/ opened search on the Settings tab")
	}
}

// newStatusModel returns a Downloads tab with one item per filterable status:
// a downloading, b queued, c paused, d cancelled, fail failed.
func newStatusModel(t *testing.T) Model {
	t.Helper()
	m, mgr, _ := newTestModel(t, 1, "a", "b", "c", "d", "fail")
	ids := map[string]string{}
	for _, it := range m.queue {
		ids[it.URL] = it.ID
	}
	for _, err := range []error{mgr.Pause(ids["c"]), mgr.Cancel(ids["d"]), mgr.Start(ids["fail"])} {
		if err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "statuses", func() bool {
		a, _ := mgr.Get(ids["a"])
		f, _ := mgr.Get(ids["fail"])
		return a.Status == queue.Downloading && f.Status == queue.Failed
	})
	return update(t, m, changedMsg{})
}

func TestStatusFilterCycles(t *testing.T) {
	m := newStatusModel(t)
	steps := []struct {
		label string
		want  []string
	}{
		{"downloading", []string{"a"}},
		{"queued", []string{"b"}},
		{"paused", []string{"c"}},
		{"failed", []string{"fail"}},
		{"cancelled", []string{"d"}},
		{"", []string{"a", "b", "c", "d", "fail"}},
	}
	for _, s := range steps {
		m = press(t, m, "f")
		if got := urls(m.items()); !slices.Equal(got, s.want) {
			t.Errorf("filter %q: items = %v, want %v", s.label, got, s.want)
		}
		if got := m.filtering(); got != (s.label != "") {
			t.Errorf("filter %q: filtering = %v", s.label, got)
		}
		if line := ansi.Strip(m.filterLine()); s.label != "" && !strings.Contains(line, "["+s.label+"]") {
			t.Errorf("filter line = %q, want [%s]", line, s.label)
		}
	}
}

func TestStatusFilterCombinesWithSearch(t *testing.T) {
	m := newStatusModel(t)
	m = press(t, m, "f", "f") // queued
	if got := urls(m.items()); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("queued: items = %v", got)
	}
	m = press(t, m, "/", "a", "enter")
	if got := m.items(); len(got) != 0 {
		t.Errorf("queued + %q: items = %v, want none", "a", urls(got))
	}
	m = press(t, m, "f", "f") // failed
	if got := urls(m.items()); !slices.Equal(got, []string{"fail"}) {
		t.Errorf("failed + %q: items = %v, want [fail]", "a", got)
	}
	if line := ansi.Strip(m.filterLine()); !strings.Contains(line, "[failed]") || !strings.Contains(line, "1 of 5") {
		t.Errorf("filter line = %q, want status and count", line)
	}
}

func TestStatusFilterCursorFollows(t *testing.T) {
	m := newStatusModel(t)
	m = press(t, m, "j", "j") // c, paused
	m = press(t, m, "f", "f", "f")
	if got := selectedURL(m); got != "c" || m.cursor[tabDownloads] != 0 {
		t.Errorf("paused: selected %q at %d, want c at 0", got, m.cursor[tabDownloads])
	}
	m = press(t, m, "f") // failed: c no longer matches
	if got := selectedURL(m); got != "fail" || m.cursor[tabDownloads] != 0 {
		t.Errorf("failed: selected %q at %d, want fail at 0", got, m.cursor[tabDownloads])
	}
	m = press(t, m, "f", "f") // cancelled, then all
	if got := selectedURL(m); got != "d" || m.cursor[tabDownloads] != 3 {
		t.Errorf("all: selected %q at %d, want d at 3", got, m.cursor[tabDownloads])
	}
}

func TestEscClearsStatusAndSearch(t *testing.T) {
	m := newStatusModel(t)
	m = press(t, m, "f", "/", "a", "enter", "enter", "esc")
	if m.detail || m.filter[tabDownloads] != "a" || !slices.Equal(urls(m.items()), []string{"a"}) {
		t.Fatalf("first esc: detail = %v, filter = %q, items = %v", m.detail, m.filter[tabDownloads], urls(m.items()))
	}
	m = press(t, m, "esc")
	if m.filter[tabDownloads] != "" || m.filtering() || len(m.items()) != 5 {
		t.Errorf("second esc: filter = %q, filtering = %v, %d items", m.filter[tabDownloads], m.filtering(), len(m.items()))
	}

	m = press(t, m, "f", "esc")
	if m.filtering() || len(m.items()) != 5 {
		t.Errorf("esc with only a status filter: filtering = %v, %d items", m.filtering(), len(m.items()))
	}
}

func TestStatusFilterOnlyOnDownloads(t *testing.T) {
	m := newStatusModel(t)
	for _, k := range []string{"2", "3"} {
		m = press(t, m, k, "f")
		if m.filtering() {
			t.Errorf("tab %s: f set a filter", k)
		}
	}
	if m = press(t, m, "1"); m.filtering() || len(m.items()) != 5 {
		t.Errorf("downloads: filter leaked from another tab, %d items", len(m.items()))
	}
}

func TestStatusFilterHelp(t *testing.T) {
	keys := newKeyMap()
	has := func(bindings []key.Binding) bool {
		return slices.ContainsFunc(bindings, func(b key.Binding) bool { return slices.Contains(b.Keys(), "f") })
	}
	for _, tt := range []struct {
		tab  tab
		want bool
	}{{tabDownloads, true}, {tabFinished, false}, {tabSettings, false}} {
		k := tabKeys{keyMap: keys, tab: tt.tab}
		if got := has(k.ShortHelp()); got != tt.want {
			t.Errorf("tab %d short help has f = %v, want %v", tt.tab, got, tt.want)
		}
		if got := has(slices.Concat(k.FullHelp()...)); got != tt.want {
			t.Errorf("tab %d full help has f = %v, want %v", tt.tab, got, tt.want)
		}
	}
}
