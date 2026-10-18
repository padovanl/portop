package openurl

import (
	"net"
	"testing"
)

func TestURLForPort(t *testing.T) {
	cases := []struct {
		port uint16
		want string
	}{
		{80, "http://localhost:80"},
		{3000, "http://localhost:3000"},
		{443, "https://localhost:443"},
		{8443, "https://localhost:8443"},
		{8080, "http://localhost:8080"},
	}
	for _, c := range cases {
		if got := URLForPort(c.port); got != c.want {
			t.Errorf("URLForPort(%d) = %q, want %q", c.port, got, c.want)
		}
	}
}

func TestURLForListener(t *testing.T) {
	cases := []struct {
		addr string
		port uint16
		want string
	}{
		{"0.0.0.0", 8080, "http://localhost:8080"},
		{"::", 8080, "http://localhost:8080"},
		{"127.0.0.1", 3000, "http://localhost:3000"},
		{"::1", 3000, "http://localhost:3000"},
		{"192.168.1.5", 8080, "http://192.168.1.5:8080"},
		{"192.168.1.5", 443, "https://192.168.1.5:443"},
		{"fe80::1", 8443, "https://[fe80::1]:8443"},
	}
	for _, c := range cases {
		if got := URLForListener(net.ParseIP(c.addr), c.port); got != c.want {
			t.Errorf("URLForListener(%s, %d) = %q, want %q", c.addr, c.port, got, c.want)
		}
	}
}
