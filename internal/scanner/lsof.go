package scanner

import (
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
)

// -F0 terminates fields with NUL and process/file sets with a newline.
// Retain process fields across files; flush only at the next f/p record.
func parseLsof(data []byte) []Connection {
	var out []Connection
	var current Connection
	var pid int
	var uid uint32
	var command, endpoint string
	flush := func() {
		if pid <= 0 || (current.Protocol != TCP && current.Protocol != UDP) || endpoint == "" {
			return
		}
		ends := strings.SplitN(endpoint, "->", 2)
		var ok bool
		current.LocalAddr, current.LocalPort, ok = parseLsofAddr(ends[0], current.IPv6)
		// lsof also lists sockets that were never bound ("*:*"); /proc and
		// the Windows tables do not, and they are not ports anyone uses.
		if !ok || current.LocalPort == 0 {
			return
		}
		current.RemoteAddr = net.IPv4zero
		if current.IPv6 {
			current.RemoteAddr = net.IPv6zero
		}
		if len(ends) == 2 {
			current.RemoteAddr, current.RemotePort, ok = parseLsofAddr(ends[1], current.IPv6)
			if !ok {
				return
			}
		}
		if current.Protocol == UDP {
			current.State = StateUnconn
			if current.RemotePort != 0 {
				current.State = StateEstablished
			}
		} else if current.State == "" {
			current.State = StateUnknown
		}
		current.PID, current.UID, current.ProcessName = pid, uid, command
		out = append(out, current)
	}
	for _, field := range strings.Split(string(data), "\x00") {
		field = strings.TrimLeft(field, "\n")
		if field == "" {
			continue
		}
		value := field[1:]
		switch field[0] {
		case 'p':
			flush()
			current, endpoint = Connection{}, ""
			pid, _ = strconv.Atoi(value)
			uid, command = 0, ""
		case 'c':
			command = value
		case 'u':
			v, _ := strconv.ParseUint(value, 10, 32)
			uid = uint32(v)
		case 'f':
			flush()
			current, endpoint = Connection{}, ""
		case 't':
			current.IPv6 = value == "IPv6"
		case 'P':
			current.Protocol = Protocol(value)
		case 'n':
			endpoint = value
		case 'T':
			if strings.HasPrefix(value, "ST=") {
				state := strings.TrimPrefix(value, "ST=")
				// Darwin/lsof spell these differently from Linux.
				switch state {
				case "CLOSED":
					state = string(StateClose)
				case "SYN_RECEIVED":
					state = string(StateSynRecv)
				case "FIN_WAIT_1":
					state = string(StateFinWait1)
				case "FIN_WAIT_2":
					state = string(StateFinWait2)
				}
				current.State = State(state)
			}
		}
	}
	flush()
	return out
}

func parseLsofAddr(value string, ipv6 bool) (net.IP, uint16, bool) {
	idx := strings.LastIndexByte(value, ':')
	if idx < 0 {
		return nil, 0, false
	}
	host, port := value[:idx], value[idx+1:]
	var n uint64
	var err error
	if port != "*" {
		n, err = strconv.ParseUint(port, 10, 16)
	}
	if err != nil {
		return nil, 0, false
	}
	host = strings.Trim(host, "[]")
	if zone := strings.LastIndexByte(host, '%'); zone >= 0 {
		host = host[:zone]
	}
	if host == "*" {
		if ipv6 {
			return net.IPv6zero, uint16(n), true
		}
		return net.IPv4zero, uint16(n), true
	}
	ip := net.ParseIP(host)
	return ip, uint16(n), ip != nil
}

// ps time is [[days-]hours:]minutes:seconds, with fractional seconds.
func parseCPUTime(value string) (uint64, error) {
	var seconds float64
	if day, rest, ok := strings.Cut(value, "-"); ok {
		days, err := strconv.ParseUint(day, 10, 32)
		if err != nil {
			return 0, err
		}
		seconds = float64(days) * 86400
		value = rest
	}
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid ps CPU time %q", value)
	}
	var clock float64
	for _, part := range parts {
		n, err := strconv.ParseFloat(part, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid ps CPU time %q", value)
		}
		clock = clock*60 + n
	}
	return uint64((seconds + clock) * clockTicksPerSec), nil
}

// mergeShared folds the rows lsof prints once per process for a socket
// several processes hold (a server master and its workers) into one row
// per socket, owned by the main process. parents maps each PID to its
// parent for the PIDs given.
func mergeShared(conns []Connection, parents func(pids []int) map[int]int) []Connection {
	type socketKey struct {
		proto         Protocol
		ipv6          bool
		local, remote string
		state         State
	}
	keyOf := func(c Connection) socketKey {
		return socketKey{c.Protocol, c.IPv6,
			net.JoinHostPort(c.LocalAddr.String(), strconv.Itoa(int(c.LocalPort))),
			net.JoinHostPort(c.RemoteAddr.String(), strconv.Itoa(int(c.RemotePort))), c.State}
	}
	index := map[socketKey]int{}
	owners := map[int][]int{}
	names := map[int]string{}
	uids := map[int]uint32{}
	var out []Connection
	for _, c := range conns {
		names[c.PID] = c.ProcessName
		uids[c.PID] = c.UID
		k := keyOf(c)
		if i, ok := index[k]; ok {
			if !slices.Contains(owners[i], c.PID) {
				owners[i] = append(owners[i], c.PID)
			}
			continue
		}
		index[k] = len(out)
		owners[len(out)] = []int{c.PID}
		out = append(out, c)
	}
	var shared []int
	for _, pids := range owners {
		if len(pids) > 1 {
			shared = append(shared, pids...)
		}
	}
	if len(shared) == 0 {
		return out
	}
	parentOf := parents(shared)
	for i, pids := range owners {
		if len(pids) > 1 {
			out[i].setOwners(pids, func(pid int) int { return parentOf[pid] }, func(pid int) string { return names[pid] })
			out[i].UID = uids[out[i].PID]
		}
	}
	return out
}

// parsePSParents reads `ps -o pid=,ppid=` output.
func parsePSParents(out []byte) map[int]int {
	parents := map[int]int{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 == nil && err2 == nil {
			parents[pid] = ppid
		}
	}
	return parents
}
