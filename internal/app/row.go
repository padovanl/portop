// Package app collects sockets and enriches them with process metadata.
package app

import (
	"context"
	"net"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/dnscache"
	"github.com/padovanl/portop/internal/docker"
	"github.com/padovanl/portop/internal/scanner"
	"github.com/padovanl/portop/internal/systemdinfo"
)

// Row is a single, fully enriched table row.
type Row struct {
	Protocol      scanner.Protocol
	LocalAddr     net.IP
	LocalPort     uint16
	RemoteAddr    net.IP
	RemotePort    uint16
	RemoteHost    string // reverse DNS, empty until resolved
	State         scanner.State
	IPv6          bool
	PID           int
	ProcessName   string
	CPUPercent    float64
	SystemdUnit   string
	ContainerName string
	FirstSeen     bool // true if this socket was absent from the previous scan
}

// Listening reports whether the socket accepts new traffic: a TCP socket
// in LISTEN, or a bound UDP socket that has not connected to a peer (UDP
// has no LISTEN state; ss -l lists these as UNCONN).
func (r Row) Listening() bool {
	if r.LocalPort == 0 {
		return false // a socket never bound to a port
	}
	return r.State == scanner.StateListen || (r.Protocol == scanner.UDP && r.State == scanner.StateUnconn)
}

// Matches reports whether a row contains the search text used by the TUI,
// JSON output, and web dashboard.
func (r Row) Matches(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	return strings.Contains(strconv.Itoa(int(r.LocalPort)), query) ||
		strings.Contains(strconv.Itoa(r.PID), query) ||
		strings.Contains(strings.ToLower(r.ProcessName), query) ||
		strings.Contains(strings.ToLower(r.SystemdUnit), query) ||
		strings.Contains(strings.ToLower(r.ContainerName), query) ||
		strings.Contains(strings.ToLower(r.LocalAddr.String()), query) ||
		(r.RemotePort != 0 && strings.Contains(strings.ToLower(r.RemoteAddr.String()), query))
}

// Key identifies a socket by protocol and endpoints across scans.
// Sockets sharing the same endpoints can map to the same key.
type Key struct {
	Protocol scanner.Protocol
	Local    string
	Remote   string
}

func keyFor(c scanner.Connection) Key {
	return Key{
		Protocol: c.Protocol,
		Local:    net.JoinHostPort(c.LocalAddr.String(), strconv.Itoa(int(c.LocalPort))),
		Remote:   net.JoinHostPort(c.RemoteAddr.String(), strconv.Itoa(int(c.RemotePort))),
	}
}

// Collector produces enriched Rows and tracks CPU usage and first-seen
// state across successive calls to Collect.
type Collector struct {
	// mu serializes Collect: the TUI starts a scan on every tick without
	// waiting for the previous one, and seen/init are not safe to share.
	mu sync.Mutex

	cpu    *scanner.CPUTracker
	dns    *dnscache.Resolver
	docker *docker.Client

	seen map[Key]bool // sockets observed in any previous Collect call
	init bool         // false until the first Collect call completes
}

// NewCollector wires up a Collector with real system backends.
func NewCollector() *Collector {
	return &Collector{
		cpu:    scanner.NewCPUTracker(),
		dns:    dnscache.New(defaultDNSTimeout),
		docker: docker.NewClient(),
		seen:   make(map[Key]bool),
	}
}

const defaultDNSTimeout = 800 * time.Millisecond

// Options controls the metadata resolved during a scan.
type Options struct {
	ResolveSystemd bool
	ResolveDocker  bool
	ResolveDNS     bool
}

// Collect scans current sockets, resolves owning processes, and enriches
// each into a Row. Rows are sorted by protocol then local port for stable
// display.
func (c *Collector) Collect(ctx context.Context, opts Options) ([]Row, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conns, err := scanner.Scan()
	if err != nil {
		return nil, err
	}
	conns = scanner.ResolveProcesses(conns)

	rows := make([]Row, 0, len(conns))
	currentKeys := make(map[Key]bool, len(conns))
	cpuByPID := make(map[int]float64)

	for _, conn := range conns {
		k := keyFor(conn)
		currentKeys[k] = true

		row := Row{
			Protocol:    conn.Protocol,
			LocalAddr:   conn.LocalAddr,
			LocalPort:   conn.LocalPort,
			RemoteAddr:  conn.RemoteAddr,
			RemotePort:  conn.RemotePort,
			State:       conn.State,
			IPv6:        conn.IPv6,
			PID:         conn.PID,
			ProcessName: conn.ProcessName,
			FirstSeen:   c.init && !c.seen[k],
		}

		if conn.PID != 0 {
			pct, sampled := cpuByPID[conn.PID]
			if !sampled {
				pct, _ = c.cpu.Sample(conn.PID)
				cpuByPID[conn.PID] = pct
			}
			row.CPUPercent = pct
			if opts.ResolveSystemd {
				row.SystemdUnit = systemdinfo.UnitForPID(conn.PID)
			}
			if opts.ResolveDocker && c.docker.Available() {
				if cid := docker.ContainerIDForPID(conn.PID); cid != "" {
					row.ContainerName = c.docker.ContainerName(ctx, cid)
				}
			}
		}

		if opts.ResolveDNS && conn.State == scanner.StateEstablished && !conn.RemoteAddr.IsUnspecified() {
			if name, ok := c.dns.Lookup(conn.RemoteAddr.String()); ok {
				row.RemoteHost = name
			}
		}

		rows = append(rows, row)
	}

	if opts.ResolveDocker && portBasedContainers && c.docker.Available() {
		labelPublishedPorts(rows, c.docker.PublishedPorts(ctx))
	}

	c.seen = currentKeys
	c.init = true
	c.cpu.Retain(cpuByPID)

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Protocol != rows[j].Protocol {
			return rows[i].Protocol < rows[j].Protocol
		}
		return rows[i].LocalPort < rows[j].LocalPort
	})

	return rows, nil
}

// portBasedContainers is true where container processes are not visible
// to the host (Docker Desktop on Windows and macOS runs them inside a VM,
// and macOS has no /proc cgroups to read): there, containers are
// recognised by the host ports they publish instead of by the owning
// PID's cgroup. Linux keeps the PID-based lookup.
var portBasedContainers = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

// labelPublishedPorts fills ContainerName on listening rows whose port is
// published by a running container and that have no name yet.
func labelPublishedPorts(rows []Row, published []docker.PublishedPort) {
	type portKey struct {
		proto scanner.Protocol
		port  uint16
	}
	names := make(map[portKey]string, len(published))
	for _, p := range published {
		if p.State != "" && p.State != "running" {
			continue
		}
		k := portKey{scanner.Protocol(strings.ToUpper(p.Type)), p.PublicPort}
		if _, dup := names[k]; !dup {
			names[k] = p.ContainerName
		}
	}
	for i := range rows {
		r := &rows[i]
		if r.ContainerName != "" || (r.State != scanner.StateListen && r.State != scanner.StateUnconn) {
			continue
		}
		if name, ok := names[portKey{r.Protocol, r.LocalPort}]; ok {
			r.ContainerName = name
		}
	}
}
