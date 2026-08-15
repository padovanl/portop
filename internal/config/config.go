// Package config loads portop's optional config.yml: default flag
// values, the color theme, and keybinding overrides. The file is
// entirely opt-in — nothing in this package is required for portop to
// run with its built-in defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config mirrors config.yml. Pointer fields are tri-state (unset / true
// / false) so a missing key in the file means "let the CLI flag's own
// default decide" rather than silently forcing false.
type Config struct {
	RefreshInterval string              `yaml:"refresh_interval,omitempty"`
	ShowEstablished *bool               `yaml:"show_established,omitempty"`
	ResolveDNS      *bool               `yaml:"resolve_dns,omitempty"`
	ResolveSystemd  *bool               `yaml:"resolve_systemd,omitempty"`
	ResolveDocker   *bool               `yaml:"resolve_docker,omitempty"`
	WatchNew        *bool               `yaml:"watch_new,omitempty"`
	Theme           string              `yaml:"theme,omitempty"`
	Keybindings     map[string][]string `yaml:"keybindings,omitempty"`
}

// DefaultPath returns config.yml's standard location under the OS's
// per-user config directory.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "portop", "config.yml"), nil
}

// Load reads and parses config.yml at path. A missing file is not an
// error — it returns a zero Config, since the file is optional.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: parsing %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to path as YAML, creating parent directories as
// needed. Used by the in-app settings screen (`,`) so a live change
// (theme, a rebound key) survives a restart without the user ever
// having to open the file themselves.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// BoolOr returns *p if set, otherwise fallback — used to let config.yml
// supply a flag's default while still letting the flag itself override.
func BoolOr(p *bool, fallback bool) bool {
	if p == nil {
		return fallback
	}
	return *p
}

// WriteDefault writes a fully-commented example config.yml to path,
// creating parent directories as needed. Used by `portop --init-config`.
func WriteDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfigTemplate), 0o644)
}

const defaultConfigTemplate = `# portop config.yml
# Every key here is optional — delete what you don't want to change.
# Command-line flags always win over these defaults when both are given.

# TUI refresh interval (Go duration syntax: "500ms", "2s", "5s", ...).
# refresh_interval: 2s

# Default view options — same meaning as the matching CLI flags.
# show_established: true   # false is the same as always passing --listen
# resolve_dns: true         # reverse DNS on ESTABLISHED remote hosts
# resolve_systemd: true     # associate the owning systemd unit
# resolve_docker: true      # associate the owning Docker container
# watch_new: false          # desktop notification on a new LISTEN port

# Color theme: default | dracula | nord | mono
# theme: default

# Keybinding overrides: action -> list of keys. Anything left out keeps
# its default. Key names follow github.com/charmbracelet/bubbles/key,
# e.g. "up", "down", "ctrl+c", "esc", "/", single characters, etc.
# The first key listed for an action is what's shown in the help/status
# bar, so put your preferred one first.
#
# Valid actions: up, down, top, bottom, enter, kill, open, filter, sort,
# protocol, established, new_mark, copy, refresh, help, quit, escape,
# settings
#
# keybindings:
#   up: ["up", "k"]
#   down: ["down", "j"]
#   kill: ["x"]
#   quit: ["q", "ctrl+c"]
#
# You don't need to hand-edit this section at all, though: press "," in
# the app to open Settings, where themes and keys can be changed live —
# portop keeps this file in sync with whatever you pick there.
`
