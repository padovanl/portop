// Package netutil decodes the hex-encoded address/port fields used by
// Linux's /proc/net/{tcp,tcp6,udp,udp6} pseudo-files.
package netutil

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ParseHexAddr decodes a "HEXADDR:HEXPORT" field (as found in /proc/net/tcp
// and friends) into an IP and a port. It supports both the 8-hex-digit
// IPv4 form and the 32-hex-digit IPv6 form.
func ParseHexAddr(field string) (net.IP, uint16, error) {
	parts := strings.SplitN(field, ":", 2)
	if len(parts) != 2 {
		return nil, 0, fmt.Errorf("netutil: malformed address field %q", field)
	}
	ipHex, portHex := parts[0], parts[1]

	port, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return nil, 0, fmt.Errorf("netutil: bad port in %q: %w", field, err)
	}

	raw, err := hex.DecodeString(ipHex)
	if err != nil {
		return nil, 0, fmt.Errorf("netutil: bad address in %q: %w", field, err)
	}

	var ip net.IP
	switch len(raw) {
	case 4:
		// Stored as a little-endian 32-bit word: reverse the 4 bytes.
		ip = net.IPv4(raw[3], raw[2], raw[1], raw[0])
	case 16:
		// Stored as four little-endian 32-bit words: reverse each word.
		ip = make(net.IP, 16)
		for word := 0; word < 4; word++ {
			for b := 0; b < 4; b++ {
				ip[word*4+b] = raw[word*4+(3-b)]
			}
		}
	default:
		return nil, 0, fmt.Errorf("netutil: unexpected address length %d in %q", len(raw), field)
	}

	return ip, uint16(port), nil
}
