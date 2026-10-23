package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/chriha/vidl/internal/queue"
)

// statusCount is the number of listed items with one status.
type statusCount struct {
	status queue.Status
	n      int
}

// tabCounts returns the counts shown after a tab's name: per status on
// Downloads, one total on Finished, none elsewhere. Zero counts are left out.
func (m Model) tabCounts(t tab) []statusCount {
	items := m.tabItems(t)
	switch t {
	case tabDownloads:
		var out []statusCount
		for _, st := range statusCycle {
			n := 0
			for _, it := range items {
				if it.Status == st {
					n++
				}
			}
			if n > 0 {
				out = append(out, statusCount{st, n})
			}
		}
		return out
	case tabFinished:
		if len(items) > 0 {
			return []statusCount{{queue.Finished, len(items)}}
		}
	}
	return nil
}

// tabCountsLabel renders the counts after a tab name: "(2/5/1)" with each
// number in its status colour, or "" when there are none.
func (m Model) tabCountsLabel(t tab) string {
	counts := m.tabCounts(t)
	if len(counts) == 0 {
		return ""
	}
	parts := make([]string, len(counts))
	for i, c := range counts {
		parts[i] = lipgloss.NewStyle().Foreground(statusColors[c.status]).Render(strconv.Itoa(c.n))
	}
	return dimStyle.Render("(") + strings.Join(parts, dimStyle.Render("/")) + dimStyle.Render(")")
}
