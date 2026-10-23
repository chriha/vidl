package ui

import (
	"errors"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
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
	dialogStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("9")).Padding(0, 1)
)

var errNotRetryable = errors.New("only failed or cancelled downloads can be retried")

type changedMsg struct{}

// confirmRemove is the pending removal shown in the confirmation dialog.
type confirmRemove struct {
	id    string
	title string
	note  string
}

// Model is the root Bubble Tea model.
type Model struct {
	mgr     *queue.Manager
	prober  Prober
	keys    keyMap
	help    help.Model
	tab     tab
	width   int
	height  int
	queue   []queue.Item
	history []queue.Item
	cursor  [tabCount]int
	sel     [tabCount]string // id under the cursor, followed across refreshes
	offset  [tabCount]int    // first visible row
	confirm *confirmRemove
	detail  bool           // detail panel open
	log     viewport.Model // yt-dlp output in the detail panel
	dialog  *newDialog     // new-download dialog, nil when closed
	err     error

	probeSeq  int                    // id of the latest probe request
	clipboard func() (string, error) // reads the system clipboard
}

// New returns the root model for mgr; prober fetches metadata in the
// new-download dialog.
func New(mgr *queue.Manager, prober Prober) Model {
	m := Model{
		mgr: mgr, prober: prober, keys: newKeyMap(), help: help.New(), log: viewport.New(),
		clipboard: clipboard.ReadAll,
	}
	m.log.SoftWrap = true
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

// refresh takes a new snapshot and keeps each cursor on the same item,
// falling back to the same index when the item left the list.
func (m *Model) refresh() {
	m.queue, m.history = m.mgr.Snapshot()
	for _, t := range []tab{tabDownloads, tabFinished} {
		items := m.tabItems(t)
		if i := slices.IndexFunc(items, func(it queue.Item) bool { return it.ID == m.sel[t] }); i >= 0 {
			m.cursor[t] = i
		} else {
			m.cursor[t] = min(m.cursor[t], max(len(items)-1, 0))
		}
		m.sel[t] = ""
		if len(items) > 0 {
			m.sel[t] = items[m.cursor[t]].ID
		}
	}
	m.scroll()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	next.syncLog()
	return next, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		if m.dialog != nil {
			m.dialog.input.SetWidth(m.dialogWidth() - 8)
		}
		m.scroll()
		return m, nil
	case changedMsg:
		m.refresh()
		return m, m.waitForChange()
	}
	if m.dialog != nil {
		return m.handleDialogMsg(msg)
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.confirm != nil {
			return m.handleConfirmKey(msg)
		}
		var cmd tea.Cmd
		m, cmd = m.handleKey(msg)
		m.scroll()
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
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
	case key.Matches(msg, k.HalfPageDown):
		m.moveCursor(max(m.tableRows()/2, 1))
	case key.Matches(msg, k.HalfPageUp):
		m.moveCursor(-max(m.tableRows()/2, 1))
	case key.Matches(msg, k.Start):
		m.act(m.mgr.Start)
	case key.Matches(msg, k.Pause):
		m.act(m.mgr.Pause)
	case key.Matches(msg, k.Cancel):
		m.act(m.mgr.Cancel)
	case key.Matches(msg, k.Retry):
		m.act(m.retry)
	case key.Matches(msg, k.MoveUp):
		m.act(func(id string) error { return m.mgr.Move(id, -1) })
		m.refresh()
	case key.Matches(msg, k.MoveDown):
		m.act(func(id string) error { return m.mgr.Move(id, 1) })
		m.refresh()
	case key.Matches(msg, k.Remove):
		m.askRemove()
	case key.Matches(msg, k.New):
		return m, m.openDialog()
	case key.Matches(msg, k.Detail):
		if _, ok := m.selected(); ok || m.detail {
			m.detail = !m.detail
		}
	case key.Matches(msg, k.Close):
		m.detail = false
	}
	return m, nil
}

func (m Model) handleConfirmKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y":
		m.err = m.mgr.Remove(m.confirm.id)
		m.confirm = nil
		m.refresh()
	case "n", "N", "esc":
		m.confirm = nil
	}
	return m, nil
}

func (m *Model) askRemove() {
	if m.tab != tabDownloads && m.tab != tabFinished {
		return
	}
	it, ok := m.selected()
	if !ok {
		return
	}
	c := &confirmRemove{id: it.ID, title: it.DisplayTitle()}
	switch {
	case it.Status == queue.Finished:
		c.note = "The downloaded file is kept."
	case it.Status == queue.Downloading || len(it.PartFiles) > 0:
		c.note = "Partial files will be deleted."
	}
	m.confirm = c
}

func (m Model) retry(id string) error {
	it, ok := m.mgr.Get(id)
	if !ok || (it.Status != queue.Failed && it.Status != queue.Cancelled) {
		return errNotRetryable
	}
	return m.mgr.Start(id)
}

func (m Model) tabItems(t tab) []queue.Item {
	switch t {
	case tabDownloads:
		return m.queue
	case tabFinished:
		return m.history
	}
	return nil
}

// items returns the list shown on the current tab.
func (m Model) items() []queue.Item {
	return m.tabItems(m.tab)
}

func (m *Model) moveCursor(delta int) {
	items := m.items()
	if len(items) == 0 {
		return
	}
	m.cursor[m.tab] = min(max(m.cursor[m.tab]+delta, 0), len(items)-1)
	m.sel[m.tab] = items[m.cursor[m.tab]].ID
}

// scroll adjusts each tab's offset so its cursor stays visible.
func (m *Model) scroll() {
	rows := m.tableRows()
	for _, t := range []tab{tabDownloads, tabFinished} {
		off, cur := m.offset[t], m.cursor[t]
		off = min(off, cur)
		if cur >= off+rows {
			off = cur - rows + 1
		}
		m.offset[t] = min(max(off, 0), max(len(m.tabItems(t))-rows, 0))
	}
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

func (m Model) tabBar() string {
	var tabs []string
	for t := range tabCount {
		style := inactiveTabStyle
		if t == m.tab {
			style = activeTabStyle
		}
		tabs = append(tabs, style.Render(tabNames[t]))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m Model) footer() string {
	_, detail := m.detailItem()
	footer := m.help.View(tabKeys{m.keys, m.tab, detail})
	if m.err != nil {
		footer = errorStyle.Render(m.err.Error()) + "\n" + footer
	} else if err := m.mgr.SaveError(); err != nil {
		footer = errorStyle.Render("saving queue: "+err.Error()) + "\n" + footer
	}
	// help.Model can overshoot its width by one item, so never trust it to fit.
	lines := strings.Split(footer, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "…")
	}
	return strings.Join(lines, "\n")
}

func (m Model) bodyHeight() int {
	return max(m.height-lipgloss.Height(m.tabBar())-lipgloss.Height(m.footer())-1, 1)
}

// tableRows is the number of item rows visible below the table header.
func (m Model) tableRows() int {
	return max(m.bodyHeight()-m.detailHeight()-1, 1)
}

func (m Model) render() string {
	if m.width == 0 {
		return ""
	}
	header, footer := m.tabBar(), m.footer()
	bodyHeight := m.bodyHeight()
	var body string
	switch m.tab {
	case tabDownloads:
		body = m.tableView(downloadColumns, downloadCells, "No downloads. Press n to add one.")
	case tabFinished:
		body = m.tableView(finishedColumns, finishedCells, "Nothing finished yet.")
	case tabSettings:
		body = dimStyle.Render("Settings coming soon.")
	}
	if panel := m.detailView(); panel != "" {
		tableH := bodyHeight - m.detailHeight()
		body = lipgloss.NewStyle().Height(tableH).MaxHeight(tableH).Render(body) + "\n" + panel
	}
	body = lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(body)
	if m.confirm != nil {
		body = m.overlay(body, m.confirmView(), bodyHeight)
	}
	if m.dialog != nil {
		body = m.overlay(body, m.dialogView(), bodyHeight)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) tableView(cols []column, cells func(int, queue.Item) []cell, empty string) string {
	items := m.items()
	if len(items) == 0 {
		return dimStyle.Render(" " + empty)
	}
	widths := layout(cols, m.width)
	lines := []string{headerRow(cols, widths, m.width)}
	off, cur := m.offset[m.tab], m.cursor[m.tab]
	for i := off; i < min(off+m.tableRows(), len(items)); i++ {
		base := lipgloss.NewStyle()
		if i == cur {
			base = cursorStyle
		}
		lines = append(lines, renderRow(cols, widths, cells(i, items[i]), base, m.width))
	}
	return strings.Join(lines, "\n")
}

func (m Model) confirmView() string {
	c := m.confirm
	w := min(60, max(m.width-4, 20))
	lines := []string{
		lipgloss.NewStyle().Bold(true).Render("Remove download?"),
		lipgloss.NewStyle().Width(w - 4).MaxHeight(3).Render(c.title),
	}
	if c.note != "" {
		lines = append(lines, dimStyle.Render(c.note))
	}
	lines = append(lines, "", "y remove · n/esc keep")
	return dialogStyle.Width(w).Render(strings.Join(lines, "\n"))
}

// overlay draws box centered on top of body.
func (m Model) overlay(body, box string, height int) string {
	x := max((m.width-lipgloss.Width(box))/2, 0)
	y := max((height-lipgloss.Height(box))/2, 0)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(body),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}
