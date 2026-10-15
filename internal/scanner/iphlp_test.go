package scanner

import (
	"encoding/binary"
	"net"
	"testing"
)

// table builds an IP Helper table: a little-endian row count followed by
// the given rows.
func table(rows ...[]byte) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(len(rows)))
	for _, r := range rows {
		buf = append(buf, r...)
	}
	return buf
}

func le(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// netPort encodes a port as the API does: network byte order in the
// first two bytes of a DWORD.
func netPort(p uint16) []byte { return []byte{byte(p >> 8), byte(p), 0xAA, 0xBB} }

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestParseTCP4Table(t *testing.T) {
	listen := cat(le(2), []byte{0, 0, 0, 0}, netPort(8080), []byte{0, 0, 0, 0}, netPort(4321), le(1234))
	estab := cat(le(5), []byte{127, 0, 0, 1}, netPort(51000), []byte{10, 1, 2, 3}, netPort(443), le(99))
	got := parseTCP4Table(table(listen, estab))
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].State != StateListen || got[0].LocalPort != 8080 || got[0].PID != 1234 || got[0].RemotePort != 0 {
		t.Errorf("listener decoded as %+v", got[0])
	}
	if !got[0].LocalAddr.Equal(net.IPv4zero) || got[0].IPv6 {
		t.Errorf("listener address %v ipv6=%v", got[0].LocalAddr, got[0].IPv6)
	}
	c := got[1]
	if c.State != StateEstablished || !c.LocalAddr.Equal(net.IPv4(127, 0, 0, 1)) || c.LocalPort != 51000 ||
		!c.RemoteAddr.Equal(net.IPv4(10, 1, 2, 3)) || c.RemotePort != 443 || c.PID != 99 || c.Protocol != TCP {
		t.Errorf("established decoded as %+v", c)
	}
}

func TestParseTCP6Table(t *testing.T) {
	local := net.ParseIP("::1").To16()
	remote := net.ParseIP("2001:db8::7").To16()
	row := cat(local, le(0), netPort(9000), remote, le(0), netPort(55555), le(5), le(42))
	got := parseTCP6Table(table(row))
	if len(got) != 1 {
		t.Fatalf("got %d rows", len(got))
	}
	c := got[0]
	if !c.IPv6 || !c.LocalAddr.Equal(local) || c.LocalPort != 9000 || !c.RemoteAddr.Equal(remote) ||
		c.RemotePort != 55555 || c.State != StateEstablished || c.PID != 42 {
		t.Errorf("decoded as %+v", c)
	}
}

func TestParseUDPTables(t *testing.T) {
	v4 := parseUDP4Table(table(cat([]byte{192, 168, 1, 5}, netPort(53), le(7))))
	if len(v4) != 1 || v4[0].Protocol != UDP || v4[0].LocalPort != 53 || v4[0].PID != 7 ||
		v4[0].State != StateUnconn || !v4[0].LocalAddr.Equal(net.IPv4(192, 168, 1, 5)) {
		t.Errorf("udp4 decoded as %+v", v4)
	}
	v6 := parseUDP6Table(table(cat(net.IPv6zero.To16(), le(0), netPort(5353), le(8))))
	if len(v6) != 1 || !v6[0].IPv6 || v6[0].LocalPort != 5353 || v6[0].PID != 8 {
		t.Errorf("udp6 decoded as %+v", v6)
	}
}

func TestParseTableTruncatedAndUnknownState(t *testing.T) {
	row := cat(le(77), []byte{0, 0, 0, 0}, netPort(1), []byte{0, 0, 0, 0}, netPort(0), le(1))
	buf := table(row, row)
	got := parseTCP4Table(buf[:len(buf)-3]) // second row cut short
	if len(got) != 1 {
		t.Fatalf("got %d rows from truncated table, want 1", len(got))
	}
	if got[0].State != StateUnknown {
		t.Errorf("state = %s, want UNKNOWN", got[0].State)
	}
	if parseTCP4Table(nil) != nil || parseUDP6Table([]byte{1}) != nil {
		t.Error("short buffers should yield no rows")
	}
}
