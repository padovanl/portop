package scanner

import (
	"net"
	"os"
	"testing"
)

func TestDarwinLiveSocketsAndOwnership(t *testing.T) {
	for _, network := range []string{"tcp4", "tcp6", "udp4", "udp6"} {
		t.Run(network, func(t *testing.T) {
			host := "127.0.0.1:0"
			if network[len(network)-1] == '6' {
				host = "[::1]:0"
			}
			var port int
			protocol, state := TCP, StateListen
			if network[:3] == "tcp" {
				ln, err := net.Listen(network, host)
				if err != nil {
					t.Fatal(err)
				}
				defer ln.Close()
				port = ln.Addr().(*net.TCPAddr).Port
			} else {
				ln, err := net.ListenPacket(network, host)
				if err != nil {
					t.Fatal(err)
				}
				defer ln.Close()
				port = ln.LocalAddr().(*net.UDPAddr).Port
				protocol, state = UDP, StateUnconn
			}
			rows, err := Scan()
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range ResolveProcesses(rows) {
				if row.Protocol == protocol && int(row.LocalPort) == port && row.PID == os.Getpid() {
					if row.State != state || row.ProcessName == "" || row.UID != uint32(os.Getuid()) || row.IPv6 != (network[len(network)-1] == '6') {
						t.Fatalf("bad socket: %+v", row)
					}
					return
				}
			}
			t.Fatalf("%s socket on port %d with PID %d missing", network, port, os.Getpid())
		})
	}
	if _, err := readProcessTicks(os.Getpid()); err != nil {
		t.Fatal(err)
	}
}
