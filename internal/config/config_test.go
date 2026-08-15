package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsZeroValue(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "" || cfg.ShowEstablished != nil || cfg.Keybindings != nil {
		t.Errorf("expected zero-value Config for a missing file, got %+v", cfg)
	}
}

func TestLoadParsesFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	content := `
refresh_interval: 500ms
show_established: false
resolve_docker: true
theme: dracula
keybindings:
  kill: ["x"]
  quit: ["q", "ctrl+c"]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshInterval != "500ms" {
		t.Errorf("RefreshInterval = %q, want 500ms", cfg.RefreshInterval)
	}
	if cfg.ShowEstablished == nil || *cfg.ShowEstablished != false {
		t.Errorf("ShowEstablished = %v, want pointer to false", cfg.ShowEstablished)
	}
	if cfg.ResolveDocker == nil || *cfg.ResolveDocker != true {
		t.Errorf("ResolveDocker = %v, want pointer to true", cfg.ResolveDocker)
	}
	if cfg.ResolveDNS != nil {
		t.Errorf("ResolveDNS = %v, want nil (unset in file)", cfg.ResolveDNS)
	}
	if cfg.Theme != "dracula" {
		t.Errorf("Theme = %q, want dracula", cfg.Theme)
	}
	if got := cfg.Keybindings["kill"]; len(got) != 1 || got[0] != "x" {
		t.Errorf("Keybindings[kill] = %v, want [x]", got)
	}
	if got := cfg.Keybindings["quit"]; len(got) != 2 || got[0] != "q" || got[1] != "ctrl+c" {
		t.Errorf("Keybindings[quit] = %v, want [q ctrl+c]", got)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("theme: [not valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("expected an error for invalid YAML, got nil")
	}
}

func TestBoolOr(t *testing.T) {
	yes, no := true, false
	if !BoolOr(&yes, false) {
		t.Error("BoolOr(&true, false) = false, want true")
	}
	if BoolOr(&no, true) {
		t.Error("BoolOr(&false, true) = true, want false")
	}
	if !BoolOr(nil, true) {
		t.Error("BoolOr(nil, true) = false, want true (fallback)")
	}
}

func TestWriteDefaultThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yml")
	if err := WriteDefault(path); err != nil {
		t.Fatalf("WriteDefault: %v", err)
	}
	// The template is all commented out by design (nothing forced on
	// the user) — loading it back should yield a harmless zero Config.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load after WriteDefault: %v", err)
	}
	if cfg.Theme != "" {
		t.Errorf("Theme = %q, want empty (template ships fully commented out)", cfg.Theme)
	}
}

func TestDefaultPathIsUnderPortopDir(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if filepath.Base(path) != "config.yml" {
		t.Errorf("DefaultPath = %q, want to end in config.yml", path)
	}
	if filepath.Base(filepath.Dir(path)) != "portop" {
		t.Errorf("DefaultPath = %q, want to live under a portop/ dir", path)
	}
}
