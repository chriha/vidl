package ui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/chriha/vidl/internal/queue"
)

// errorView is the full-screen view of why a download failed (i).
type errorView struct {
	id string         // the failed item
	vp viewport.Model // its error and saved yt-dlp output
}

// openErrorView opens the error view for the selected download if it failed.
func (m *Model) openErrorView() {
	it, ok := m.selected()
	if !ok || it.Status != queue.Failed {
		m.notice("No error to show")
		return
	}
	vp := viewport.New()
	vp.SoftWrap = true
	m.errView = &errorView{id: it.ID, vp: vp}
}

// syncErrorView sizes the error view and fills it from the latest snapshot,
// closing it once its item is gone or no longer failed.
func (m *Model) syncErrorView() {
	if m.errView == nil {
		return
	}
	i := slices.IndexFunc(m.queue, func(it queue.Item) bool { return it.ID == m.errView.id })
	if i < 0 || m.queue[i].Status != queue.Failed {
		m.errView = nil
		return
	}
	vp := &m.errView.vp
	vp.SetWidth(max(m.width-1, 1))
	vp.SetHeight(m.bodyHeight())
	vp.SetContentLines(errorViewLines(m.queue[i], vp.Width()))
}

func (m Model) handleErrorViewKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k, vp := m.keys, &m.errView.vp
	switch {
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case key.Matches(msg, k.CloseError):
		m.errView = nil
	case key.Matches(msg, k.Up):
		vp.ScrollUp(1)
	case key.Matches(msg, k.Down):
		vp.ScrollDown(1)
	case key.Matches(msg, k.Top):
		vp.GotoTop()
	case key.Matches(msg, k.Bottom):
		vp.GotoBottom()
	case key.Matches(msg, k.HalfPageDown):
		vp.HalfPageDown()
	case key.Matches(msg, k.HalfPageUp):
		vp.HalfPageUp()
	}
	return m, nil
}

// errorViewLines is the content of the error view for it, w cells wide.
func errorViewLines(it queue.Item, w int) []string {
	wrap := func(s lipgloss.Style, text string) []string {
		return strings.Split(s.Width(w).Render(text), "\n")
	}
	status := lipgloss.NewStyle().Foreground(statusColors[it.Status]).Render(it.Status.String())
	lines := wrap(lipgloss.NewStyle(), status+"  "+lipgloss.NewStyle().Bold(true).Render(it.DisplayTitle()))
	lines = append(lines, wrap(dimStyle, it.URL)...)
	lines = append(lines, "")
	lines = append(lines, wrap(errorStyle, it.Error)...)
	title := fmt.Sprintf(" yt-dlp output (%d lines) ", len(it.Log))
	lines = append(lines, "", detailRuleStyle.Render("─"+title+strings.Repeat("─", max(w-1-len(title), 0))))
	if len(it.Log) == 0 {
		return append(lines, dimStyle.Render("no output saved"))
	}
	return append(lines, it.Log...)
}
