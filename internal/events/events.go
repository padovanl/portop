// Package events turns successive scans into a history of listening
// ports opening and closing.
package events

import (
	"sort"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/app"
)

// Kinds of Event.
const (
	Opened = "opened"
	Closed = "closed"
)

// Event is one listening port appearing or disappearing.
type Event struct {
	ID        uint64    `json:"id"`
	Time      time.Time `json:"time"`
	Kind      string    `json:"kind"`
	Protocol  string    `json:"protocol"`
	Port      uint16    `json:"port"`
	Scope     string    `json:"scope"`
	PID       int       `json:"pid,omitempty"`
	Process   string    `json:"process,omitempty"`
	Project   string    `json:"project,omitempty"`
	Framework string    `json:"framework,omitempty"`
	Category  string    `json:"category,omitempty"`
}

// key identifies a listener. The IPv4 and IPv6 sockets of one server, and
// the sockets it binds on several addresses, are one listener.
type key struct {
	protocol string
	port     uint16
	pid      int
}

// Log keeps the most recent events.
type Log struct {
	mu      sync.Mutex
	limit   int
	events  []Event
	last    map[key]Event
	started bool
	nextID  uint64
}

// NewLog keeps up to limit events.
func NewLog(limit int) *Log {
	return &Log{limit: limit, last: map[key]Event{}}
}

// Update compares a scan with the previous one and records what changed.
// The first scan only sets the baseline: what was already listening when
// portop started did not just open. It returns the new events.
func (l *Log) Update(rows []app.Row, at time.Time) []Event {
	now := map[key]Event{}
	for _, r := range rows {
		if !r.Listening() {
			continue
		}
		k := key{string(r.Protocol), r.LocalPort, r.PID}
		e, seen := now[k]
		// A wider bind wins: 0.0.0.0 and 127.0.0.1 together are "all".
		if !seen || scopeRank(r.Scope()) > scopeRank(e.Scope) {
			now[k] = Event{
				Protocol: string(r.Protocol), Port: r.LocalPort, Scope: r.Scope(), PID: r.PID,
				Process: r.ProcessName, Project: r.Project.Name, Framework: r.Project.Framework, Category: r.Category,
			}
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	var fresh []Event
	if l.started {
		for k, e := range now {
			if _, ok := l.last[k]; !ok {
				e.Kind = Opened
				fresh = append(fresh, e)
			}
		}
		for k, e := range l.last {
			if _, ok := now[k]; !ok {
				e.Kind = Closed
				fresh = append(fresh, e)
			}
		}
	}
	sort.Slice(fresh, func(i, j int) bool {
		if fresh[i].Kind != fresh[j].Kind {
			return fresh[i].Kind == Closed // a restart reads closed, then opened
		}
		return fresh[i].Port < fresh[j].Port
	})
	for i := range fresh {
		l.nextID++
		fresh[i].ID = l.nextID
		fresh[i].Time = at
	}
	l.events = append(l.events, fresh...)
	if over := len(l.events) - l.limit; over > 0 {
		l.events = append([]Event(nil), l.events[over:]...)
	}
	l.last = now
	l.started = true
	return fresh
}

// Events returns the recorded events, oldest first.
func (l *Log) Events() []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Event(nil), l.events...)
}

func scopeRank(scope string) int {
	switch scope {
	case app.ScopeAll:
		return 2
	case app.ScopeAddress:
		return 1
	}
	return 0
}
