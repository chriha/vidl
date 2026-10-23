package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/queue"
)

var searchPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)

// matches reports whether it's title or URL contains the lowercase query.
func matches(it queue.Item, query string) bool {
	return strings.Contains(strings.ToLower(it.Title), query) ||
		strings.Contains(strings.ToLower(it.URL), query)
}

func filterItems(items []queue.Item, query string) []queue.Item {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return items
	}
	var out []queue.Item
	for _, it := range items {
		if matches(it, query) {
			out = append(out, it)
		}
	}
	return out
}

// hasTable reports whether the current tab lists downloads.
func (m Model) hasTable() bool {
	return m.tab == tabDownloads || m.tab == tabFinished
}

// filtering reports whether the current tab shows the filter line.
func (m Model) filtering() bool {
	return m.hasTable() && (m.search != nil || strings.TrimSpace(m.filter[m.tab]) != "")
}

// openSearch shows the filter input for the current tab, starting from its
// current filter.
func (m *Model) openSearch() tea.Cmd {
	if !m.hasTable() {
		return nil
	}
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "title or URL"
	in.SetValue(m.filter[m.tab])
	in.CursorEnd()
	in.SetWidth(max(m.width-4, 8))
	m.search = &in
	return m.search.Focus()
}

// setFilter filters the current tab by query, keeping the cursor on the
// selected item while it still matches.
func (m *Model) setFilter(query string) {
	m.filter[m.tab] = query
	m.follow()
	m.scroll()
}

// handleSearchKey edits the filter while the input is open: enter keeps the
// filter, esc clears it.
func (m Model) handleSearchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.search = nil
		m.scroll()
	case "esc":
		m.search = nil
		m.setFilter("")
	default:
		var cmd tea.Cmd
		*m.search, cmd = m.search.Update(msg)
		m.setFilter(m.search.Value())
		return m, cmd
	}
	return m, nil
}

// filterLine is the line above the table: the filter input while editing,
// otherwise the active filter and how many items match.
func (m Model) filterLine() string {
	if m.search != nil {
		return ansi.Truncate(searchPromptStyle.Render(" / ")+m.search.View(), m.width, "…")
	}
	total := len(m.queue)
	if m.tab == tabFinished {
		total = len(m.history)
	}
	line := searchPromptStyle.Render(" / ") + m.filter[m.tab] +
		dimStyle.Render(fmt.Sprintf("  %d of %d · esc clear", len(m.items()), total))
	return ansi.Truncate(line, m.width, "…")
}
