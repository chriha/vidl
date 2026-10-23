package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/notify"
	"github.com/chriha/vidl/internal/queue"
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

// Below this size the UI shows a message instead of the tabs.
const (
	minWidth  = 40
	minHeight = 8
)

var errNotRetryable = errors.New("only failed, cancelled or finished downloads can be retried")

type changedMsg struct{}

// confirmation is a pending destructive action awaiting y/n.
type confirmation struct {
	question string // "Remove download?"
	subject  string // what the action applies to
	note     string // consequences, optional
	verb     string // label of y
	action   func() error
	done     string // notice shown after the action succeeded, optional

	marked []string   // ids a bulk action applies to instead of action
	bulk   bulkAction // set for bulk actions on marked rows
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
	confirm *confirmation
	detail  bool           // detail panel open
	log     viewport.Model // yt-dlp output in the detail panel
	dialog  *newDialog     // new-download dialog, nil when closed
	toast   toast

	filter [tabCount]string // search query per tab
	status [tabCount]int    // status filter per tab: 0 all, else statusCycle[i-1]
	search *textinput.Model // filter input, nil when closed
	open   opener
	browse browser

	marks [tabCount]map[string]bool // marked item ids per tab

	settings settingsForm
	cfgPath  string // where settings are saved

	versionFn func(context.Context) (string, error) // yt-dlp version for the startup check, nil to skip
	notifier  notify.Notifier                       // desktop notifications, nil to disable
	now       func() time.Time

	ended             []queue.Item // finished or failed downloads awaiting a notification
	notifyUnavailable bool         // the notifier binary is missing; stop trying

	probeSeq  int                    // id of the latest probe request
	clipboard func() (string, error) // reads the system clipboard
}

// New returns the root model for mgr; prober fetches metadata in the
// new-download dialog, and settings changes are saved to cfgPath.
func New(mgr *queue.Manager, prober Prober, cfgPath string) Model {
	m := Model{
		mgr: mgr, prober: prober, keys: newKeyMap(), help: help.New(), log: viewport.New(),
		clipboard: clipboard.ReadAll, cfgPath: cfgPath, open: systemOpen, browse: systemBrowse, now: time.Now,
	}
	m.log.SoftWrap = true
	m.refresh()
	return m
}

// WithVersionCheck makes the model warn at startup when the yt-dlp version
// returned by fn is older than ytdlp.StaleAfter.
func (m Model) WithVersionCheck(fn func(context.Context) (string, error)) Model {
	m.versionFn = fn
	return m
}

// WithNotifier makes the model send desktop notifications through n when
// downloads finish or fail.
func (m Model) WithNotifier(n notify.Notifier) Model {
	m.notifier = n
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.waitForChange(), m.checkVersion())
}

func (m Model) waitForChange() tea.Cmd {
	ch := m.mgr.Changes()
	return func() tea.Msg {
		<-ch
		return changedMsg{}
	}
}

// refresh takes a new snapshot and keeps the cursors on their items.
func (m *Model) refresh() {
	prev := m.statuses()
	m.queue, m.history = m.mgr.Snapshot()
	m.collectEnded(prev)
	m.pruneMarks()
	m.follow()
}

// follow keeps each cursor on the same item, falling back to the same index
// when the item left the (filtered) list.
func (m *Model) follow() {
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
	if next.toast.seq != m.toast.seq && next.toast.text != "" {
		d := toastDuration
		if next.toast.long {
			d = longToastDuration
		}
		cmd = tea.Batch(cmd, expireToast(next.toast.seq, d))
	}
	if len(m.ended) == 0 && len(next.ended) > 0 {
		cmd = tea.Batch(cmd, tea.Tick(notifyWindow, func(time.Time) tea.Msg { return notifyFlushMsg{} }))
	}
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
		if m.settings.edit != nil {
			m.settings.edit.SetWidth(m.settingValueWidth() - 1)
		}
		if m.search != nil {
			m.search.SetWidth(max(m.width-4, 8))
		}
		m.scroll()
		return m, nil
	case changedMsg:
		m.refresh()
		return m, m.waitForChange()
	case toastExpiredMsg:
		if msg.seq == m.toast.seq {
			m.toast = toast{seq: m.toast.seq}
		}
		return m, nil
	case openedMsg:
		m.report(msg.err)
		return m, nil
	case versionMsg:
		m.handleVersion(msg)
		return m, nil
	case notifyFlushMsg:
		return m, m.flushNotifications()
	case notifiedMsg:
		m.handleNotified(msg.err)
		return m, nil
	case AddURLsMsg:
		m.addExternal(msg)
		return m, nil
	case ErrorMsg:
		m.report(msg.Err)
		return m, nil
	}
	if m.dialog != nil {
		return m.handleDialogMsg(msg)
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.confirm != nil {
			return m.handleConfirmKey(msg)
		}
		if m.search != nil {
			return m.handleSearchKey(msg)
		}
		if m.tab == tabSettings {
			if next, cmd, ok := m.handleSettingsKey(msg); ok {
				return next, cmd
			}
		}
		var cmd tea.Cmd
		m, cmd = m.handleKey(msg)
		m.scroll()
		return m, cmd
	default: // paste and cursor blink
		var cmd tea.Cmd
		switch {
		case m.search != nil:
			*m.search, cmd = m.search.Update(msg)
			m.setFilter(m.search.Value())
		case m.settings.edit != nil:
			*m.settings.edit, cmd = m.settings.edit.Update(msg)
		}
		return m, cmd
	}
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
		if !m.bulk(bulkStart) {
			m.act(m.mgr.Start)
		}
	case key.Matches(msg, k.Pause):
		if !m.bulk(bulkPause) {
			m.act(m.pauseOrRequeue)
		}
	case key.Matches(msg, k.Cancel):
		if !m.bulk(bulkCancel) {
			m.act(m.mgr.Cancel)
		}
	case key.Matches(msg, k.Retry):
		if !m.bulk(bulkRetry) {
			m.retrySelected()
		}
	case key.Matches(msg, k.MoveUp):
		m.act(func(id string) error { return m.mgr.Move(id, -1) })
		m.refresh()
	case key.Matches(msg, k.MoveDown):
		m.act(func(id string) error { return m.mgr.Move(id, 1) })
		m.refresh()
	case key.Matches(msg, k.Remove):
		m.askRemove()
	case key.Matches(msg, k.ClearHistory):
		m.askClearHistory()
	case key.Matches(msg, k.Open):
		return m, m.openSelected(false)
	case key.Matches(msg, k.Reveal):
		return m, m.openSelected(true)
	case key.Matches(msg, k.Browse):
		cmd := m.browseSelected() // after, so m carries any reported error
		return m, cmd
	case key.Matches(msg, k.Mark):
		m.toggleMark()
	case key.Matches(msg, k.MarkAll):
		m.markAll()
	case key.Matches(msg, k.Search):
		return m, m.openSearch()
	case key.Matches(msg, k.StatusFilter):
		m.cycleStatus()
	case key.Matches(msg, k.New):
		return m, m.openDialog()
	case key.Matches(msg, k.Detail):
		if _, ok := m.selected(); ok || m.detail {
			m.detail = !m.detail
		}
	case key.Matches(msg, k.Close):
		switch {
		case m.detail:
			m.detail = false
		case m.markCount() > 0:
			m.clearMarks()
		case m.filtering():
			m.clearFilters()
		}
	}
	return m, nil
}

func (m Model) handleConfirmKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y":
		c := m.confirm
		m.confirm = nil
		if c.bulk != nil {
			m.applyMarked(c.marked, c.bulk)
		} else if err := c.action(); err != nil {
			m.report(err)
		} else if c.done != "" {
			m.notice(c.done)
		} else {
			m.report(nil)
		}
		m.refresh()
	case "n", "N", "esc":
		m.confirm = nil
	}
	return m, nil
}

func (m *Model) askRemove() {
	if !m.hasTable() {
		return
	}
	if m.markCount() > 0 {
		m.askRemoveMarked()
		return
	}
	it, ok := m.selected()
	if !ok {
		return
	}
	mgr := m.mgr
	c := &confirmation{
		question: "Remove download?", subject: it.DisplayTitle(), verb: "remove",
		action: func() error { return mgr.Remove(it.ID) },
	}
	switch {
	case it.Status == queue.Finished:
		c.note = "The downloaded file is kept."
	case it.Status == queue.Downloading || len(it.PartFiles) > 0:
		c.note = "Partial files will be deleted."
	}
	m.confirm = c
}

// askClearHistory asks to clear the finished history, or only the items
// matching the filter while one is active.
func (m *Model) askClearHistory() {
	if m.tab != tabFinished || len(m.history) == 0 {
		return
	}
	items := m.items()
	if len(items) == 0 {
		return
	}
	mgr := m.mgr
	c := &confirmation{
		question: "Clear finished history?",
		subject:  plural(len(items), "download"),
		note:     "Downloaded files are kept.",
		verb:     "clear",
		done:     "Cleared " + plural(len(items), "download") + " from history",
		action: func() error {
			mgr.ClearHistory()
			return nil
		},
	}
	if len(items) < len(m.history) {
		c.question = "Remove matching downloads from history?"
		c.action = func() error {
			var errs []error
			for _, it := range items {
				errs = append(errs, mgr.Remove(it.ID))
			}
			return errors.Join(errs...)
		}
	}
	m.confirm = c
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// retrySelected retries a failed or cancelled download, or downloads a
// finished one again.
func (m *Model) retrySelected() {
	if m.tab != tabDownloads && m.tab != tabFinished {
		return
	}
	if it, ok := m.selected(); ok {
		m.report(m.retry(it.ID))
	}
}

func (m Model) retry(id string) error {
	it, ok := m.mgr.Get(id)
	if !ok || (it.Status != queue.Failed && it.Status != queue.Cancelled && it.Status != queue.Finished) {
		return errNotRetryable
	}
	return m.mgr.Start(id)
}

// pauseOrRequeue puts a paused download back in the queue and pauses any other.
func (m Model) pauseOrRequeue(id string) error {
	if it, ok := m.mgr.Get(id); ok && it.Status == queue.Paused {
		return m.mgr.Requeue(id)
	}
	return m.mgr.Pause(id)
}

func (m Model) tabItems(t tab) []queue.Item {
	switch t {
	case tabDownloads:
		return filterItems(filterStatus(m.queue, m.status[t]), m.filter[t])
	case tabFinished:
		return filterItems(m.history, m.filter[t])
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
		m.report(fn(it.ID))
	}
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = m.windowTitle()
	v.ProgressBar = m.progressBar()
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
		if label := m.tabCountsLabel(t); label != "" {
			tabs = append(tabs, label+" ")
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m Model) footer() string {
	_, detail := m.detailItem()
	footer := m.statusLine() + "\n" + m.help.View(tabKeys{m.keys, m.tab, detail, m.filtering(), m.markCount() > 0})
	if err := m.mgr.SaveError(); err != nil {
		footer = errorStyle.Render(" saving queue: "+err.Error()) + "\n" + footer
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
	rows := m.bodyHeight() - m.detailHeight() - 1
	if m.filtering() {
		rows--
	}
	return max(rows, 1)
}

func (m Model) render() string {
	if m.width == 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return m.tooSmallView()
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
		body = m.settingsView(bodyHeight)
	}
	if panel := m.detailView(); panel != "" {
		tableH := bodyHeight - m.detailHeight()
		body = lipgloss.NewStyle().Height(tableH).MaxHeight(tableH).Render(body) + "\n" + panel
	}
	if m.filtering() {
		body = m.filterLine() + "\n" + body
	}
	body = lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(body)
	if t := m.toastView(); t != "" {
		body = lipgloss.NewCompositor(
			lipgloss.NewLayer(body),
			lipgloss.NewLayer(t).X(max(m.width-lipgloss.Width(t)-1, 0)).Y(max(bodyHeight-lipgloss.Height(t), 0)).Z(2),
		).Render()
	}
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
	all := m.queue
	if m.tab == tabFinished {
		all = m.history
	}
	off, cur := m.offset[m.tab], m.cursor[m.tab]
	for i := off; i < min(off+m.tableRows(), len(items)); i++ {
		base := lipgloss.NewStyle()
		if i == cur {
			base = cursorStyle
		}
		// Number rows by their position in the whole list, not the filtered one.
		pos := slices.IndexFunc(all, func(it queue.Item) bool { return it.ID == items[i].ID })
		line := renderRow(cols, widths, cells(pos, items[i]), base, m.width)
		if m.marks[m.tab][items[i].ID] {
			line = markStyle.Inherit(base).Render(markGlyph) + ansi.Cut(line, 1, m.width)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) confirmView() string {
	c := m.confirm
	w := min(60, max(m.width-4, 20))
	lines := []string{
		lipgloss.NewStyle().Bold(true).Render(c.question),
		lipgloss.NewStyle().Width(w - 4).MaxHeight(3).Render(c.subject),
	}
	if c.note != "" {
		lines = append(lines, dimStyle.Render(c.note))
	}
	lines = append(lines, "", "y "+c.verb+" · n/esc keep")
	return dialogStyle.Width(w).Render(strings.Join(lines, "\n"))
}

// overlay draws box centered on top of body.
func (m Model) overlay(body, box string, height int) string {
	x := max((m.width-lipgloss.Width(box))/2, 0)
	y := max((height-lipgloss.Height(box))/2, 0)
	out := lipgloss.NewCompositor(
		lipgloss.NewLayer(body),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
	return lipgloss.NewStyle().MaxHeight(height).Render(out)
}

func (m Model) tooSmallView() string {
	msg := fmt.Sprintf("Terminal too small\nneed %d×%d, have %d×%d", minWidth, minHeight, m.width, m.height)
	lines := strings.Split(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dimStyle.Render(msg)), "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "")
	}
	return strings.Join(lines, "\n")
}
