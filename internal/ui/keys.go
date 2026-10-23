package ui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up, Down, Top, Bottom    key.Binding
	HalfPageDown, HalfPageUp key.Binding
	NextTab, PrevTab         key.Binding
	Tab1, Tab2, Tab3         key.Binding
	Start, Pause, Cancel     key.Binding
	Retry, Remove, New       key.Binding
	Detail, Close            key.Binding
	MoveUp, MoveDown         key.Binding
	Search, ClearFilter      key.Binding
	Open, Reveal             key.Binding
	ClearHistory             key.Binding
	Edit, Toggle             key.Binding
	Decrease, Increase       key.Binding
	Help, Quit               key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up:           key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:         key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Top:          key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:       key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d", "page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u", "page up")),
		NextTab:      key.NewBinding(key.WithKeys("tab", "l"), key.WithHelp("tab", "next tab")),
		PrevTab:      key.NewBinding(key.WithKeys("shift+tab", "h"), key.WithHelp("shift+tab", "prev tab")),
		Tab1:         key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "downloads")),
		Tab2:         key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "finished")),
		Tab3:         key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "settings")),
		Start:        key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start")),
		Pause:        key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause")),
		Cancel:       key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cancel")),
		Retry:        key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "retry")),
		Remove:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "remove")),
		New:          key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		MoveUp:       key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "move up")),
		MoveDown:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "move down")),
		Detail:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
		Close:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close details")),
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		ClearFilter:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear search")),
		Open:         key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open file")),
		Reveal:       key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "show in folder")),
		ClearHistory: key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "clear history")),
		Edit:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit")),
		Toggle:       key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
		Decrease:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "decrease")),
		Increase:     key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "increase")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// tabKeys adapts the help to the current tab.
type tabKeys struct {
	keyMap
	tab       tab
	detail    bool // detail panel open
	filtering bool // a search filter is active
}

// detailKey is enter to open the panel, or esc to close it once it's open.
func (k tabKeys) detailKey() key.Binding {
	if k.detail {
		return k.Close
	}
	return k.Detail
}

// searchKey is / to search, or esc to clear the filter when esc does that.
func (k tabKeys) searchKey() key.Binding {
	if k.filtering && !k.detail {
		return k.ClearFilter
	}
	return k.Search
}

// escKeys is esc's current meaning: closing the panel, then clearing the
// filter.
func (k tabKeys) escKeys() []key.Binding {
	if k.filtering {
		return []key.Binding{k.Close, k.ClearFilter}
	}
	return []key.Binding{k.Close}
}

func (k tabKeys) ShortHelp() []key.Binding {
	switch k.tab {
	case tabDownloads:
		return []key.Binding{k.New, k.Up, k.Down, k.detailKey(), k.Start, k.Pause, k.Cancel, k.Remove, k.MoveUp, k.MoveDown, k.searchKey(), k.NextTab, k.Help, k.Quit}
	case tabFinished:
		return []key.Binding{k.New, k.Up, k.Down, k.detailKey(), k.Open, k.Reveal, k.Remove, k.searchKey(), k.NextTab, k.Help, k.Quit}
	case tabSettings:
		return []key.Binding{k.Up, k.Down, k.Edit, k.Decrease, k.Increase, k.NextTab, k.Help, k.Quit}
	}
	return []key.Binding{k.New, k.NextTab, k.Help, k.Quit}
}

func (k tabKeys) FullHelp() [][]key.Binding {
	nav := []key.Binding{k.Up, k.Down, k.Top, k.Bottom, k.HalfPageDown, k.HalfPageUp}
	tabs := []key.Binding{k.NextTab, k.PrevTab, k.Tab1, k.Tab2, k.Tab3}
	general := []key.Binding{k.New, k.Help, k.Quit}
	switch k.tab {
	case tabDownloads:
		return [][]key.Binding{nav, append([]key.Binding{k.Detail, k.Search}, k.escKeys()...), {k.Start, k.Pause, k.Cancel, k.Retry}, {k.Remove, k.MoveUp, k.MoveDown}, tabs, general}
	case tabFinished:
		return [][]key.Binding{nav, append([]key.Binding{k.Detail, k.Search}, k.escKeys()...), {k.Open, k.Reveal, k.Remove, k.ClearHistory}, tabs, general}
	case tabSettings:
		return [][]key.Binding{{k.Up, k.Down, k.Top, k.Bottom}, {k.Edit, k.Toggle, k.Decrease, k.Increase}, {k.NextTab, k.PrevTab, k.Tab1, k.Tab2, k.Tab3}, general}
	}
	return [][]key.Binding{tabs, general}
}
