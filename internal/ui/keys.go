package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up          key.Binding
	Down        key.Binding
	Top         key.Binding
	Bottom      key.Binding
	Enter       key.Binding
	Kill        key.Binding
	Open        key.Binding
	Filter      key.Binding
	Sort        key.Binding
	Protocol    key.Binding
	Established key.Binding
	NewMark     key.Binding
	Copy        key.Binding
	Refresh     key.Binding
	Help        key.Binding
	Quit        key.Binding
	Escape      key.Binding
}

var keys = keyMap{
	Up:          key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
	Down:        key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
	Top:         key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
	Bottom:      key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
	Enter:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "process details")),
	Kill:        key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "kill")),
	Open:        key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open URL")),
	Filter:      key.NewBinding(key.WithKeys("f", "/"), key.WithHelp("f", "filter/search")),
	Sort:        key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
	Protocol:    key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "IPv4/IPv6")),
	Established: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "established")),
	NewMark:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "clear new-port marks")),
	Copy:        key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy")),
	Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Escape:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
}
