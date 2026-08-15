package services

import (
	"os"
	"strings"
	"sync"
	"testing"
)

const sample = `# comment line
ssh		22/tcp				# SSH Remote Login Protocol
http		80/tcp		www www-http
https		443/tcp
domain		53/tcp
domain		53/udp
` + "custom-svc\t9999/tcp\tsomealias\t# a made up one\n"

func TestParse(t *testing.T) {
	out := make(map[key]string)
	parse(strings.NewReader(sample), out)

	cases := []struct {
		port  uint16
		proto string
		want  string
		ok    bool
	}{
		{22, "tcp", "ssh", true},
		{80, "tcp", "http", true},
		{443, "tcp", "https", true},
		{53, "udp", "domain", true},
		{9999, "tcp", "custom-svc", true},
		{9999, "udp", "", false},
		{1, "tcp", "", false},
	}
	for _, c := range cases {
		got, ok := out[key{port: c.port, proto: c.proto}]
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("port %d/%s: got (%q, %v), want (%q, %v)", c.port, c.proto, got, ok, c.want, c.ok)
		}
	}
}

func TestLookupUsesPathAndCaches(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/services"
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}

	Path = path
	once = sync.Once{}

	name, ok := Lookup(22, "TCP")
	if !ok || name != "ssh" {
		t.Errorf("Lookup(22, TCP) = (%q, %v), want (ssh, true)", name, ok)
	}

	name, ok = Lookup(59999, "TCP")
	if ok {
		t.Errorf("Lookup(59999, TCP) = (%q, %v), want not ok", name, ok)
	}
}
