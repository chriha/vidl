package ui

import (
	"testing"

	"vidl/internal/queue"
	"vidl/internal/ytdlp"
)

func TestSummary(t *testing.T) {
	items := []queue.Item{
		{Status: queue.Downloading, Progress: ytdlp.Progress{Percent: 20, Speed: 1000}},
		{Status: queue.Downloading, Progress: ytdlp.Progress{Percent: 60, Speed: 500}},
		{Status: queue.Queued},
		{Status: queue.Queued},
		{Status: queue.Failed, Progress: ytdlp.Progress{Percent: 10}},
	}
	s := summarize(items)
	if s.speed != 1500 || s.percent != 40 {
		t.Errorf("speed %v percent %v, want 1500 and 40", s.speed, s.percent)
	}
	if got, want := s.countsLabel(), "2 downloading · 2 queued · 1 failed"; got != want {
		t.Errorf("counts %q, want %q", got, want)
	}
	if got := summarize(nil).countsLabel(); got != "" {
		t.Errorf("empty counts %q", got)
	}
}

func TestWindowTitleAndProgressBar(t *testing.T) {
	m, mgr, _ := newTestModel(t, 1, "a", "b")
	waitFor(t, "a running", func() bool { q, _ := mgr.Snapshot(); return q[0].Progress.Percent == 42 })
	m = update(t, m, changedMsg{})
	if got, want := m.windowTitle(), "vidl · 42% · 1 downloading"; got != want {
		t.Errorf("title %q, want %q", got, want)
	}
	if pb := m.progressBar(); pb == nil || pb.Value != 42 {
		t.Errorf("progress bar %+v, want 42", pb)
	}

	idle, _, _ := newTestModel(t, 1)
	if idle.windowTitle() != "vidl" || idle.progressBar() != nil {
		t.Errorf("idle: title %q, bar %+v", idle.windowTitle(), idle.progressBar())
	}
}
