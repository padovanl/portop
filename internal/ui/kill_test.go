package ui

import (
	"errors"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestMain makes the kill dialog independent of which PIDs exist on the host:
// test rows use small fake PIDs that may or may not be real processes.
func TestMain(m *testing.M) {
	processStartTime = func(int) (time.Time, error) { return time.Unix(1700000000, 0), nil }
	os.Exit(m.Run())
}

func TestKillDialogCapturesProcessIdentity(t *testing.T) {
	m := mouseTestModel()
	model, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = model.(Model)
	if m.mode != modeConfirmKill || !m.killStart.Equal(time.Unix(1700000000, 0)) {
		t.Fatalf("mode=%v killStart=%v, want confirmation with captured start time", m.mode, m.killStart)
	}
}

func TestKillDialogRefusesUnverifiableProcess(t *testing.T) {
	old := processStartTime
	t.Cleanup(func() { processStartTime = old })
	processStartTime = func(int) (time.Time, error) { return time.Time{}, errors.New("gone") }

	m := mouseTestModel()
	model, cmd := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = model.(Model)
	if m.mode == modeConfirmKill || cmd != nil || !m.statusIsErr {
		t.Fatalf("mode=%v cmd nil=%v statusIsErr=%v, want error status and no dialog", m.mode, cmd == nil, m.statusIsErr)
	}
}
