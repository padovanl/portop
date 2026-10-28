package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/userfile"
)

// Settings are the window's preferences, saved as JSON next to portop's
// config.yml.
type Settings struct {
	Theme      string   `json:"theme"`       // system, dark or light
	IntervalMS int      `json:"interval_ms"` // scan interval
	Watched    []uint16 `json:"watched"`     // ports to notify about
	Pinned     []uint16 `json:"pinned"`      // ports kept on top
	NotifyNew  bool     `json:"notify_new"`  // notify about new listening ports that pass the filters below
	// Which new ports are worth a notification and an entry in the
	// "Relevant" activity: categories, UDP, and processes the user muted.
	NotifyCategories []string `json:"notify_categories"`
	NotifyUDP        bool     `json:"notify_udp"`
	Muted            []string `json:"muted"`         // process names, lower case
	CloseToTray      bool     `json:"close_to_tray"` // closing the window keeps portop in the tray
	ReduceMotion     bool     `json:"reduce_motion"` // turn off animations
	// NotifyExposed warns when a dev server, database or container starts
	// listening on every interface and the firewall lets it through.
	NotifyExposed bool     `json:"notify_exposed"`
	Editor        string   `json:"editor"`       // command that opens a folder; "" picks an installed one
	AutoStart     bool     `json:"auto_start"`   // start portop at login, in the tray
	Shortcut      string   `json:"shortcut"`     // global shortcut that shows the port finder; "" for none
	Hosts         []Host   `json:"hosts"`        // remote portop --web dashboards
	ComposeDirs   []string `json:"compose_dirs"` // Compose projects audited lately, newest first
}

// Host is a remote machine running portop --web. The password is never
// saved: it is asked when connecting and kept in memory.
type Host struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	User        string `json:"user,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"` // SHA-256 of a trusted self-signed certificate
}

// DefaultShortcut shows the port finder from anywhere.
const DefaultShortcut = "CmdOrCtrl+Alt+P"

// DefaultCategories leave out desktop apps and system services, whose
// short-lived ports (browsers, music players, discovery) are noise.
var DefaultCategories = []string{"dev", "database", "web", "container"}

var knownCategories = map[string]bool{"dev": true, "database": true, "web": true, "container": true, "system": true, "app": true}

func defaultSettings() Settings {
	return Settings{Theme: "system", IntervalMS: 2000, CloseToTray: true,
		NotifyCategories: append([]string{}, DefaultCategories...), Muted: []string{},
		NotifyExposed: true, Shortcut: DefaultShortcut, Hosts: []Host{}, ComposeDirs: []string{}}
}

func (s Settings) interval() time.Duration {
	if s.IntervalMS < 500 {
		return 2 * time.Second
	}
	return time.Duration(s.IntervalMS) * time.Millisecond
}

// clean bounds and sorts what the window sends.
func (s Settings) clean() Settings {
	switch s.Theme {
	case "dark", "light", "system":
	default:
		s.Theme = "system"
	}
	if s.IntervalMS < 500 {
		s.IntervalMS = 500
	}
	if s.IntervalMS > 60000 {
		s.IntervalMS = 60000
	}
	s.Watched = uniquePorts(s.Watched)
	s.Pinned = uniquePorts(s.Pinned)
	cats := []string{}
	for _, c := range s.NotifyCategories {
		if knownCategories[c] && !slices.Contains(cats, c) {
			cats = append(cats, c)
		}
	}
	s.NotifyCategories = cats
	muted := []string{}
	for _, m := range s.Muted {
		m = strings.ToLower(strings.TrimSpace(m))
		if m != "" && len(m) <= 200 && !slices.Contains(muted, m) {
			muted = append(muted, m)
		}
	}
	sort.Strings(muted)
	if len(muted) > 200 {
		muted = muted[:200]
	}
	s.Muted = muted
	s.Editor = strings.TrimSpace(s.Editor)
	if len(s.Editor) > 500 || strings.ContainsAny(s.Editor, "\n\r\x00") {
		s.Editor = ""
	}
	if !validShortcut(s.Shortcut) {
		s.Shortcut = ""
	}
	hosts := []Host{}
	for _, h := range s.Hosts {
		h.Name, h.URL, h.User = strings.TrimSpace(h.Name), strings.TrimRight(strings.TrimSpace(h.URL), "/"), strings.TrimSpace(h.User)
		if h.Name != "" && validHostURL(h.URL) && len(hosts) < 50 && !slices.ContainsFunc(hosts, func(x Host) bool { return x.Name == h.Name }) {
			hosts = append(hosts, h)
		}
	}
	s.Hosts = hosts
	dirs := []string{}
	for _, d := range s.ComposeDirs {
		if d != "" && !slices.Contains(dirs, d) && len(dirs) < 10 {
			dirs = append(dirs, d)
		}
	}
	s.ComposeDirs = dirs
	return s
}

func uniquePorts(ports []uint16) []uint16 {
	seen := map[uint16]bool{}
	out := []uint16{}
	for _, p := range ports {
		if p != 0 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}

type settingsStore struct {
	mu   sync.Mutex
	path string
	cur  Settings
}

// newSettingsStore loads path, or the default location when path is "".
func newSettingsStore(path string) *settingsStore {
	if path == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			path = filepath.Join(dir, "portop", "desktop.json")
		}
	}
	st := &settingsStore{path: path, cur: defaultSettings()}
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			s := defaultSettings()
			if json.Unmarshal(b, &s) == nil {
				st.cur = s.clean()
			}
		}
	}
	return st
}

func (st *settingsStore) get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.cur
	s.Watched = append([]uint16{}, s.Watched...)
	s.Pinned = append([]uint16{}, s.Pinned...)
	s.NotifyCategories = append([]string{}, s.NotifyCategories...)
	s.Muted = append([]string{}, s.Muted...)
	s.Hosts = append([]Host{}, s.Hosts...)
	s.ComposeDirs = append([]string{}, s.ComposeDirs...)
	return s
}

func (st *settingsStore) set(s Settings) (Settings, error) {
	s = s.clean()
	if s.Watched == nil {
		s.Watched = []uint16{}
	}
	if s.Pinned == nil {
		s.Pinned = []uint16{}
	}
	st.mu.Lock()
	st.cur = s
	path := st.path
	st.mu.Unlock()
	if path == "" {
		return s, nil
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return s, err
	}
	if err := userfile.Write(path, append(b, '\n')); err != nil {
		return s, err
	}
	// Watched and pinned ports describe what the user runs; keep them
	// readable by the user only.
	return s, os.Chmod(path, 0o600)
}

// Settings returns the current preferences.
func (b *Backend) Settings() Settings { return b.settings.get() }

// validShortcut accepts accelerators such as "CmdOrCtrl+Alt+P": one or
// more modifiers and a key, as Wails registers them.
func validShortcut(s string) bool {
	if s == "" {
		return true
	}
	parts := strings.Split(s, "+")
	if len(parts) < 2 || len(parts) > 4 {
		return false
	}
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "CmdOrCtrl", "Ctrl", "Cmd", "Alt", "Option", "Shift", "Super":
		default:
			return false
		}
	}
	key := parts[len(parts)-1]
	return len(key) == 1 && strings.Trim(key, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") == "" ||
		len(key) >= 2 && len(key) <= 3 && key[0] == 'F' && strings.Trim(key[1:], "0123456789") == ""
}

// SaveSettings stores new preferences and applies the interval at once.
func (b *Backend) SaveSettings(s Settings) (Settings, error) {
	saved, err := b.settings.set(s)
	if err == nil && b.OnSettings != nil {
		b.OnSettings(saved)
	}
	b.Refresh()
	return saved, err
}
