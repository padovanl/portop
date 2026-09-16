package scanner

import (
	"fmt"
	"net"
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
		if !ok {
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
