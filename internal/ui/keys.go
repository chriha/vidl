package ui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up, Down, Top, Bottom key.Binding
	NextTab, PrevTab      key.Binding
	Tab1, Tab2, Tab3      key.Binding
	Start, Pause, Cancel  key.Binding
	Help, Quit            key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up:      key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:    key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Top:     key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:  key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		NextTab: key.NewBinding(key.WithKeys("tab", "l"), key.WithHelp("tab", "next tab")),
		PrevTab: key.NewBinding(key.WithKeys("shift+tab", "h"), key.WithHelp("shift+tab", "prev tab")),
		Tab1:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "downloads")),
		Tab2:    key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "finished")),
		Tab3:    key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "settings")),
		Start:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start")),
		Pause:   key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause")),
		Cancel:  key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cancel")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Start, k.Pause, k.Cancel, k.NextTab, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Start, k.Pause, k.Cancel},
		{k.NextTab, k.PrevTab, k.Tab1, k.Tab2, k.Tab3},
		{k.Help, k.Quit},
	}
}
