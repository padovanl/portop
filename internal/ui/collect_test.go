package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A scan slower than the refresh interval must not queue another scan on
// every tick: only one runs at a time, and the next starts after it ends.
func TestOnlyOneScanRunsAtATime(t *testing.T) {
	m := New(Config{})
	if cmd := m.startCollect(); cmd != nil {
		t.Fatal("started a second scan while Init's scan is running")
	}

	model, _ := m.Update(tickMsg{})
	m = model.(Model)
	model, _ = m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = model.(Model)
	if cmd := m.startCollect(); cmd != nil {
		t.Fatal("a tick or a refresh started a scan while one was running")
	}

	model, _ = m.Update(collectResultMsg{})
	m = model.(Model)
	if cmd := m.startCollect(); cmd == nil {
		t.Fatal("no scan started after the previous one finished")
	}
}
