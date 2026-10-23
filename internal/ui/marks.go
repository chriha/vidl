package ui

import (
	"fmt"
	"slices"

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
