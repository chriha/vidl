package ui

import (
	"slices"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/chriha/vidl/internal/queue"
)

func itemsOf(statuses ...queue.Status) []queue.Item {
	var out []queue.Item
	for i, s := range statuses {
		out = append(out, queue.Item{ID: string(rune('a' + i)), URL: string(rune('a' + i)), Status: s})
	}
	return out
}

func TestTabCounts(t *testing.T) {
	const (
		dl, q, p = queue.Downloading, queue.Queued, queue.Paused
		f, c, fi = queue.Failed, queue.Cancelled, queue.Finished
	)
	mixed := itemsOf(dl, dl, q, q, q, p, f, c, c)
	tests := []struct {
		name  string
		model Model
		tab   tab
		want  []statusCount
	}{
		{"mixed statuses in order", Model{queue: mixed}, tabDownloads,
			[]statusCount{{dl, 2}, {q, 3}, {p, 1}, {f, 1}, {c, 2}}},
		{"all zero", Model{}, tabDownloads, nil},
		{"zeros hidden in the middle", Model{queue: itemsOf(c, dl, f)}, tabDownloads,
			[]statusCount{{dl, 1}, {f, 1}, {c, 1}}},
		{"search filter", Model{queue: itemsOf(dl, q, q), filter: [tabCount]string{tabDownloads: "b"}}, tabDownloads,
			[]statusCount{{q, 1}}},
		{"status filter", Model{queue: mixed, status: [tabCount]int{tabDownloads: 2}}, tabDownloads, // queued
			[]statusCount{{q, 3}}},
		{"finished with history", Model{history: itemsOf(fi, fi, fi)}, tabFinished,
			[]statusCount{{fi, 3}}},
		{"finished search", Model{history: itemsOf(fi, fi, fi), filter: [tabCount]string{tabFinished: "c"}}, tabFinished,
			[]statusCount{{fi, 1}}},
		{"finished without history", Model{}, tabFinished, nil},
		{"settings", Model{queue: mixed, history: itemsOf(fi)}, tabSettings, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.model.tabCounts(tt.tab); !slices.Equal(got, tt.want) {
				t.Errorf("tabCounts = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCancelledIsMagenta(t *testing.T) {
	if got := statusColors[queue.Cancelled]; got != lipgloss.Color("13") {
		t.Errorf("cancelled color = %v, want 13", got)
	}
}
