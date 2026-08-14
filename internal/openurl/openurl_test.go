package openurl

import "testing"

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
