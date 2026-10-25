package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/chriha/vidl/internal/queue"
)

// summary aggregates the queue for the status line and the window title.
type summary struct {
	counts  map[queue.Status]int
	speed   float64 // bytes per second of all running downloads
	percent float64 // mean progress of the running downloads
}

func summarize(items []queue.Item) summary {
	s := summary{counts: map[queue.Status]int{}}
	for _, it := range items {
		s.counts[it.Status]++
		if it.Status == queue.Downloading {
			s.speed += it.Progress.Speed
			s.percent += it.Progress.Percent
		}
	}
	if n := s.counts[queue.Downloading]; n > 0 {
		s.percent /= float64(n)
	}
	return s
}

// countsLabel is e.g. "2 downloading · 5 queued · 1 failed".
func (s summary) countsLabel() string {
	var parts []string
	for _, st := range []queue.Status{queue.Downloading, queue.Queued, queue.Paused, queue.Failed, queue.Cancelled} {
		if n := s.counts[st]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, st))
		}
	}
	return strings.Join(parts, " · ")
}

// statusLine shows the total speed, the number of downloads per status and
// the number of marked rows on the current tab.
func (m Model) statusLine() string {
	s := summarize(m.queue)
	counts := s.countsLabel()
	var line string
	switch {
	case counts == "":
		line = dimStyle.Render(" No active downloads")
	case s.speed > 0:
		speed := lipgloss.NewStyle().Foreground(statusColors[queue.Downloading]).Render("↓ " + humanSpeed(s.speed))
		line = " " + speed + dimStyle.Render(" · ") + counts
	default:
		line = " " + counts
	}
	if marks := m.marksLabel(); marks != "" {
		line += dimStyle.Render(" · ") + marks
	}
	return line
}

// windowTitle includes the overall progress while downloading.
func (m Model) windowTitle() string {
	s := summarize(m.queue)
	if n := s.counts[queue.Downloading]; n > 0 {
		return fmt.Sprintf("vidl · %.0f%% · %d downloading", s.percent, n)
	}
	return "vidl"
}

// progressBar is the terminal-native progress bar (OSC 9;4): the overall
// progress while downloading, nil (hidden) otherwise.
func (m Model) progressBar() *tea.ProgressBar {
	s := summarize(m.queue)
	if s.counts[queue.Downloading] == 0 {
		return nil
	}
	return tea.NewProgressBar(tea.ProgressBarDefault, int(s.percent))
}
