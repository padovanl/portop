package netutil

import "testing"

func TestParseHexAddrIPv4(t *testing.T) {
	ip, port, err := ParseHexAddr("0100007F:0035")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip.String() != "127.0.0.1" {
		t.Errorf("ip = %s, want 127.0.0.1", ip)
	}
	if port != 53 {
		t.Errorf("port = %d, want 53", port)
	}
}

func TestParseHexAddrIPv4Any(t *testing.T) {
	ip, port, err := ParseHexAddr("00000000:1F90")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip.String() != "0.0.0.0" {
		t.Errorf("ip = %s, want 0.0.0.0", ip)
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}
}

func TestParseHexAddrIPv6Loopback(t *testing.T) {
	ip, port, err := ParseHexAddr("00000000000000000000000001000000:1538")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip.String() != "::1" {
		t.Errorf("ip = %s, want ::1", ip)
	}
	if port != 5432 {
		t.Errorf("port = %d, want 5432", port)
	}
}

func TestParseHexAddrMalformed(t *testing.T) {
	if _, _, err := ParseHexAddr("not-an-address"); err == nil {
		t.Error("expected error for malformed field, got nil")
	}
	if _, _, err := ParseHexAddr("ZZ:1"); err == nil {
		t.Error("expected error for bad hex address, got nil")
	}
	if _, _, err := ParseHexAddr("0100007F:ZZ"); err == nil {
		t.Error("expected error for bad hex port, got nil")
	}
}
