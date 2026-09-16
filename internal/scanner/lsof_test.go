package scanner

import "testing"

func TestParseLsof(t *testing.T) {
	data := "p42\x00cmy server\x00u501\x00\n" +
		"f3\x00tIPv4\x00PTCP\x00n*:8080\x00TST=LISTEN\x00\n" +
		"f4\x00tIPv6\x00PTCP\x00n[::1]:9000->[fe80::1%en0]:443\x00TST=ESTABLISHED\x00\n" +
		"f5\x00tIPv6\x00PUDP\x00n*:5353\x00\n" +
		"p99\x00cclient\x00u502\x00\n" +
		"f7\x00tIPv4\x00PUDP\x00n127.0.0.1:5000->127.0.0.1:53\x00\n" +
		"f8\x00tIPv4\x00PTCP\x00ninvalid\x00\n"
	rows := parseLsof([]byte(data))
	if len(rows) != 4 {
		t.Fatalf("got %d rows: %+v", len(rows), rows)
	}
	if rows[0].PID != 42 || rows[0].UID != 501 || rows[0].ProcessName != "my server" || rows[0].LocalPort != 8080 || rows[0].State != StateListen || !rows[0].LocalAddr.IsUnspecified() {
		t.Fatalf("listener: %+v", rows[0])
	}
	if !rows[1].IPv6 || rows[1].RemoteAddr.String() != "fe80::1" || rows[1].RemotePort != 443 || rows[1].State != StateEstablished {
		t.Fatalf("IPv6: %+v", rows[1])
	}
	if rows[2].State != StateUnconn || !rows[2].IPv6 || rows[2].RemoteAddr.String() != "::" {
		t.Fatalf("UDP: %+v", rows[2])
	}
	if rows[3].PID != 99 || rows[3].UID != 502 || rows[3].ProcessName != "client" || rows[3].State != StateEstablished {
		t.Fatalf("ownership/UDP: %+v", rows[3])
	}
}

func TestParseLsofAddrRejectsInvalidPorts(t *testing.T) {
	for _, value := range []string{"127.0.0.1:65536", "127.0.0.1:-1", "127.0.0.1:http", "garbage:80", "127.0.0.1"} {
		if _, _, ok := parseLsofAddr(value, false); ok {
			t.Errorf("accepted %q", value)
		}
	}
}

func TestParseCPUTime(t *testing.T) {
	for input, want := range map[string]uint64{"0:01.25": 125, "01:02:03.50": 372350, "2-01:00:00": 17640000} {
		got, err := parseCPUTime(input)
		if err != nil || got != want {
			t.Errorf("%q = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"", "bad", "1:-2", "1:xx"} {
		if _, err := parseCPUTime(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}
