package ui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/chriha/vidl/internal/queue"
)

// markGlyph replaces the leading gap of a marked row.
const markGlyph = "▌"

var markStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))

// markCount is the number of marked rows on the current tab.
func (m Model) markCount() int {
	return len(m.marks[m.tab])
}

func (m *Model) setMark(t tab, id string, on bool) {
	if !on {
		delete(m.marks[t], id)
		return
	}
	if m.marks[t] == nil {
		m.marks[t] = map[string]bool{}
	}
	m.marks[t][id] = true
}

// toggleMark marks or unmarks the selected row and moves the cursor down.
func (m *Model) toggleMark() {
	if !m.hasTable() {
		return
	}
	it, ok := m.selected()
	if !ok {
		return
	}
	m.setMark(m.tab, it.ID, !m.marks[m.tab][it.ID])
	m.moveCursor(1)
}

// markAll marks every visible row, or unmarks them all when they're all
// marked already.
func (m *Model) markAll() {
	if !m.hasTable() {
		return
	}
	items := m.items()
	all := !slices.ContainsFunc(items, func(it queue.Item) bool { return !m.marks[m.tab][it.ID] })
	for _, it := range items {
		m.setMark(m.tab, it.ID, !all)
	}
}

func (m *Model) clearMarks() {
	m.marks[m.tab] = nil
}

// pruneMarks drops the marks of items that left their tab.
func (m *Model) pruneMarks() {
	for t, items := range map[tab][]queue.Item{tabDownloads: m.queue, tabFinished: m.history} {
		if len(m.marks[t]) == 0 {
			continue
		}
		ids := make(map[string]bool, len(items))
		for _, it := range items {
			ids[it.ID] = true
		}
		for id := range m.marks[t] {
			if !ids[id] {
				delete(m.marks[t], id)
			}
		}
	}
}

// marksLabel is e.g. "3 marked", or "" without marks on the current tab.
func (m Model) marksLabel() string {
	if !m.hasTable() || m.markCount() == 0 {
		return ""
	}
	return markStyle.Render(fmt.Sprintf("%d marked", m.markCount()))
}

// bulkAction applies an action to one marked item. It returns what it did,
// e.g. "paused", or "" when the item's status doesn't support the action.
type bulkAction func(mgr *queue.Manager, it queue.Item) (string, error)

func bulkStart(mgr *queue.Manager, it queue.Item) (string, error) {
	switch it.Status {
	case queue.Queued, queue.Paused, queue.Failed, queue.Cancelled:
		return "started", mgr.Start(it.ID)
	}
	return "", nil
}

func bulkPause(mgr *queue.Manager, it queue.Item) (string, error) {
	switch it.Status {
	case queue.Queued, queue.Downloading:
		return "paused", mgr.Pause(it.ID)
	case queue.Paused:
		return "requeued", mgr.Requeue(it.ID)
	}
	return "", nil
}

func bulkCancel(mgr *queue.Manager, it queue.Item) (string, error) {
	switch it.Status {
	case queue.Queued, queue.Downloading, queue.Paused, queue.Failed:
		return "cancelled", mgr.Cancel(it.ID)
	}
	return "", nil
}

func bulkRetry(mgr *queue.Manager, it queue.Item) (string, error) {
	switch it.Status {
	case queue.Failed, queue.Cancelled:
		return "retried", mgr.Start(it.ID)
	}
	return "", nil
}

func bulkRemove(mgr *queue.Manager, it queue.Item) (string, error) {
	return "removed", mgr.Remove(it.ID)
}

// markedItems returns the marked items on the current tab in list order,
// including those a filter hides.
func (m Model) markedItems() []queue.Item {
	all := m.queue
	if m.tab == tabFinished {
		all = m.history
	}
	var out []queue.Item
	for _, it := range all {
		if m.marks[m.tab][it.ID] {
			out = append(out, it)
		}
	}
	return out
}

// bulk applies act to the marked rows of the Downloads tab and reports
// whether there were any; without marks the caller acts on the selected row.
func (m *Model) bulk(act bulkAction) bool {
	if m.tab != tabDownloads || m.markCount() == 0 {
		return false
	}
	ids := make([]string, 0, m.markCount())
	for _, it := range m.markedItems() {
		ids = append(ids, it.ID)
	}
	m.applyMarked(ids, act)
	return true
}

// applyMarked runs act on the items with the given ids and shows a summary,
// e.g. "paused 3 · skipped 1", with the first error if any failed.
func (m *Model) applyMarked(ids []string, act bulkAction) {
	var verbs []string
	counts := map[string]int{}
	skipped, failed := 0, 0
	var first error
	for _, id := range ids {
		verb, err := "", error(queue.ErrNotFound)
		if it, ok := m.mgr.Get(id); ok {
			verb, err = act(m.mgr, it)
		}
		switch {
		case err != nil:
			failed++
			if first == nil {
				first = err
			}
		case verb == "":
			skipped++
		default:
			if counts[verb] == 0 {
				verbs = append(verbs, verb)
			}
			counts[verb]++
		}
	}
	var parts []string
	for _, v := range verbs {
		parts = append(parts, fmt.Sprintf("%s %d", v, counts[v]))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("skipped %d", skipped))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("failed %d", failed))
	}
	summary := strings.Join(parts, " · ")
	if first != nil {
		m.report(fmt.Errorf("%s: %w", summary, first))
		return
	}
	m.notice(summary)
}

// askRemoveMarked asks once to remove every marked row on the current tab.
func (m *Model) askRemoveMarked() {
	items := m.markedItems()
	ids := make([]string, len(items))
	titles := make([]string, len(items))
	for i, it := range items {
		ids[i], titles[i] = it.ID, it.DisplayTitle()
	}
	c := &confirmation{
		question: "Remove " + plural(len(items), "download") + "?",
		subject:  strings.Join(titles, ", "),
		verb:     "remove",
		marked:   ids,
		bulk:     bulkRemove,
	}
	switch {
	case m.tab == tabFinished:
		c.note = "The downloaded files are kept."
	case slices.ContainsFunc(items, func(it queue.Item) bool { return it.Status == queue.Downloading || len(it.PartFiles) > 0 }):
		c.note = "Partial files will be deleted."
	}
	m.confirm = c
}
