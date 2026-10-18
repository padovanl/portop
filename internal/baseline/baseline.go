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

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/userfile"
)

// Entry is one listening socket as recorded in a baseline snapshot.
type Entry struct {
	Protocol string `json:"protocol"`
	Port     uint16 `json:"port"`
	Process  string `json:"process,omitempty"`
	// Scope is who can reach the port: ScopeLoopback, ScopeAll or
	// ScopeAddress. Baselines saved before it existed leave it empty.
	Scope string `json:"scope,omitempty"`
}

// Scopes of a listening socket. A service moving from loopback to every
// interface is the drift that matters most, so it is part of the identity
// of an entry. A specific address is kept as one scope so a DHCP or
// container address change is not reported.
const (
	ScopeLoopback = app.ScopeLoopback
	ScopeAll      = app.ScopeAll
	ScopeAddress  = app.ScopeAddress
)

func (e Entry) key(withScope bool) string {
	k := e.Protocol + ":" + fmt.Sprint(e.Port)
	if withScope {
		k += ":" + e.Scope
	}
	return k
}

type file struct {
	SavedAt time.Time `json:"saved_at"`
	Entries []Entry   `json:"entries"`
}

// FromRows extracts the baseline-relevant fields (listening TCP and UDP
// sockets only — baselines are about which services are supposed to be
// running, not ephemeral connections) from a Collector snapshot. A service
// listening on IPv4 and IPv6 alike yields one entry.
func FromRows(rows []app.Row) []Entry {
	var entries []Entry
	seen := map[string]bool{}
	for _, r := range rows {
		if !r.Listening() {
			continue
		}
		e := Entry{
			Protocol: string(r.Protocol),
			Port:     r.LocalPort,
			Process:  r.ProcessName,
			Scope:    r.Scope(),
		}
		if seen[e.key(true)] {
			continue
		}
		seen[e.key(true)] = true
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Port != entries[j].Port {
			return entries[i].Port < entries[j].Port
		}
		if entries[i].Protocol != entries[j].Protocol {
			return entries[i].Protocol < entries[j].Protocol
		}
		return entries[i].Scope < entries[j].Scope
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
	data, err := json.MarshalIndent(file{SavedAt: time.Now(), Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	return userfile.Write(path, data)
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
// in the baseline that aren't listening now. A port whose scope changed
// (say from loopback to every interface) is both removed and added. A
// baseline saved before scopes existed is compared by protocol and port
// only, so upgrading portop does not report every port as changed.
func Diff(baseline, current []Entry) (added, removed []Entry) {
	withScope := true
	for _, e := range baseline {
		if e.Scope == "" {
			withScope = false
			break
		}
	}
	inBaseline := make(map[string]bool, len(baseline))
	for _, e := range baseline {
		inBaseline[e.key(withScope)] = true
	}
	inCurrent := make(map[string]bool, len(current))
	for _, e := range current {
		k := e.key(withScope)
		if !inBaseline[k] && !inCurrent[k] {
			added = append(added, e)
		}
		inCurrent[k] = true
	}
	reported := map[string]bool{}
	for _, e := range baseline {
		k := e.key(withScope)
		if !inCurrent[k] && !reported[k] {
			removed = append(removed, e)
			reported[k] = true
		}
	}
	return added, removed
}
