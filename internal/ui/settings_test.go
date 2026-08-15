package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/padovanl/portop/internal/config"
)

func resetGlobalUIState() {
	ApplyPalette(Themes["default"])
	ApplyKeyBindings(nil)
}

func newSettingsModel(t *testing.T, configPath string) Model {
	t.Helper()
	resetGlobalUIState()
	t.Cleanup(resetGlobalUIState)
	m := New(Config{ConfigPath: configPath, Theme: "default"})
	m.mode = modeSettings
	m.settingsCursor = settingsThemeRow
	return m
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestSettingsNavigationClampsAtEnds(t *testing.T) {
	m := newSettingsModel(t, "")

	mm, _ := m.handleSettingsKey(keyMsg("up"))
	m = mm.(Model)
	if m.settingsCursor != settingsThemeRow {
		t.Errorf("cursor = %d, want to stay at theme row (0) when already at top", m.settingsCursor)
	}

	for i := 0; i < settingsRowCount()+5; i++ {
		mm, _ := m.handleSettingsKey(keyMsg("down"))
		m = mm.(Model)
	}
	if m.settingsCursor != settingsRowCount()-1 {
		t.Errorf("cursor = %d, want clamped to last row (%d)", m.settingsCursor, settingsRowCount()-1)
	}
}

func TestSettingsThemeCycleAppliesPaletteLive(t *testing.T) {
	m := newSettingsModel(t, "")

	mm, _ := m.handleSettingsKey(keyMsg("right"))
	m = mm.(Model)
	wantIdx := 1 % len(ThemeNames)
	if m.settingsThemeIdx != wantIdx {
		t.Fatalf("settingsThemeIdx = %d, want %d", m.settingsThemeIdx, wantIdx)
	}
	if colorAccent.Dark != Themes[ThemeNames[wantIdx]].AccentDark {
		t.Errorf("palette not applied live: colorAccent.Dark = %q, want %q", colorAccent.Dark, Themes[ThemeNames[wantIdx]].AccentDark)
	}

	// left wraps back around past 0
	for i := 0; i < len(ThemeNames); i++ {
		mm, _ := m.handleSettingsKey(keyMsg("left"))
		m = mm.(Model)
	}
	if m.settingsThemeIdx != wantIdx {
		t.Errorf("after a full left cycle, settingsThemeIdx = %d, want back to %d", m.settingsThemeIdx, wantIdx)
	}
}

func TestSettingsLeftRightNoOpOffThemeRow(t *testing.T) {
	m := newSettingsModel(t, "")
	m.settingsCursor = 1 // first keybinding row, not the theme row

	mm, _ := m.handleSettingsKey(keyMsg("right"))
	m = mm.(Model)
	if m.settingsThemeIdx != 0 {
		t.Errorf("left/right off the theme row should be a no-op, settingsThemeIdx = %d", m.settingsThemeIdx)
	}
}

func TestSettingsRebindCapturesNextKey(t *testing.T) {
	m := newSettingsModel(t, "")
	killRow := 1 + indexOfAction(t, "kill")
	m.settingsCursor = killRow

	mm, _ := m.handleSettingsKey(keyMsg("enter"))
	m = mm.(Model)
	if !m.settingsCapturing {
		t.Fatal("expected settingsCapturing = true after enter on a keybinding row")
	}

	mm, _ = m.handleSettingsKey(keyMsg("x"))
	m = mm.(Model)
	if m.settingsCapturing {
		t.Error("settingsCapturing should be false after a key was captured")
	}
	if got := keys.Kill.Keys(); len(got) != 1 || got[0] != "x" {
		t.Errorf("keys.Kill.Keys() = %v, want [x]", got)
	}
	if got := m.keyOverrides["kill"]; len(got) != 1 || got[0] != "x" {
		t.Errorf("keyOverrides[kill] = %v, want [x]", got)
	}
}

func TestSettingsEscCancelsCapture(t *testing.T) {
	m := newSettingsModel(t, "")
	m.settingsCursor = 1 + indexOfAction(t, "kill")

	mm, _ := m.handleSettingsKey(keyMsg("enter"))
	m = mm.(Model)
	mm, _ = m.handleSettingsKey(keyMsg("esc"))
	m = mm.(Model)

	if m.settingsCapturing {
		t.Error("esc should cancel capture")
	}
	if got := keys.Kill.Keys(); len(got) != 1 || got[0] != "k" {
		t.Errorf("keys.Kill.Keys() = %v, want unchanged default [k]", got)
	}
}

func TestSettingsResetRestoresDefaults(t *testing.T) {
	m := newSettingsModel(t, "")
	m.keyOverrides = map[string][]string{"kill": {"x"}}
	ApplyKeyBindings(m.keyOverrides)
	m.settingsCursor = settingsResetRow()

	mm, _ := m.handleSettingsKey(keyMsg("enter"))
	m = mm.(Model)

	if len(m.keyOverrides) != 0 {
		t.Errorf("keyOverrides = %v, want empty after reset", m.keyOverrides)
	}
	if got := keys.Kill.Keys(); len(got) != 1 || got[0] != "k" {
		t.Errorf("keys.Kill.Keys() = %v, want default [k] after reset", got)
	}
}

func TestSettingsEscClosesWhenNotCapturing(t *testing.T) {
	m := newSettingsModel(t, "")
	mm, _ := m.handleSettingsKey(keyMsg("esc"))
	m = mm.(Model)
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal after esc", m.mode)
	}
}

func TestSettingsPersistsThemeAndKeybindingsToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	// Pre-existing field the settings screen doesn't know about, must survive.
	if err := os.WriteFile(path, []byte("refresh_interval: 5s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newSettingsModel(t, path)

	mm, _ := m.handleSettingsKey(keyMsg("right"))
	m = mm.(Model)

	saved, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if saved.Theme != ThemeNames[m.settingsThemeIdx] {
		t.Errorf("persisted Theme = %q, want %q", saved.Theme, ThemeNames[m.settingsThemeIdx])
	}
	if saved.RefreshInterval != "5s" {
		t.Errorf("persisted RefreshInterval = %q, want untouched 5s", saved.RefreshInterval)
	}

	killRow := 1 + indexOfAction(t, "kill")
	m.settingsCursor = killRow
	mm, _ = m.handleSettingsKey(keyMsg("enter"))
	m = mm.(Model)
	mm, _ = m.handleSettingsKey(keyMsg("x"))
	m = mm.(Model)

	saved, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := saved.Keybindings["kill"]; len(got) != 1 || got[0] != "x" {
		t.Errorf("persisted Keybindings[kill] = %v, want [x]", got)
	}
}

func TestOpenSettingsFromNormalMode(t *testing.T) {
	resetGlobalUIState()
	t.Cleanup(resetGlobalUIState)
	m := New(Config{})

	mm, _ := m.handleNormalKey(keyMsg(","))
	m = mm.(Model)
	if m.mode != modeSettings {
		t.Errorf("mode = %v, want modeSettings after \",\"", m.mode)
	}
	if m.settingsCursor != settingsThemeRow {
		t.Errorf("settingsCursor = %d, want reset to theme row (0) on open", m.settingsCursor)
	}
}

func indexOfAction(t *testing.T, action string) int {
	t.Helper()
	for i, a := range KeyActions {
		if a == action {
			return i
		}
	}
	t.Fatalf("action %q not found in KeyActions", action)
	return -1
}
