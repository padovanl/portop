package traffic

import (
	"net"
	"strconv"
	"strings"
)

// parseNettop reads `nettop -L 1 -n -x -m tcp -J bytes_in,bytes_out`: CSV
// rows of "name,bytes_in,bytes_out," (some versions put a time column
// first), where a connection's name is "tcp4 10.0.0.2:51000<->1.2.3.4:443"
// and an IPv6 port follows a '.' ("tcp6 ::1.8080<->::1.51000").
func parseNettop(out string) map[Key]Counter {
	counters := map[Key]Counter{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, ",")
		at := -1
		for i, f := range fields {
			if strings.HasPrefix(f, "tcp4 ") || strings.HasPrefix(f, "tcp6 ") {
				at = i
				break
			}
		}
		if at < 0 || at+2 >= len(fields) {
			continue
		}
		name := strings.TrimSpace(fields[at])
		v6 := strings.HasPrefix(name, "tcp6 ")
		local, remote, ok := strings.Cut(name[5:], "<->")
		if !ok {
			continue
		}
		lip, lport, ok1 := nettopEndpoint(local, v6)
		rip, rport, ok2 := nettopEndpoint(remote, v6)
		rx, err1 := strconv.ParseUint(strings.TrimSpace(fields[at+1]), 10, 64)
		tx, err2 := strconv.ParseUint(strings.TrimSpace(fields[at+2]), 10, 64)
		if !ok1 || !ok2 || err1 != nil || err2 != nil {
			continue // listeners ("*:22<->*:*") and rows without counters
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
