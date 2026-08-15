// Package scanner enumerates TCP/UDP sockets on Linux by reading the
// /proc/net/{tcp,tcp6,udp,udp6} pseudo-files and resolves each socket to
// the owning process by walking /proc/<pid>/fd.
package scanner

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/padovanl/portop/internal/netutil"
)

// Protocol identifies the transport protocol of a Connection.
type Protocol string

const (
	TCP Protocol = "TCP"
	UDP Protocol = "UDP"
)

// State is the socket state, as reported by the kernel for TCP sockets.
// UDP sockets are always reported as StateUnconn since UDP has no
// connection concept, but a "connected" UDP socket (one that has called
// connect(2)) is reported as StateEstablished for convenience.
type State string

const (
	StateEstablished State = "ESTABLISHED"
	StateSynSent     State = "SYN_SENT"
	StateSynRecv     State = "SYN_RECV"
	StateFinWait1    State = "FIN_WAIT1"
	StateFinWait2    State = "FIN_WAIT2"
	StateTimeWait    State = "TIME_WAIT"
	StateClose       State = "CLOSE"
	StateCloseWait   State = "CLOSE_WAIT"
	StateLastAck     State = "LAST_ACK"
	StateListen      State = "LISTEN"
	StateClosing     State = "CLOSING"
	StateUnconn      State = "UNCONN"
	StateUnknown     State = "UNKNOWN"
)

var tcpStates = map[string]State{
	"01": StateEstablished,
	"02": StateSynSent,
	"03": StateSynRecv,
	"04": StateFinWait1,
	"05": StateFinWait2,
	"06": StateTimeWait,
	"07": StateClose,
	"08": StateCloseWait,
	"09": StateLastAck,
	"0A": StateListen,
	"0B": StateClosing,
}

// Connection describes a single socket found in /proc/net, optionally
// enriched with the owning process once ResolveProcesses has run.
type Connection struct {
	Protocol   Protocol
	LocalAddr  net.IP
	LocalPort  uint16
	RemoteAddr net.IP
	RemotePort uint16
	State      State
	Inode      uint64
	IPv6       bool
	UID        uint32

	PID         int    // 0 if the owning process could not be determined
	ProcessName string // empty if unknown
}

// IsIPv6 reports whether this connection was read from a v6 proc table.
func (c Connection) IsIPv6() bool { return c.IPv6 }

// ProcRoot is the root of the /proc filesystem. It is a variable so tests
// can point it at a fixture directory.
var ProcRoot = "/proc"

var sourceFiles = []struct {
	path     string
	protocol Protocol
	ipv6     bool
}{
	{"net/tcp", TCP, false},
	{"net/tcp6", TCP, true},
	{"net/udp", UDP, false},
	{"net/udp6", UDP, true},
}

// Scan reads all four /proc/net tables and returns the sockets found,
// without process information (see ResolveProcesses).
func Scan() ([]Connection, error) {
	var out []Connection
	for _, src := range sourceFiles {
		conns, err := scanFile(ProcRoot+"/"+src.path, src.protocol, src.ipv6)
		if err != nil {
			if os.IsNotExist(err) {
				continue // e.g. IPv6 disabled: tcp6/udp6 absent
			}
			return nil, err
		}
		out = append(out, conns...)
	}
	return out, nil
}

func scanFile(path string, protocol Protocol, ipv6 bool) ([]Connection, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var conns []Connection
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first {
			first = false // header line
			continue
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		localIP, localPort, err := netutil.ParseHexAddr(fields[1])
		if err != nil {
			continue
		}
		remoteIP, remotePort, err := netutil.ParseHexAddr(fields[2])
		if err != nil {
			continue
		}

		state := StateUnconn
		if protocol == TCP {
			s, ok := tcpStates[strings.ToUpper(fields[3])]
			if ok {
				state = s
			} else {
				state = StateUnknown
			}
		} else if remotePort != 0 {
			// A UDP socket with a non-zero peer has called connect(2).
			state = StateEstablished
		}

		inode, _ := strconv.ParseUint(fields[9], 10, 64)
		uid64, _ := strconv.ParseUint(fields[7], 10, 32)

		conns = append(conns, Connection{
			Protocol:   protocol,
			LocalAddr:  localIP,
			LocalPort:  localPort,
			RemoteAddr: remoteIP,
			RemotePort: remotePort,
			State:      state,
			Inode:      inode,
			IPv6:       ipv6,
			UID:        uint32(uid64),
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scanner: reading %s: %w", path, err)
	}
	return conns, nil
}
