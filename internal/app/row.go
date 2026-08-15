// Package app wires together the scanner, docker, systemd, dns and cpu
// packages into the enriched rows the UI (and --json mode) display, and
// holds the CLI-level configuration for a run.
package app

import (
	"context"
	"net"
	"sort"
	"strconv"
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
	FirstSeen     bool // true if this socket was not present in the previous scan (see Collector.baseline)
}

// Key uniquely identifies a socket across scans, for CPU sampling and new
// port detection: protocol/local address/local port is sufficient since
// the kernel guarantees only one listener per (proto, addr, port) and we
// key established connections the same way, accepting that a very fast
// connect/disconnect cycle on the same 4-tuple within one refresh
// interval could be conflated. That's an acceptable trade-off for a
// monitoring UI refreshing multiple times a second.
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

// Options configures what a Collect call resolves. Resolving systemd/
// Docker/DNS info touches the filesystem and network per row, so callers
// that just want a fast snapshot (e.g. --json without --enrich) can skip
// them.
type Options struct {
	ResolveSystemd bool
	ResolveDocker  bool
	ResolveDNS     bool
}

// Collect scans current sockets, resolves owning processes, and enriches
// each into a Row. Rows are sorted by protocol then local port for stable
// display.
func (c *Collector) Collect(ctx context.Context, opts Options) ([]Row, error) {
	conns, err := scanner.Scan()
	if err != nil {
		return nil, err
	}
	conns = scanner.ResolveProcesses(conns)

	rows := make([]Row, 0, len(conns))
	currentKeys := make(map[Key]bool, len(conns))

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
			if pct, ok := c.cpu.Sample(conn.PID); ok {
				row.CPUPercent = pct
			}
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

	c.seen = currentKeys
	c.init = true

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Protocol != rows[j].Protocol {
			return rows[i].Protocol < rows[j].Protocol
		}
		return rows[i].LocalPort < rows[j].LocalPort
	})

	return rows, nil
}
