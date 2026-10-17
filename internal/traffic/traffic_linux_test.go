package traffic

import (
	"io"
	"net"
	"testing"
)

// A live loopback connection carrying a known number of bytes shows up
// with those counters, on both ends.
func TestSampleCountsALiveConnection(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
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

	counters := Sample()
	if counters == nil {
		t.Fatal("no counters")
	}
	c := client.LocalAddr().(*net.TCPAddr)
	s := server.LocalAddr().(*net.TCPAddr)
	sent := counters[KeyFor(c.IP, uint16(c.Port), s.IP, uint16(s.Port))]
	received := counters[KeyFor(s.IP, uint16(s.Port), c.IP, uint16(c.Port))]
	if sent.TX != n || received.RX != n {
		t.Fatalf("client TX = %d, server RX = %d, want %d", sent.TX, received.RX, n)
	}
}
