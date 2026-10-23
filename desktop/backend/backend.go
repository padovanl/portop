// Package backend is portop desktop's engine: it scans on a timer, keeps
// the history the charts draw, records events, and carries out the
// actions the window asks for. It has no GUI dependency, so the same code
// serves the Wails window and the browser development server.
package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/baseline"
	"github.com/padovanl/portop/internal/docker"
	"github.com/padovanl/portop/internal/events"
	"github.com/padovanl/portop/internal/openurl"
	"github.com/padovanl/portop/internal/procinfo"
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
	total    []TrafficPoint              // whole-machine rates, oldest first
	byPID    map[int][]TrafficPoint      // per-process rates, oldest first
	pending  map[pendingKey]events.Event // new ports waiting to settle before a notification
	wake     chan struct{}
	long     *longHistory
	lastRows []app.Row
	recent   *recentStore
	remote   remoteState
	elev     elevation
	docker   *docker.Client
	saveAt   time.Time

	baseMod   time.Time        // baseline file's modification time when last read
	baseSaved []baseline.Entry // its entries

	// Version is shown in Settings; set by the shell.
	Version string
	// HidePID leaves one process out of every scan (the development
	// server's own socket); 0 hides nothing.
	HidePID int
	// BaselinePath overrides where the baseline is read and saved (tests);
	// "" uses the same file as portop --save-baseline.
	BaselinePath string

	// OnSettings applies saved preferences the shell owns (autostart, the
	// global shortcut). SaveDialog and FolderDialog ask the user for a
	// path; without them (the browser development server) exports are
	// returned as text and folders cannot be picked.
	OnSettings   func(Settings)
	SaveDialog   func(title, name string) (string, error)
	FolderDialog func(title string) (string, error)

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
	// Elevated is true while the administrator helper scans (see Elevate).
	Elevated bool `json:"elevated"`
	// BaselineChanges counts listening ports added or removed since the
	// baseline was saved; -1 when there is no baseline.
	BaselineChanges int `json:"baseline_changes"`
}

// Summary holds the headline numbers.
type Summary struct {
	Listening   int     `json:"listening"`
	Established int     `json:"established"`
	Processes   int     `json:"processes"`
	DevServers  int     `json:"dev_servers"`
	Exposed     int     `json:"exposed"`
	AtRisk      int     `json:"at_risk"` // exposed dev servers, databases and containers the firewall lets through
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
	AtRisk        bool     `json:"at_risk,omitempty"`
}

// New returns a Backend reading settings from path ("" for the default).
func New(settingsPath string) *Backend {
	return &Backend{
		collector: app.NewCollector(),
		log:       events.NewLog(500),
		settings:  newSettingsStore(settingsPath),
		byPID:     map[int][]TrafficPoint{},
		pending:   map[pendingKey]events.Event{},
		wake:      make(chan struct{}, 1),
		long:      newLongHistory(),
		recent:    newRecentStore(recentPath(settingsPath)),
		remote:    remoteState{passwords: map[string]string{}},
		docker:    docker.NewClient(),
	}
}

func recentPath(settingsPath string) string {
	if settingsPath == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			settingsPath = filepath.Join(dir, "portop", "desktop.json")
		}
	}
	return defaultRecentPath(settingsPath)
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

// collect scans through the administrator helper while there is one, and
// as this user otherwise or when the helper stops answering.
func (b *Backend) collect(ctx context.Context, opts app.Options) ([]app.Row, error) {
	if c := b.helper(); c != nil {
		rows, err := c.rows(ctx)
		if err == nil {
			return rows, nil
		}
		b.dropHelper(c)
	}
	return b.collector.Collect(ctx, opts)
}

func (b *Backend) scan(ctx context.Context) {
	opts := app.Options{
		ResolveSystemd: true, ResolveDocker: true, ResolveDNS: true,
		ResolveFirewall: true, ResolveTraffic: true, ResolveProjects: true,
	}
	scanCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	rows, err := b.collect(scanCtx, opts)
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
	snap.Elevated = b.helper() != nil

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
	b.long.add(now, point, perPID, live)
	b.lastRows = rows
	snap.BaselineChanges = b.baselineChanges(rows)
	b.snapshot = snap
	b.mu.Unlock()

	b.recent.observe(rows, now, func(pid int) (procinfo.Info, []string, error) {
		info, err := procinfo.Load(pid)
		return info, processArgv(pid), err
	})
	if now.Sub(b.saveAt) > 10*time.Second {
		b.saveAt = now
		_ = b.recent.save()
	}

	b.alert(fresh, rows, now)
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

// Relevant reports whether an event passes the notification filters:
// a category the user wants, TCP unless UDP is on, and a process that is
// not muted. Watched ports are always relevant.
func (s Settings) Relevant(e events.Event) bool {
	if slices.Contains(s.Watched, e.Port) {
		return true
	}
	if e.Protocol == "UDP" && !s.NotifyUDP {
		return false
	}
	if !slices.Contains(s.NotifyCategories, e.Category) {
		return false
	}
	return !slices.Contains(s.Muted, strings.ToLower(e.Process))
}

// wantsNotice is true for a new port worth a notification: a relevant
// one when new ports notify, or one exposed to the network.
func (s Settings) wantsNotice(e events.Event) bool {
	if s.NotifyNew && s.Relevant(e) {
		return true
	}
	return s.NotifyExposed && e.Protocol != "UDP" && atRisk(e.Scope, e.Category, e.Firewall) && !s.isMuted(e)
}

func (s Settings) isMuted(e events.Event) bool {
	return slices.Contains(s.Muted, strings.ToLower(e.Process))
}

// settleTime is how long a new port must stay open before it is worth a
// notification: browsers and players open and close ports in seconds.
const settleTime = 5 * time.Second

type pendingKey struct {
	protocol string
	port     uint16
	pid      int
}

// alert notifies about watched ports opening and closing, and, when the
// setting is on, about relevant new ports that stayed open settleTime.
func (b *Backend) alert(fresh []events.Event, rows []app.Row, now time.Time) {
	s := b.settings.get()
	open := map[pendingKey]bool{}
	for _, r := range rows {
		if r.Listening() {
			open[pendingKey{string(r.Protocol), r.LocalPort, r.PID}] = true
		}
	}
	var due []events.Event
	b.mu.Lock()
	for k, e := range b.pending {
		switch {
		case !open[k]:
			delete(b.pending, k) // closed before it settled: not worth a word
		case now.Sub(e.Time) >= settleTime:
			// The settings may have changed while it settled.
			if s.wantsNotice(e) {
				due = append(due, e)
			}
			delete(b.pending, k)
		}
	}
	for _, e := range fresh {
		if e.Kind == events.Opened && !slices.Contains(s.Watched, e.Port) && s.wantsNotice(e) {
			b.pending[pendingKey{e.Protocol, e.Port, e.PID}] = e
		}
	}
	b.mu.Unlock()
	if b.OnNotify == nil {
		return
	}
	for _, e := range fresh {
		if slices.Contains(s.Watched, e.Port) {
			due = append(due, e)
		}
	}
	for _, e := range due {
		who := e.Process
		if e.Project != "" {
			who = e.Project
		}
		if who == "" {
			who = "an unknown process"
		}
		title := fmt.Sprintf("Port %d is now open", e.Port)
		body := fmt.Sprintf("%s is listening on %s %d.", who, e.Protocol, e.Port)
		if e.Kind == events.Opened && s.NotifyExposed && atRisk(e.Scope, e.Category, e.Firewall) && !s.isMuted(e) {
			title = fmt.Sprintf("Port %d is reachable from the network", e.Port)
			body = fmt.Sprintf("%s listens on every interface and the firewall lets connections in.", who)
		}
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
			if atRisk(r.Scope(), r.Category, r.Firewall) {
				row.AtRisk = true
				s.AtRisk++
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
