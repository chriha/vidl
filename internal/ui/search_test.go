package ui

import (
	"slices"
	"testing"

	"vidl/internal/queue"
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
