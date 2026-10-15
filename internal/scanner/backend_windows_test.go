package scanner

import (
	"net"
	"os"
	"testing"
)

func TestWindowsLiveSocketsAndOwnership(t *testing.T) {
	for _, network := range []string{"tcp4", "tcp6", "udp4", "udp6"} {
		t.Run(network, func(t *testing.T) {
			v6 := network[len(network)-1] == '6'
			host := "127.0.0.1:0"
			if v6 {
				host = "[::1]:0"
			}
			var port int
			protocol, state := TCP, StateListen
			if network[:3] == "tcp" {
				ln, err := net.Listen(network, host)
				if err != nil {
					t.Skipf("cannot listen on %s: %v", network, err)
				}
				defer ln.Close()
				port = ln.Addr().(*net.TCPAddr).Port
			} else {
				ln, err := net.ListenPacket(network, host)
				if err != nil {
					t.Skipf("cannot listen on %s: %v", network, err)
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
					if row.State != state || row.ProcessName == "" || row.IPv6 != v6 {
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

func TestDisplayName(t *testing.T) {
	for in, want := range map[string]string{"nginx.exe": "nginx", "Code.EXE": "Code", "System": "System"} {
		if got := displayName(in); got != want {
			t.Errorf("displayName(%q) = %q, want %q", in, got, want)
		}
	}
}
