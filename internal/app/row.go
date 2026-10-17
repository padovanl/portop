// Package app collects sockets and enriches them with process metadata.
package app

import (
	"context"
	"net"
	"os/user"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/dnscache"
	"github.com/padovanl/portop/internal/docker"
	"github.com/padovanl/portop/internal/firewall"
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
	Username      string // owner of the socket; empty if unknown
	Workers       []int  // other processes holding the same socket (a server's workers)
	Firewall      string // the host firewall's verdict for an exposed listener; "" if unknown or loopback
	CPUPercent    float64
	SystemdUnit   string
	ContainerName string
	FirstSeen     bool // true if this socket was absent from the previous scan
}

// Exposure says who can reach a socket's local address.
const (
	ScopeLoopback = "loopback" // 127.0.0.1, ::1: only this machine
	ScopeAll      = "all"      // 0.0.0.0, ::: every interface
	ScopeAddress  = "address"  // one specific address
)

// ScopeOf classifies a bind address by exposure.
func ScopeOf(ip net.IP) string {
	switch {
	case ip == nil || ip.IsUnspecified():
		return ScopeAll
	case ip.IsLoopback():
		return ScopeLoopback
	default:
		return ScopeAddress
	}
}

// Scope is ScopeOf the row's local address.
func (r Row) Scope() string { return ScopeOf(r.LocalAddr) }

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
// JSON output, and web dashboard. Each whitespace-separated word must
// match some field, so "nginx 443" finds nginx's rows on port 443.
func (r Row) Matches(query string) bool {
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !r.matchesTerm(term) {
			return false
		}
	}
	return true
}

func (r Row) matchesTerm(term string) bool {
	return strings.Contains(strconv.Itoa(int(r.LocalPort)), term) ||
		strings.Contains(strconv.Itoa(r.PID), term) ||
		strings.Contains(strings.ToLower(r.ProcessName), term) ||
		strings.Contains(strings.ToLower(r.Username), term) ||
		strings.Contains(strings.ToLower(r.SystemdUnit), term) ||
		strings.Contains(strings.ToLower(r.ContainerName), term) ||
		strings.Contains(strings.ToLower(r.LocalAddr.String()), term) ||
		(r.RemotePort != 0 && strings.Contains(strings.ToLower(r.RemoteAddr.String()), term))
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

	seen  map[Key]bool // sockets observed in any previous Collect call
	init  bool         // false until the first Collect call completes
	users map[uint32]string
}

// NewCollector wires up a Collector with real system backends.
func NewCollector() *Collector {
	return &Collector{
		cpu:    scanner.NewCPUTracker(),
		dns:    dnscache.New(defaultDNSTimeout),
		docker: docker.NewClient(),
		seen:   make(map[Key]bool),
		users:  make(map[uint32]string),
	}
}

// applyFirewall records the firewall's verdict on every listener the
// network can reach; for loopback the firewall does not matter.
func applyFirewall(rows []Row, policy firewall.Policy) {
	for i := range rows {
		r := &rows[i]
		if r.Listening() && r.Scope() != ScopeLoopback {
			r.Firewall = string(policy.Verdict(strings.ToLower(string(r.Protocol)), r.LocalPort))
		}
	}
}

// containerName names the container pid runs in: through the Docker
// (or Podman) API when it knows the container, else from the kubelet's
// records for Kubernetes pods run by containerd or CRI-O.
func (c *Collector) containerName(ctx context.Context, pid int) string {
	cid := docker.ContainerIDForPID(pid)
	if cid == "" {
		return ""
	}
	if c.docker.Available() {
		if name := c.docker.ContainerName(ctx, cid); name != "" {
			return name
		}
	}
	return docker.KubernetesName(cid)
}

// workers lists the holders of conn's socket other than its main process.
func workers(conn scanner.Connection) []int {
	var out []int
	for _, pid := range conn.Owners {
		if pid != conn.PID {
			out = append(out, pid)
		}
	}
	return out
}

// username names a socket's owner. Windows reports the account with the
// socket; Linux and macOS report a uid, resolved once per uid.
func (c *Collector) username(conn scanner.Connection) string {
	if conn.User != "" || runtime.GOOS == "windows" {
		return conn.User
	}
	if conn.PID == 0 && conn.UID == 0 && conn.Inode == 0 {
		return "" // no owner information at all (closing socket)
	}
	if name, ok := c.users[conn.UID]; ok {
		return name
	}
	name := strconv.FormatUint(uint64(conn.UID), 10)
	if u, err := user.LookupId(name); err == nil {
		name = u.Username
	}
	c.users[conn.UID] = name
	return name
}

const defaultDNSTimeout = 800 * time.Millisecond

// Options controls the metadata resolved during a scan.
type Options struct {
	ResolveSystemd  bool
	ResolveDocker   bool
	ResolveDNS      bool
	ResolveFirewall bool
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
			Username:    c.username(conn),
			Workers:     workers(conn),
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
			if opts.ResolveDocker {
				row.ContainerName = c.containerName(ctx, conn.PID)
			}
		}

		if opts.ResolveDNS && conn.State == scanner.StateEstablished && !conn.RemoteAddr.IsUnspecified() {
			if name, ok := c.dns.Lookup(conn.RemoteAddr.String()); ok {
				row.RemoteHost = name
			}
		}

		rows = append(rows, row)
	}

	if opts.ResolveFirewall {
		applyFirewall(rows, firewall.Current())
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
