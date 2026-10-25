package ui

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/queue"
)

// dupLabel is why it makes a new download a duplicate.
func dupLabel(it queue.Item) string {
	if it.Status == queue.Finished {
		return "already downloaded"
	}
	return "in queue"
}

// markEntryDups unchecks and marks the playlist entries already downloaded
// or queued.
func (m *Model) markEntryDups() {
	d := m.dialog
	d.entryDups = make([]string, len(d.info.Entries))
	for i, e := range d.info.Entries {
		if it, ok := m.mgr.FindDuplicate(queue.NewItem{URL: e.URL, Extractor: e.Extractor, VideoID: e.ID}); ok {
			d.entryDups[i], d.checked[i] = dupLabel(it), false
		}
	}
}

// showBatch asks to add the pasted urls, queued by default, and marks the
// ones already downloaded or queued.
func (m *Model) showBatch(urls []string) {
	d := m.dialog
	d.step, d.batch, d.err, d.status = stepBatch, urls, nil, addQueued
	d.batchDups = make([]bool, len(urls))
	for i, u := range urls {
		_, d.batchDups[i] = m.mgr.FindDuplicate(queue.NewItem{URL: u})
	}
}

// batchNew is the number of pasted URLs that aren't duplicates.
func (d *newDialog) batchNew() int {
	n := 0
	for _, dup := range d.batchDups {
		if !dup {
			n++
		}
	}
	return n
}

// duplicateLines renders the warning that the probed video already exists.
func (d *newDialog) duplicateLines(w int) []string {
	it := d.dup
	bold := lipgloss.NewStyle().Bold(true)
	heading, when := "Already in the queue", joinNonEmpty(it.Status.String(), timeLabel("added", it.AddedAt))
	if it.Status == queue.Finished {
		heading, when = "Already downloaded", timeLabel("finished", it.FinishedAt)
	}
	lines := []string{
		errorStyle.Render(heading), "",
		bold.Width(w).MaxHeight(2).Render(it.DisplayTitle()),
		dimStyle.Render(ansi.Truncate(when, w, "…")),
	}
	if it.FilePath != "" {
		lines = append(lines, dimStyle.Render(ansi.Truncate(it.FilePath, w, "…")))
	}
	return append(lines, "", dimStyle.Render(ansi.Truncate("enter add anyway · esc cancel", w, "…")))
}
