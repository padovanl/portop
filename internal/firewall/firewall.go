// Package firewall reports whether the host firewall lets traffic reach a
// listening port. It reads the firewall's own tools (ufw and firewalld on
// Linux, the application firewall on macOS, the firewall profiles on
// Windows); when they are missing, cannot be read without privileges or
// decide per program, the verdict is Unknown.
package firewall

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// Verdict is what the firewall does with incoming traffic to a port.
type Verdict string

const (
	Unknown    Verdict = ""
	Allowed    Verdict = "allowed"    // reachable from anywhere (or no firewall running)
	Restricted Verdict = "restricted" // allowed only from some sources
	Blocked    Verdict = "blocked"
)

// Policy answers for one protocol ("tcp" or "udp") and port.
type Policy interface {
	Verdict(proto string, port uint16) Verdict
}

// unknownPolicy is used when no firewall could be read.
type unknownPolicy struct{}

func (unknownPolicy) Verdict(string, uint16) Verdict { return Unknown }

// allowAll is a firewall that is installed but not running.
type allowAll struct{}

func (allowAll) Verdict(string, uint16) Verdict { return Allowed }

// blockAll blocks every incoming connection.
type blockAll struct{}

func (blockAll) Verdict(string, uint16) Verdict { return Blocked }

const refresh = 30 * time.Second

var cache struct {
	mu     sync.Mutex
	at     time.Time
	policy Policy
}

// Current returns the host's firewall policy, read at most every 30
// seconds: rules rarely change and reading them runs external tools.
func Current() Policy {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.policy == nil || time.Since(cache.at) > refresh {
		cache.policy = load()
		if cache.policy == nil {
			cache.policy = unknownPolicy{}
		}
		cache.at = time.Now()
	}
	return cache.policy
}

// portRange is an inclusive range of ports for one protocol ("" for both).
type portRange struct {
	proto    string
	from, to uint16
}

func (r portRange) covers(proto string, port uint16) bool {
	return (r.proto == "" || r.proto == proto) && port >= r.from && port <= r.to
}

// parsePorts reads "22", "80,443/tcp", "8000:8010/udp" or "60000-61000/udp".
func parsePorts(spec string) ([]portRange, bool) {
	ports, proto, _ := strings.Cut(spec, "/")
	proto = strings.ToLower(proto)
	if proto != "" && proto != "tcp" && proto != "udp" {
		return nil, false
	}
	var out []portRange
	for _, part := range strings.Split(ports, ",") {
		lo, hi, isRange := strings.Cut(part, ":")
		if !isRange {
			lo, hi, isRange = strings.Cut(part, "-")
		}
		from, err := strconv.ParseUint(lo, 10, 16)
		if err != nil || from == 0 {
			return nil, false
		}
		to := from
		if isRange {
			if to, err = strconv.ParseUint(hi, 10, 16); err != nil || to < from {
				return nil, false
			}
		}
		out = append(out, portRange{proto, uint16(from), uint16(to)})
	}
	return out, len(out) > 0
}
