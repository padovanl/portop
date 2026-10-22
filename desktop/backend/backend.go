// Package backend is portop desktop's engine: it scans on a timer, keeps
// the history the charts draw, records events, and carries out the
// actions the window asks for. It has no GUI dependency, so the same code
// serves the Wails window and the browser development server.
package backend

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/events"
	"github.com/padovanl/portop/internal/openurl"
	"github.com/padovanl/portop/internal/scanner"
)

// historyLen is how many scans the traffic charts remember.
const historyLen = 90

// Backend is bound to the frontend: its exported methods are what the
// window can call.
type Backend struct {
	collector *app.Collector
	log       *events.Log
	settings  *settingsStore

	mu       sync.RWMutex
	snapshot Snapshot
	total    []TrafficPoint         // whole-machine rates, oldest first
	byPID    map[int][]TrafficPoint // per-process rates, oldest first
	wake     chan struct{}

	// Version is shown in Settings; set by the shell.
	Version string
	// HidePID leaves one process out of every scan (the development
	// server's own socket); 0 hides nothing.
	HidePID int

	// OnSnapshot receives every new snapshot; OnNotify shows a desktop
	// notification. Both are set by the shell before Start.
	OnSnapshot func(Snapshot)
	OnNotify   func(title, body string)
}

// Snapshot is everything the window draws.
type Snapshot struct {
	Time    time.Time              `json:"time"`
	Rows    []Row                  `json:"rows"`
	Summary Summary                `json:"summary"`
	Traffic []TrafficPoint         `json:"traffic"`
	Process map[int][]TrafficPoint `json:"process_traffic"`
	Events  []events.Event         `json:"new_events"`
	Error   string                 `json:"error,omitempty"`
	Hidden  int                    `json:"hidden"` // listening sockets whose owner portop cannot see
}

// Summary holds the headline numbers.
type Summary struct {
	Listening   int     `json:"listening"`
	Established int     `json:"established"`
	Processes   int     `json:"processes"`
	DevServers  int     `json:"dev_servers"`
	Exposed     int     `json:"exposed"`
	RXRate      float64 `json:"rx_rate"`
	TXRate      float64 `json:"tx_rate"`
}

// TrafficPoint is one scan's rates in bytes per second.
type TrafficPoint struct {
	Time int64   `json:"t"` // Unix milliseconds
	RX   float64 `json:"rx"`
	TX   float64 `json:"tx"`
}

// Row is one socket as the window shows it.
type Row struct {
	Key           string   `json:"key"`
	Protocol      string   `json:"protocol"`
	LocalAddress  string   `json:"local_address"`
	LocalPort     uint16   `json:"local_port"`
	RemoteAddress string   `json:"remote_address,omitempty"`
	RemotePort    uint16   `json:"remote_port,omitempty"`
	RemoteHost    string   `json:"remote_host,omitempty"`
	State         string   `json:"state"`
	Listening     bool     `json:"listening"`
	Scope         string   `json:"scope"`
	IPv6          bool     `json:"ipv6"`
	PID           int      `json:"pid,omitempty"`
	Process       string   `json:"process,omitempty"`
	User          string   `json:"user,omitempty"`
	Workers       []int    `json:"workers,omitempty"`
	Firewall      string   `json:"firewall,omitempty"`
	RXBytes       *uint64  `json:"rx_bytes,omitempty"`
	TXBytes       *uint64  `json:"tx_bytes,omitempty"`
	RXRate        *float64 `json:"rx_rate,omitempty"`
	TXRate        *float64 `json:"tx_rate,omitempty"`
	CPU           float64  `json:"cpu"`
	Unit          string   `json:"unit,omitempty"`
	Container     string   `json:"container,omitempty"`
	Project       string   `json:"project,omitempty"`
	ProjectRoot   string   `json:"project_root,omitempty"`
	Language      string   `json:"language,omitempty"`
	Framework     string   `json:"framework,omitempty"`
	Category      string   `json:"category"`
	URL           string   `json:"url,omitempty"`
	New           bool     `json:"new,omitempty"`
}

// New returns a Backend reading settings from path ("" for the default).
func New(settingsPath string) *Backend {
	return &Backend{
		collector: app.NewCollector(),
		log:       events.NewLog(500),
		settings:  newSettingsStore(settingsPath),
		byPID:     map[int][]TrafficPoint{},
		wake:      make(chan struct{}, 1),
	}
}

// Start scans until ctx ends, at the interval in the settings.
func (b *Backend) Start(ctx context.Context) {
	go func() {
		for {
			b.scan(ctx)
			interval := b.settings.get().interval()
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			case <-b.wake:
			}
		}
	}()
}

// Refresh scans now instead of at the next tick.
func (b *Backend) Refresh() {
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// About describes the app and the machine it watches.
type About struct {
	Version string `json:"version"`
	Host    string `json:"host"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

// About returns the version, host name and platform.
func (b *Backend) About() About {
	host, _ := os.Hostname()
	return About{Version: b.Version, Host: host, OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// Snapshot returns the latest scan.
func (b *Backend) Snapshot() Snapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.snapshot
}

// Events returns the recorded history, oldest first.
func (b *Backend) Events() []events.Event {
	return append([]events.Event{}, b.log.Events()...)
}

func (b *Backend) scan(ctx context.Context) {
	opts := app.Options{
		ResolveSystemd: true, ResolveDocker: true, ResolveDNS: true,
		ResolveFirewall: true, ResolveTraffic: true, ResolveProjects: true,
	}
	scanCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	rows, err := b.collector.Collect(scanCtx, opts)
	cancel()
	if b.HidePID != 0 {
		rows = hidePID(rows, b.HidePID)
	}
	now := time.Now()
	snap := Snapshot{Time: now}
	if err != nil {
		snap.Error = err.Error()
	}
	fresh := b.log.Update(rows, now)
	snap.Events = append([]events.Event{}, fresh...)
	snap.Rows, snap.Summary, snap.Hidden = convert(rows)

	b.mu.Lock()
	point := TrafficPoint{Time: now.UnixMilli(), RX: snap.Summary.RXRate, TX: snap.Summary.TXRate}
	b.total = appendPoint(b.total, point)
	perPID := map[int]TrafficPoint{}
	for _, r := range rows {
		if r.PID == 0 || !r.RateKnown {
			continue
		}
		p := perPID[r.PID]
		p.Time, p.RX, p.TX = point.Time, p.RX+r.RXRate, p.TX+r.TXRate
		perPID[r.PID] = p
	}
	live := map[int]bool{}
	for _, r := range rows {
		if r.PID != 0 {
			live[r.PID] = true
		}
	}
	for pid := range b.byPID {
		if !live[pid] {
			delete(b.byPID, pid)
		}
	}
	for pid := range live {
		p, ok := perPID[pid]
		if !ok {
			p = TrafficPoint{Time: point.Time}
		}
		b.byPID[pid] = appendPoint(b.byPID[pid], p)
	}
	snap.Traffic = append([]TrafficPoint(nil), b.total...)
	snap.Process = make(map[int][]TrafficPoint, len(b.byPID))
	for pid, h := range b.byPID {
		snap.Process[pid] = append([]TrafficPoint(nil), h...)
	}
	b.snapshot = snap
	b.mu.Unlock()

	b.alert(fresh)
	if b.OnSnapshot != nil {
		b.OnSnapshot(snap)
	}
}

// hidePID drops a process's sockets and the local connections to its
// listening ports.
func hidePID(rows []app.Row, pid int) []app.Row {
	ports := map[uint16]bool{}
	for _, r := range rows {
		if r.PID == pid && r.Listening() {
			ports[r.LocalPort] = true
		}
	}
	kept := rows[:0]
	for _, r := range rows {
		if r.PID == pid || (r.RemotePort != 0 && ports[r.RemotePort] && r.RemoteAddr.IsLoopback()) {
			continue
		}
		kept = append(kept, r)
	}
	return kept
}

func appendPoint(h []TrafficPoint, p TrafficPoint) []TrafficPoint {
	h = append(h, p)
	if len(h) > historyLen {
		h = h[len(h)-historyLen:]
	}
	return h
}

// alert notifies about watched ports, and about every new port when the
// setting asks for it.
func (b *Backend) alert(fresh []events.Event) {
	if b.OnNotify == nil || len(fresh) == 0 {
		return
	}
	s := b.settings.get()
	watched := map[uint16]bool{}
	for _, p := range s.Watched {
		watched[p] = true
	}
	for _, e := range fresh {
		if !watched[e.Port] && !(s.NotifyNew && e.Kind == events.Opened) {
			continue
		}
		who := e.Process
		if e.Project != "" {
			who = e.Project
		}
		if who == "" {
			who = "an unknown process"
		}
		title := fmt.Sprintf("Port %d is now open", e.Port)
		body := fmt.Sprintf("%s is listening on %s %d.", who, e.Protocol, e.Port)
		if e.Kind == events.Closed {
			title = fmt.Sprintf("Port %d closed", e.Port)
			body = fmt.Sprintf("%s stopped listening on %s %d.", who, e.Protocol, e.Port)
		}
		b.OnNotify(title, body)
	}
}

// convert turns collector rows into window rows and the summary.
func convert(rows []app.Row) ([]Row, Summary, int) {
	out := make([]Row, 0, len(rows))
	var s Summary
	pids, dev := map[int]bool{}, map[int]bool{}
	hidden := 0
	for _, r := range rows {
		row := Row{
			Key:          rowKey(r),
			Protocol:     string(r.Protocol),
			LocalAddress: r.LocalAddr.String(),
			LocalPort:    r.LocalPort,
			State:        string(r.State),
			Listening:    r.Listening(),
			Scope:        r.Scope(),
			IPv6:         r.IPv6,
			PID:          r.PID,
			Process:      r.ProcessName,
			User:         r.Username,
			Workers:      r.Workers,
			Firewall:     r.Firewall,
			CPU:          r.CPUPercent,
			Unit:         r.SystemdUnit,
			Container:    r.ContainerName,
			Project:      r.Project.Name,
			ProjectRoot:  r.Project.Root,
			Language:     r.Project.Language,
			Framework:    r.Project.Framework,
			Category:     r.Category,
			New:          r.FirstSeen,
		}
		if r.RemotePort != 0 {
			row.RemoteAddress, row.RemotePort, row.RemoteHost = r.RemoteAddr.String(), r.RemotePort, r.RemoteHost
		}
		if r.TrafficKnown {
			rx, tx := r.RXBytes, r.TXBytes
			row.RXBytes, row.TXBytes = &rx, &tx
		}
		if r.RateKnown {
			rx, tx := r.RXRate, r.TXRate
			row.RXRate, row.TXRate = &rx, &tx
			s.RXRate += rx
			s.TXRate += tx
		}
		if row.Listening {
			s.Listening++
			if r.Protocol == scanner.TCP && r.LocalPort != 0 {
				row.URL = openurl.URLForListener(r.LocalAddr, r.LocalPort)
			}
			if r.Scope() == app.ScopeAll || r.Scope() == app.ScopeAddress {
				s.Exposed++
			}
			if r.PID == 0 {
				hidden++
			}
			if r.Category == "dev" && r.PID != 0 {
				dev[r.PID] = true
			}
		} else if r.State == scanner.StateEstablished {
			s.Established++
		}
		if r.PID != 0 {
			pids[r.PID] = true
		}
		out = append(out, row)
	}
	s.Processes = len(pids)
	s.DevServers = len(dev)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Listening != out[j].Listening {
			return out[i].Listening
		}
		return false
	})
	return out, s, hidden
}

func rowKey(r app.Row) string {
	return string(r.Protocol) + "|" + r.LocalAddr.String() + "|" + strconv.Itoa(int(r.LocalPort)) +
		"|" + r.RemoteAddr.String() + "|" + strconv.Itoa(int(r.RemotePort))
}

// OpenURL opens a listening port's address in the default browser. Only
// http(s) URLs on this machine are accepted.
func (b *Backend) OpenURL(url string) error {
	if !isLocalURL(url) {
		return fmt.Errorf("not a local web address: %q", url)
	}
	return openurl.Open(url)
}
