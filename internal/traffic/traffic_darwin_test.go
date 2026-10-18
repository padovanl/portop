package traffic

import (
	"io"
	"net"
	"strings"
	"testing"
)

// Runs the real nettop: a loopback connection carrying a known number of
// bytes must show up with at least that many bytes on both ends.
func TestDarwinSampleCountsALiveConnection(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			accepted <- c
		}
	}()
	client, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	defer server.Close()
	const n = 100_000
	go func() { _, _ = client.Write(make([]byte, n)) }()
	if _, err := io.ReadFull(server, make([]byte, n)); err != nil {
		t.Fatal(err)
	}

	counters, err := sample()
	if err != nil {
		t.Fatalf("nettop: %v", err)
	}
	c := client.LocalAddr().(*net.TCPAddr)
	s := server.LocalAddr().(*net.TCPAddr)
	sent := counters[KeyFor(c.IP, uint16(c.Port), s.IP, uint16(s.Port))]
	received := counters[KeyFor(s.IP, uint16(s.Port), c.IP, uint16(c.Port))]
	if sent.TX < n || received.RX < n {
		// The raw output shows what the parser did not understand.
		raw, err := nettopOutput()
		lines := strings.Split(string(raw), "\n")
		t.Logf("nettop %v: err=%v, %d lines", nettopArgs, err, len(lines))
		for i, line := range lines {
			if i == 12 {
				break
			}
			t.Logf("nettop line %d: %q", i, line)
		}
		t.Fatalf("client TX = %d, server RX = %d, want at least %d (%d connections parsed)", sent.TX, received.RX, n, len(counters))
	}
}
