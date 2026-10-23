package ui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up, Down, Top, Bottom    key.Binding
	HalfPageDown, HalfPageUp key.Binding
	NextTab, PrevTab         key.Binding
	Tab1, Tab2, Tab3         key.Binding
	Start, Pause, Cancel     key.Binding
	Retry, Remove            key.Binding
	Detail, Close            key.Binding
	MoveUp, MoveDown         key.Binding
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
		MoveUp:       key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "move up")),
		MoveDown:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "move down")),
		Detail:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
		Close:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close details")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// tabKeys adapts the help to the current tab.
type tabKeys struct {
	keyMap
	tab    tab
	detail bool // detail panel open
}

// detailKey is enter to open the panel, or esc to close it once it's open.
func (k tabKeys) detailKey() key.Binding {
	if k.detail {
		return k.Close
	}
	return k.Detail
}

func (k tabKeys) ShortHelp() []key.Binding {
	switch k.tab {
	case tabDownloads:
		return []key.Binding{k.Up, k.Down, k.detailKey(), k.Start, k.Pause, k.Cancel, k.Remove, k.MoveUp, k.MoveDown, k.NextTab, k.Help, k.Quit}
	case tabFinished:
		return []key.Binding{k.Up, k.Down, k.detailKey(), k.Remove, k.NextTab, k.Help, k.Quit}
	}
	return []key.Binding{k.NextTab, k.Help, k.Quit}
}

func (k tabKeys) FullHelp() [][]key.Binding {
	nav := []key.Binding{k.Up, k.Down, k.Top, k.Bottom, k.HalfPageDown, k.HalfPageUp}
	tabs := []key.Binding{k.NextTab, k.PrevTab, k.Tab1, k.Tab2, k.Tab3}
	general := []key.Binding{k.Help, k.Quit}
	switch k.tab {
	case tabDownloads:
		return [][]key.Binding{nav, {k.Detail, k.Close}, {k.Start, k.Pause, k.Cancel, k.Retry}, {k.Remove, k.MoveUp, k.MoveDown}, tabs, general}
	case tabFinished:
		return [][]key.Binding{nav, {k.Detail, k.Close, k.Remove}, tabs, general}
	}
	return [][]key.Binding{tabs, general}
}
