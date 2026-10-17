// Package traffic reads how many bytes each TCP connection has sent and
// received, so portop can show per-connection throughput. Linux reads the
// kernel's socket diagnostics (as ss -ti does) and macOS reads nettop;
// Windows only keeps these counters for connections an administrator has
// enabled statistics on, so it reports nothing.
package traffic

import (
	"net"
	"strconv"
)

// Key identifies a TCP connection by its endpoints.
type Key struct {
	Local, Remote string
}

// KeyFor builds the Key of a connection.
func KeyFor(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) Key {
	return Key{
		Local:  net.JoinHostPort(localIP.String(), strconv.Itoa(int(localPort))),
		Remote: net.JoinHostPort(remoteIP.String(), strconv.Itoa(int(remotePort))),
	}
}

// Counter is a connection's lifetime totals.
type Counter struct {
	RX, TX uint64 // bytes received, bytes sent and acknowledged
}

// Sample returns the counters of every TCP connection the platform can
// report, or nil when it reports none (unsupported, or not permitted).
func Sample() map[Key]Counter {
	counters, err := sample()
	if err != nil {
		return nil
	}
	return counters
}
