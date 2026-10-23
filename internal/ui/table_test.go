package ui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"vidl/internal/queue"
	"vidl/internal/ytdlp"
)

func TestLayoutDropsColumnsWhenNarrow(t *testing.T) {
	visible := func(total int) []string {
		var names []string
		for i, w := range layout(downloadColumns, total) {
			if w > 0 {
				names = append(names, downloadColumns[i].title)
			}
		}
		return names
	}
	all := []string{"#", "Title", "Status", "Progress", "Speed", "ETA", "Size"}
	cases := []struct {
		width int
		want  []string
	}{
		{86, all},
		{85, all[:6]},
		{76, all[:6]},
		{75, all[:5]},
		{68, all[:5]},
		{67, all[:4]},
		{57, all[:4]},
		{56, []string{"#", "Title", "Progress"}},
		{20, []string{"#", "Title", "Progress"}},
	}
	for _, c := range cases {
		if got := visible(c.width); !slices.Equal(got, c.want) {
			t.Errorf("width %d: %v, want %v", c.width, got, c.want)
		}
	}
}

func TestRowsFillWidthExactly(t *testing.T) {
	items := []queue.Item{
		{URL: "u", Title: "a very long title that will certainly need to be truncated somewhere", Status: queue.Downloading,
			Progress: ytdlp.Progress{Percent: 42, Speed: 1 << 20, Total: 5 << 20}},
		{URL: "v", Status: queue.Downloading, Progress: ytdlp.Progress{Percent: 100, Stage: "extracting audio"}},
		{URL: "w", Title: "日本語のタイトル日本語のタイトル日本語のタイトル", Status: queue.Queued},
	}
	for _, width := range []int{30, 60, 80, 140} {
		widths := layout(downloadColumns, width)
		for i, it := range items {
			for _, base := range []lipgloss.Style{lipgloss.NewStyle(), cursorStyle} {
				line := renderRow(downloadColumns, widths, downloadCells(i, it), base, width)
				if got := ansi.StringWidth(line); got != width {
					t.Errorf("width %d, item %d: line is %d wide: %q", width, i, got, ansi.Strip(line))
				}
			}
		}
		if got := ansi.StringWidth(headerRow(downloadColumns, widths, width)); got != width {
			t.Errorf("header at %d is %d wide", width, got)
		}
	}
}

func TestFinishedRow(t *testing.T) {
	it := queue.Item{Title: "t", Format: ytdlp.MP3, Resolution: "1080", Size: 2048, FilePath: "/x/t.mp3", Status: queue.Finished}
	widths := layout(finishedColumns, 100)
	line := ansi.Strip(renderRow(finishedColumns, widths, finishedCells(0, it), lipgloss.NewStyle(), 100))
	for _, want := range []string{"mp3", "–", "2.0 KiB", "t.mp3"} {
		if !strings.Contains(line, want) {
			t.Errorf("row %q lacks %q", line, want)
		}
	}
}
