package scanner

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestMPTCPRequest(t *testing.T) {
	req := mptcpRequest(unix.AF_INET6, 7)
	if len(req) != 72 || binary.NativeEndian.Uint32(req) != 72 ||
		binary.NativeEndian.Uint16(req[4:]) != unix.SOCK_DIAG_BY_FAMILY ||
		binary.NativeEndian.Uint32(req[8:]) != 7 {
		t.Fatalf("bad netlink header: %x", req)
	}
	body := req[16:]
	if body[0] != unix.AF_INET6 || body[1] != unix.IPPROTO_TCP || body[2] != 2 || binary.NativeEndian.Uint32(body[4:]) != ^uint32(0) {
		t.Fatalf("bad MPTCP request: %x", body)
	}
}

func TestParseMPTCPDiag(t *testing.T) {
	for _, tc := range []struct {
		name          string
		family        byte
		local, remote string
		state         byte
		want          State
	}{
		{"ipv4 listener", unix.AF_INET, "0.0.0.0", "0.0.0.0", 10, StateListen},
		{"ipv4 established", unix.AF_INET, "192.0.2.1", "192.0.2.2", 1, StateEstablished},
		{"ipv6 established", unix.AF_INET6, "2001:db8::1", "2001:db8::2", 1, StateEstablished},
		{"unknown state", unix.AF_INET6, "::1", "::", 255, StateUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, 72)
			data[0], data[1] = tc.family, tc.state
			binary.BigEndian.PutUint16(data[4:], 8080)
			binary.BigEndian.PutUint16(data[6:], 443)
			local, remote := net.ParseIP(tc.local), net.ParseIP(tc.remote)
			if tc.family == unix.AF_INET {
				local, remote = local.To4(), remote.To4()
			}
			copy(data[8:24], local)
			copy(data[24:40], remote)
			binary.NativeEndian.PutUint32(data[64:], 1000)
			binary.NativeEndian.PutUint32(data[68:], 12345)
			data = append(data, ulpAttributes("mptcp")...)
			c, ok := parseMPTCPDiag(data)
			if !ok || c.Protocol != TCP || !c.MPTCP || !c.LocalAddr.Equal(local) || !c.RemoteAddr.Equal(remote) ||
				c.LocalPort != 8080 || c.RemotePort != 443 || c.State != tc.want || c.UID != 1000 ||
				c.Inode != 12345 || c.IPv6 != (tc.family == unix.AF_INET6) {
				t.Fatalf("parsed socket: %+v, %v", c, ok)
			}
			// Socket addresses must survive reuse of the receive buffer.
			clear(data)
			if !c.LocalAddr.Equal(local) || !c.RemoteAddr.Equal(remote) {
				t.Fatal("addresses alias the receive buffer")
			}
		})
	}
	for _, data := range [][]byte{nil, make([]byte, 71), make([]byte, 72)} {
		if _, ok := parseMPTCPDiag(data); ok {
			t.Fatal("accepted short message or unsupported address family")
		}
	}
}

func TestMPTCPDiagError(t *testing.T) {
	data := make([]byte, 4)
	code := int32(-int32(unix.EOPNOTSUPP))
	binary.NativeEndian.PutUint32(data, uint32(code))
	if err := mptcpDiagError(data); !errors.Is(err, unix.EOPNOTSUPP) {
		t.Fatalf("kernel error = %v", err)
	}
	clear(data)
	if err := mptcpDiagError(data); err != nil {
		t.Fatalf("acknowledgement = %v", err)
	}
	if err := mptcpDiagError(data[:3]); err == nil {
		t.Fatal("accepted truncated error")
	}
}

func TestLinuxLiveMPTCPSockets(t *testing.T) {
	for _, network := range []string{"tcp4", "tcp6"} {
		t.Run(network, func(t *testing.T) {
			host := "127.0.0.1:0"
			if network == "tcp6" {
				host = "[::1]:0"
			}
			lc := net.ListenConfig{}
			lc.SetMultipathTCP(true)
			ln, err := lc.Listen(context.Background(), network, host)
			if err != nil {
				t.Skipf("listener unavailable: %v", err)
			}
			defer ln.Close()
			raw, err := ln.(*net.TCPListener).SyscallConn()
			if err != nil {
				t.Fatal(err)
			}
			var protocol int
			var sockErr error
			if err := raw.Control(func(fd uintptr) {
				protocol, sockErr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PROTOCOL)
			}); err != nil {
				t.Fatal(err)
			}
			if sockErr != nil {
				t.Fatal(sockErr)
			}
			if protocol != unix.IPPROTO_MPTCP {
				t.Skip("kernel does not support or enable MPTCP")
			}
			d := net.Dialer{Timeout: 2 * time.Second}
			d.SetMultipathTCP(true)
			client, err := d.Dial(network, ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := ln.(*net.TCPListener).SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			server, err := ln.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			mptcp, err := client.(*net.TCPConn).MultipathTCP()
			if err != nil || !mptcp {
				t.Fatalf("MPTCP negotiation: %v, %v", mptcp, err)
			}
			if _, err := scanMPTCP(); err != nil {
				if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EINVAL) {
					t.Skipf("MPTCP diagnostics unavailable: %v", err)
				}
				t.Fatal(err)
			}
			conns, err := Scan()
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []struct {
				port  int
				state State
			}{
				{ln.Addr().(*net.TCPAddr).Port, StateListen},
				{client.LocalAddr().(*net.TCPAddr).Port, StateEstablished},
			} {
				found := false
				for _, c := range conns {
					if c.MPTCP && c.Protocol == TCP && int(c.LocalPort) == want.port && c.State == want.state {
						if c.IPv6 != (network == "tcp6") {
							t.Fatalf("bad MPTCP socket: %+v", c)
						}
						found = true
					}
				}
				if !found {
					t.Fatalf("MPTCP %s port %d missing: %+v", want.state, want.port, conns)
				}
			}
		})
	}
}

func ulpAttributes(name string) []byte {
	attr := func(kind uint16, payload []byte) []byte {
		length := 4 + len(payload)
		out := make([]byte, (length+3)&^3)
		binary.NativeEndian.PutUint16(out, uint16(length))
		binary.NativeEndian.PutUint16(out[2:], kind)
		copy(out[4:], payload)
		return out
	}
	return attr(19|0x8000, attr(1, append([]byte(name), 0)))
}

func TestMPTCPRequiresULPName(t *testing.T) {
	data := make([]byte, 72)
	data[0] = unix.AF_INET
	for _, attrs := range [][]byte{nil, ulpAttributes("tls"), {3, 0, 19, 0}, {255, 0, 19, 0}, {8, 0, 19, 0, 3, 0, 1, 0}} {
		if _, ok := parseMPTCPDiag(append(append([]byte(nil), data...), attrs...)); ok {
			t.Fatalf("marked non-MPTCP or malformed attributes: %x", attrs)
		}
	}
}

func TestMarkMPTCPPreservesRows(t *testing.T) {
	subflow := Connection{Protocol: TCP, MPTCP: true, LocalAddr: net.ParseIP("192.0.2.1"), LocalPort: 50000,
		RemoteAddr: net.ParseIP("192.0.2.2"), RemotePort: 443}
	tcp := subflow
	tcp.MPTCP, tcp.PID, tcp.Inode = false, 123, 456
	other := tcp
	other.RemotePort = 8443
	udp := tcp
	udp.Protocol = UDP
	rows := []Connection{tcp, other, udp}
	markMPTCP(rows, []Connection{subflow, subflow})
	if len(rows) != 3 || !rows[0].MPTCP || rows[0].Protocol != TCP || rows[0].PID != 123 || rows[0].Inode != 456 ||
		rows[1].MPTCP || rows[2].MPTCP || rows[2].Protocol != UDP {
		t.Fatalf("marking changed unrelated sockets or metadata: %+v", rows)
	}
}
