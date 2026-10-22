package traffic

import (
	"io"
	"net"
	"testing"
	"time"
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

	// Different sizes each way, so a count off by one on either the
	// connecting or the accepting side shows.
	const n, m = 100_000, 3_000
	go func() { _, _ = client.Write(make([]byte, n)) }()
	if _, err := io.ReadFull(server, make([]byte, n)); err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = server.Write(make([]byte, m)) }()
	if _, err := io.ReadFull(client, make([]byte, m)); err != nil {
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
	// The last ACK may still be in flight right after the read.
	for i := 0; i < 50 && received.TX != m; i++ {
		time.Sleep(20 * time.Millisecond)
		counters = Sample()
		sent = counters[KeyFor(c.IP, uint16(c.Port), s.IP, uint16(s.Port))]
		received = counters[KeyFor(s.IP, uint16(s.Port), c.IP, uint16(c.Port))]
	}
	if sent.TX != n || received.RX != n {
		t.Errorf("client TX = %d, server RX = %d, want %d", sent.TX, received.RX, n)
	}
	if received.TX != m || sent.RX != m {
		t.Errorf("server TX = %d, client RX = %d, want %d", received.TX, sent.RX, m)
	}
}

// Only connections this host opened lose the byte bytes_acked counts for
// the SYN; accepted ones are recognised by a listener on their local
// address, whichever family and wildcard it uses.
func TestCountersSubtractTheSYNOnlyForOutgoingConnections(t *testing.T) {
	ip := net.ParseIP
	socks := []diagSocket{
		{state: tcpListen, src: ip("0.0.0.0").To4(), sport: 80},
		{state: tcpListen, src: ip("::"), sport: 443},
		{state: tcpListen, src: ip("127.0.0.1").To4(), sport: 5432},
		{state: tcpEstablished, src: ip("10.0.0.5").To4(), sport: 80, dst: ip("10.0.0.9").To4(), dport: 50000, acked: 10, rcv: 4},
		{state: tcpEstablished, src: ip("::ffff:10.0.0.5"), sport: 443, dst: ip("::ffff:10.0.0.9"), dport: 50001, acked: 10},
		{state: tcpEstablished, src: ip("127.0.0.1").To4(), sport: 5432, dst: ip("127.0.0.1").To4(), dport: 50002, acked: 10},
		{state: tcpEstablished, src: ip("10.0.0.5").To4(), sport: 50003, dst: ip("10.0.0.9").To4(), dport: 5432, acked: 10},
		{state: tcpEstablished, src: ip("10.0.0.5").To4(), sport: 50004, dst: ip("10.0.0.9").To4(), dport: 22, acked: 0},
	}
	got := counters(socks)
	want := map[Key]Counter{
		KeyFor(ip("10.0.0.5"), 80, ip("10.0.0.9"), 50000):                {TX: 10, RX: 4},
		KeyFor(ip("::ffff:10.0.0.5"), 443, ip("::ffff:10.0.0.9"), 50001): {TX: 10},
		KeyFor(ip("127.0.0.1"), 5432, ip("127.0.0.1"), 50002):            {TX: 10},
		KeyFor(ip("10.0.0.5"), 50003, ip("10.0.0.9"), 5432):              {TX: 9},
		KeyFor(ip("10.0.0.5"), 50004, ip("10.0.0.9"), 22):                {TX: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d counters, want %d: %v", len(got), len(want), got)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%v = %+v, want %+v", k, got[k], w)
		}
	}
}
