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
}

// DefaultCategories leave out desktop apps and system services, whose
// short-lived ports (browsers, music players, discovery) are noise.
var DefaultCategories = []string{"dev", "database", "web", "container"}

var knownCategories = map[string]bool{"dev": true, "database": true, "web": true, "container": true, "system": true, "app": true}

func defaultSettings() Settings {
	return Settings{Theme: "system", IntervalMS: 2000, CloseToTray: true,
		NotifyCategories: append([]string{}, DefaultCategories...), Muted: []string{}}
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

// SaveSettings stores new preferences and applies the interval at once.
func (b *Backend) SaveSettings(s Settings) (Settings, error) {
	saved, err := b.settings.set(s)
	b.Refresh()
	return saved, err
}
