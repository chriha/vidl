package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"vidl/internal/queue"
)

type tab int

const (
	tabDownloads tab = iota
	tabFinished
	tabSettings
	tabCount
)

var tabNames = [tabCount]string{"Downloads", "Finished", "Settings"}

var (
	activeTabStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	cursorStyle      = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("237"))
	dimStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

type changedMsg struct{}

// Model is the root Bubble Tea model.
type Model struct {
	mgr     *queue.Manager
	keys    keyMap
	help    help.Model
	tab     tab
	width   int
	height  int
	queue   []queue.Item
	history []queue.Item
	cursor  [tabCount]int
	err     error
}

func New(mgr *queue.Manager) Model {
	m := Model{mgr: mgr, keys: newKeyMap(), help: help.New()}
	m.refresh()
	return m
}

func (m Model) Init() tea.Cmd {
	return m.waitForChange()
}

func (m Model) waitForChange() tea.Cmd {
	ch := m.mgr.Changes()
	return func() tea.Msg {
		<-ch
		return changedMsg{}
	}
}

func (m *Model) refresh() {
	m.queue, m.history = m.mgr.Snapshot()
	for t, n := range map[tab]int{tabDownloads: len(m.queue), tabFinished: len(m.history)} {
		m.cursor[t] = min(m.cursor[t], max(n-1, 0))
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
	case changedMsg:
		m.refresh()
		return m, m.waitForChange()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.help.ShowAll = !m.help.ShowAll
	case key.Matches(msg, k.NextTab):
		m.tab = (m.tab + 1) % tabCount
	case key.Matches(msg, k.PrevTab):
		m.tab = (m.tab + tabCount - 1) % tabCount
	case key.Matches(msg, k.Tab1):
		m.tab = tabDownloads
	case key.Matches(msg, k.Tab2):
		m.tab = tabFinished
	case key.Matches(msg, k.Tab3):
		m.tab = tabSettings
	case key.Matches(msg, k.Up):
		m.moveCursor(-1)
	case key.Matches(msg, k.Down):
		m.moveCursor(1)
	case key.Matches(msg, k.Top):
		m.moveCursor(-len(m.items()))
	case key.Matches(msg, k.Bottom):
		m.moveCursor(len(m.items()))
	case key.Matches(msg, k.Start):
		m.act(m.mgr.Start)
	case key.Matches(msg, k.Pause):
		m.act(m.mgr.Pause)
	case key.Matches(msg, k.Cancel):
		m.act(m.mgr.Cancel)
	}
	return m, nil
}

// items returns the list shown on the current tab.
func (m Model) items() []queue.Item {
	switch m.tab {
	case tabDownloads:
		return m.queue
	case tabFinished:
		return m.history
	}
	return nil
}

func (m *Model) moveCursor(delta int) {
	n := len(m.items())
	if n == 0 {
		return
	}
	m.cursor[m.tab] = min(max(m.cursor[m.tab]+delta, 0), n-1)
}

func (m Model) selected() (queue.Item, bool) {
	items := m.items()
	if len(items) == 0 {
		return queue.Item{}, false
	}
	return items[m.cursor[m.tab]], true
}

func (m *Model) act(fn func(id string) error) {
	if m.tab != tabDownloads {
		return
	}
	if it, ok := m.selected(); ok {
		m.err = fn(it.ID)
	}
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "vidl"
	return v
}

func (m Model) render() string {
	if m.width == 0 {
		return ""
	}
	var tabs []string
	for t := range tabCount {
		style := inactiveTabStyle
		if t == m.tab {
			style = activeTabStyle
		}
		tabs = append(tabs, style.Render(tabNames[t]))
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	footer := m.help.View(m.keys)
	if m.err != nil {
		footer = errorStyle.Render(m.err.Error()) + "\n" + footer
	} else if err := m.mgr.SaveError(); err != nil {
		footer = errorStyle.Render("saving queue: "+err.Error()) + "\n" + footer
	}

	bodyHeight := max(m.height-lipgloss.Height(header)-lipgloss.Height(footer)-1, 1)
	var body string
	switch m.tab {
	case tabDownloads, tabFinished:
		body = m.listView(bodyHeight)
	case tabSettings:
		body = dimStyle.Render("Settings coming soon.")
	}
	body = lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// listView is a minimal placeholder list; it becomes a proper table later.
func (m Model) listView(height int) string {
	items := m.items()
	if len(items) == 0 {
		if m.tab == tabDownloads {
			return dimStyle.Render("No downloads. Run vidl <url> to queue one.")
		}
		return dimStyle.Render("Nothing finished yet.")
	}
	cur := m.cursor[m.tab]
	start := max(0, min(cur-height/2, len(items)-height))
	var b strings.Builder
	for i := start; i < min(start+height, len(items)); i++ {
		it := items[i]
		p := it.Progress
		line := fmt.Sprintf("%-11s %5.1f%%  %10s  %7s  %s",
			it.Status, p.Percent, humanSpeed(p.Speed), humanDuration(p.ETA), it.DisplayTitle())
		line = ansi.Truncate(line, m.width, "…")
		if i == cur {
			line = cursorStyle.Width(m.width).Render(line)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
