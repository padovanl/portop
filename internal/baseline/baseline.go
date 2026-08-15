// Package baseline implements portop's drift-detection feature: save a
// snapshot of which ports are currently listening, then later diff the
// live system against it to see what changed. This is what powers
// `portop --save-baseline` / `portop --diff`, e.g. from a cron job or
// systemd timer, to notice a port that started listening unexpectedly.
package baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/padovan93/portop/internal/app"
	"github.com/padovan93/portop/internal/scanner"
)

// Entry is one listening socket as recorded in a baseline snapshot.
type Entry struct {
	Protocol string `json:"protocol"`
	Port     uint16 `json:"port"`
	Process  string `json:"process,omitempty"`
}

func (e Entry) key() string { return e.Protocol + ":" + fmt.Sprint(e.Port) }

type file struct {
	SavedAt time.Time `json:"saved_at"`
	Entries []Entry   `json:"entries"`
}

// FromRows extracts the baseline-relevant fields (LISTEN sockets only —
// baselines are about which services are supposed to be running, not
// ephemeral connections) from a Collector snapshot.
func FromRows(rows []app.Row) []Entry {
	var entries []Entry
	for _, r := range rows {
		if r.State != scanner.StateListen {
			continue
		}
		entries = append(entries, Entry{
			Protocol: string(r.Protocol),
			Port:     r.LocalPort,
			Process:  r.ProcessName,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Port != entries[j].Port {
			return entries[i].Port < entries[j].Port
		}
		return entries[i].Protocol < entries[j].Protocol
	})
	return entries
}

// DefaultPath returns the standard location for the baseline file,
// following the OS's conventional per-user config directory.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "portop", "baseline.json"), nil
}

// Save writes entries to path, creating parent directories as needed.
func Save(path string, entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file{SavedAt: time.Now(), Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Load reads back a previously saved baseline.
func Load(path string) ([]Entry, time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, time.Time{}, fmt.Errorf("baseline: parsing %s: %w", path, err)
	}
	return f.Entries, f.SavedAt, nil
}

// Diff compares a baseline against the current live entries. added are
// ports listening now that weren't in the baseline; removed are ports
// in the baseline that aren't listening now.
func Diff(baseline, current []Entry) (added, removed []Entry) {
	inBaseline := make(map[string]bool, len(baseline))
	for _, e := range baseline {
		inBaseline[e.key()] = true
	}
	inCurrent := make(map[string]bool, len(current))
	for _, e := range current {
		inCurrent[e.key()] = true
	}
	for _, e := range current {
		if !inBaseline[e.key()] {
			added = append(added, e)
		}
	}
	for _, e := range baseline {
		if !inCurrent[e.key()] {
			removed = append(removed, e)
		}
	}
	return added, removed
}
