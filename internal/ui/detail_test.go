package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/queue"
)

func TestDetailToggle(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a", "b")
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	rows := m.tableRows()
	m = press(t, m, "enter")
	if !m.detail || m.detailHeight() < detailMinHeight {
		t.Fatalf("enter: detail = %v, height %d", m.detail, m.detailHeight())
	}
	if got := m.tableRows(); got != rows-m.detailHeight() {
		t.Errorf("table rows with panel = %d, want %d", got, rows-m.detailHeight())
	}
	m = press(t, m, "enter")
	if m.detail {
		t.Error("second enter didn't close the panel")
	}
	m = press(t, m, "enter", "esc")
	if m.detail || m.tableRows() != rows {
		t.Errorf("esc: detail = %v, rows %d", m.detail, m.tableRows())
	}
}

func TestDetailEmptyList(t *testing.T) {
	m, _, _ := newTestModel(t, 1)
	if m = press(t, m, "enter"); m.detail {
		t.Error("panel opened without items")
	}
}

func TestDetailFollowsCursor(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "a", "b")
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(t, m, "enter", "j")
	if it, _ := m.detailItem(); it.URL != "b" {
		t.Errorf("panel shows %q, want b", it.URL)
	}
	m = press(t, m, "2")
	if _, ok := m.detailItem(); ok {
		t.Error("panel shown on an empty Finished tab")
	}
}

func TestDetailLogSticksToBottom(t *testing.T) {
	for _, width := range []int{120, 80} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m, mgr, _ := newTestModel(t, 1, "a", "b")
			id := m.queue[0].ID
			waitFor(t, "a logging", func() bool { it, _ := mgr.Get(id); return len(it.Log) == fakeLogLines })
			m = update(t, m, changedMsg{})
			m = update(t, m, tea.WindowSizeMsg{Width: width, Height: 30})
			m = press(t, m, "enter")
			if m.log.TotalLineCount() != fakeLogLines || !m.log.AtBottom() {
				t.Fatalf("log: %d lines, at bottom %v", m.log.TotalLineCount(), m.log.AtBottom())
			}
			out := m.render()
			if !strings.Contains(out, fmt.Sprintf("a line %d", fakeLogLines)) {
				t.Errorf("last log line not rendered:\n%s", out)
			}
			assertFits(t, m)
		})
	}
}

func TestDetailFitsSmallTerminals(t *testing.T) {
	m, _, _ := newTestModel(t, 1, "fail-a", "b")
	for _, size := range [][2]int{{120, 40}, {100, 20}, {60, 14}, {40, 10}, {40, 6}} {
		m = update(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = press(t, m, "enter")
		assertFits(t, m)
		m = press(t, m, "esc")
	}
	// Narrower than the tab bar doesn't fit yet (milestone 8), but mustn't panic.
	m = update(t, m, tea.WindowSizeMsg{Width: 20, Height: 3})
	m = press(t, m, "enter")
	m.render()
}

func TestDetailFields(t *testing.T) {
	it := queue.Item{
		URL: "https://example.com/v", Title: "Video", Uploader: "someone", Extractor: "Youtube",
		Duration: 75, Format: "mp4", Resolution: "720", Status: queue.Failed,
		Error: strings.Repeat("broken ", 20), PartFiles: []string{"/dl/v.part"},
	}
	it.Progress.Percent, it.Progress.Downloaded, it.Progress.Total = 50, 1024, 2048
	it.Progress.Stream, it.Progress.Streams = 2, 2
	var m Model
	lines := m.detailFields(it, 60)
	text := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"someone · Youtube · 1:15", "mp4 · 720p", "50.0% · 1.0 KiB / 2.0 KiB · stream 2/2", "/dl/v.part"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	errLines := 0
	for _, l := range lines {
		if ansi.StringWidth(l) > 60 {
			t.Errorf("line wider than 60: %q", l)
		}
		if strings.Contains(l, "broken") {
			errLines++
		}
	}
	if errLines < 2 {
		t.Errorf("error not wrapped (%d lines)", errLines)
	}
}

func TestDetailFieldsOverrides(t *testing.T) {
	off, on := false, true
	for _, tc := range []struct {
		name    string
		it      queue.Item
		want    []string
		notWant []string
	}{
		{"none", queue.Item{URL: "u"}, nil, []string{"Clip", "Template", "Subtitles"}},
		{"clip", queue.Item{URL: "u", ClipStart: 90 * time.Second, ClipEnd: 2*time.Hour + 5*time.Second}, []string{"Clip      1:30 – 2:00:05"}, []string{"Template", "Subtitles"}},
		{"open start", queue.Item{URL: "u", ClipEnd: 45 * time.Second}, []string{"Clip      0:00 – 0:45"}, nil},
		{"fractions", queue.Item{URL: "u", ClipStart: 400 * time.Millisecond, ClipEnd: 62500 * time.Millisecond}, []string{"Clip      0:00.4 – 1:02.5"}, nil},
		{"open end", queue.Item{URL: "u", ClipStart: 30 * time.Second}, []string{"Clip      0:30 – end"}, nil},
		{"template", queue.Item{URL: "u", Template: "%(id)s.%(ext)s"}, []string{"Template  %(id)s.%(ext)s"}, []string{"Clip", "Subtitles"}},
		{"subtitles off", queue.Item{URL: "u", Subtitles: &off}, []string{"Subtitles off"}, nil},
		{"subtitles on", queue.Item{URL: "u", Subtitles: &on}, []string{"Subtitles embedded"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m Model
			text := ansi.Strip(strings.Join(m.detailFields(tc.it, 80), "\n"))
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("missing %q in:\n%s", w, text)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(text, w) {
					t.Errorf("unexpected %q in:\n%s", w, text)
				}
			}
		})
	}
}

func assertFits(t *testing.T, m Model) {
	t.Helper()
	out := m.render()
	for _, line := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(line); w > m.width {
			t.Errorf("%dx%d: line wider than terminal (%d): %q", m.width, m.height, w, line)
		}
	}
	if h := strings.Count(out, "\n") + 1; h > m.height {
		t.Errorf("%dx%d: render height %d", m.width, m.height, h)
	}
}
