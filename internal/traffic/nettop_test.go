package traffic

import (
	"net"
	"testing"
)

func TestParseNettop(t *testing.T) {
	// Lines as macOS 15 prints them (from CI), plus the time-first layout.
	out := ",bytes_in,bytes_out,\n" +
		"launchd.1,0,0,\n" +
		"tcp6 *.22<->*.*,,,\n" +
		"tcp4 *:22<->*:*,,,\n" +
		"trustd.157,13836,2960,\n" +
		"tcp4 10.0.0.2:51000<->93.184.216.34:443,1200,300,\n" +
		"10:01:02.123456,tcp6 fe80::1%en0.49152<->2001:db8::7.443,50,60,\n" +
		"tcp6 ::1.8080<->::1.51001,7,8,\n"
	got := parseNettop(out)
	cases := map[Key]Counter{
		KeyFor(net.ParseIP("10.0.0.2"), 51000, net.ParseIP("93.184.216.34"), 443): {RX: 1200, TX: 300},
		KeyFor(net.ParseIP("fe80::1"), 49152, net.ParseIP("2001:db8::7"), 443):    {RX: 50, TX: 60},
		KeyFor(net.ParseIP("::1"), 8080, net.ParseIP("::1"), 51001):               {RX: 7, TX: 8},
	}
	if len(got) != len(cases) {
		t.Fatalf("got %d connections: %v", len(got), got)
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("%v = %+v, want %+v", k, got[k], want)
		}
	}
}
