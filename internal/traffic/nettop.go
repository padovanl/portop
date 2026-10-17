package traffic

import (
	"net"
	"strconv"
	"strings"
)

// parseNettop reads `nettop -L 1 -n -x -m tcp -J bytes_in,bytes_out`:
// CSV rows of "time,name,bytes_in,bytes_out,", where a connection's name
// is "tcp4 10.0.0.2:51000<->1.2.3.4:443" (IPv6 ports follow a '.').
func parseNettop(out string) map[Key]Counter {
	counters := map[Key]Counter{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, ",")
		if len(fields) < 4 {
			continue
		}
		name := strings.TrimSpace(fields[1])
		v6 := strings.HasPrefix(name, "tcp6 ")
		if !v6 && !strings.HasPrefix(name, "tcp4 ") {
			continue
		}
		local, remote, ok := strings.Cut(name[5:], "<->")
		if !ok {
			continue
		}
		lip, lport, ok1 := nettopEndpoint(local, v6)
		rip, rport, ok2 := nettopEndpoint(remote, v6)
		rx, err1 := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 64)
		tx, err2 := strconv.ParseUint(strings.TrimSpace(fields[3]), 10, 64)
		if !ok1 || !ok2 || err1 != nil || err2 != nil {
			continue
		}
		counters[KeyFor(lip, lport, rip, rport)] = Counter{RX: rx, TX: tx}
	}
	return counters
}

func nettopEndpoint(s string, v6 bool) (net.IP, uint16, bool) {
	s = strings.TrimSpace(s)
	sep := strings.LastIndexByte(s, ':')
	if v6 && !strings.HasPrefix(s, "[") {
		sep = strings.LastIndexByte(s, '.')
	}
	if sep < 0 {
		return nil, 0, false
	}
	host, portStr := strings.Trim(s[:sep], "[]"), s[sep+1:]
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || port == 0 {
		return nil, 0, false
	}
	return ip, uint16(port), true
}
