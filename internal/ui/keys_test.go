package ui

import "testing"

func TestApplyKeyBindingsOverridesOnlyGivenActions(t *testing.T) {
	defer ApplyKeyBindings(nil) // restore defaults for other tests

	ApplyKeyBindings(map[string][]string{
		"kill": {"x"},
	})

	if got := keys.Kill.Keys(); len(got) != 1 || got[0] != "x" {
		t.Errorf("Kill.Keys() = %v, want [x]", got)
	}
	// Everything else should still be the default.
	if got := keys.Quit.Keys(); len(got) != 2 || got[0] != "q" || got[1] != "ctrl+c" {
		t.Errorf("Quit.Keys() = %v, want default [q ctrl+c]", got)
	}
}

func TestApplyKeyBindingsEmptyOverrideKeepsDefault(t *testing.T) {
	defer ApplyKeyBindings(nil)

	ApplyKeyBindings(map[string][]string{"kill": {}})

	if got := keys.Kill.Keys(); len(got) != 1 || got[0] != "k" {
		t.Errorf("Kill.Keys() = %v, want default [k] when override is empty", got)
	}
}

func TestApplyKeyBindingsNilResetsToDefault(t *testing.T) {
	ApplyKeyBindings(map[string][]string{"kill": {"x"}})
	ApplyKeyBindings(nil)

	if got := keys.Kill.Keys(); len(got) != 1 || got[0] != "k" {
		t.Errorf("Kill.Keys() = %v, want default [k] after nil override", got)
	}
}

func TestApplyPaletteChangesColors(t *testing.T) {
	defer ApplyPalette(Themes["default"])

	ApplyPalette(Themes["dracula"])
	if colorAccent.Dark != Themes["dracula"].AccentDark {
		t.Errorf("colorAccent.Dark = %q, want dracula's %q", colorAccent.Dark, Themes["dracula"].AccentDark)
	}

	ApplyPalette(Themes["default"])
	if colorAccent.Dark != Themes["default"].AccentDark {
		t.Errorf("colorAccent.Dark = %q, want default's %q", colorAccent.Dark, Themes["default"].AccentDark)
	}
}

func TestAllThemesHaveValidColors(t *testing.T) {
	for name, p := range Themes {
		if p.Accent == "" || p.AccentDark == "" {
			t.Errorf("theme %q: Accent/AccentDark must not be empty", name)
		}
		if p.BadgeText == "" {
			t.Errorf("theme %q: BadgeText must not be empty", name)
		}
	}
}
