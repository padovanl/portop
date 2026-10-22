package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/userfile"
)

// Settings are the window's preferences, saved as JSON next to portop's
// config.yml.
type Settings struct {
	Theme        string   `json:"theme"`         // system, dark or light
	IntervalMS   int      `json:"interval_ms"`   // scan interval
	Watched      []uint16 `json:"watched"`       // ports to notify about
	Pinned       []uint16 `json:"pinned"`        // ports kept on top
	NotifyNew    bool     `json:"notify_new"`    // notify about every new listening port
	CloseToTray  bool     `json:"close_to_tray"` // closing the window keeps portop in the tray
	ReduceMotion bool     `json:"reduce_motion"` // turn off animations
}

func defaultSettings() Settings {
	return Settings{Theme: "system", IntervalMS: 2000, CloseToTray: true}
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
	return s, userfile.Write(path, append(b, '\n'))
}

// Settings returns the current preferences.
func (b *Backend) Settings() Settings { return b.settings.get() }

// SaveSettings stores new preferences and applies the interval at once.
func (b *Backend) SaveSettings(s Settings) (Settings, error) {
	saved, err := b.settings.set(s)
	b.Refresh()
	return saved, err
}
