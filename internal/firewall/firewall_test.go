package firewall

import (
	"os"
	"path/filepath"
	"testing"
)

const ufwStatus = `Status: active
Logging: on (low)
Default: deny (incoming), allow (outgoing), disabled (routed)
New profiles: skip

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW IN    Anywhere
80,443/tcp                 ALLOW IN    Anywhere
8000:8010/tcp              ALLOW IN    Anywhere
53                         ALLOW IN    Anywhere
OpenSSH                    ALLOW IN    Anywhere
5432/tcp                   ALLOW IN    192.168.1.0/24
3306/tcp                   DENY IN     10.0.0.0/8
3306/tcp                   ALLOW IN    Anywhere
6379/tcp                   DENY IN     Anywhere
6379/tcp                   ALLOW IN    Anywhere
10.0.0.5 9000/tcp          ALLOW IN    Anywhere
22/tcp (v6)                ALLOW IN    Anywhere (v6)
`

func TestUFW(t *testing.T) {
	p, ok := parseUFW(ufwStatus)
	if !ok {
		t.Fatal("status not recognised")
	}
	cases := []struct {
		proto string
		port  uint16
		want  Verdict
	}{
		{"tcp", 22, Allowed},
		{"tcp", 443, Allowed},
		{"tcp", 8005, Allowed},
		{"udp", 53, Allowed}, // no protocol: both
		{"udp", 22, Blocked}, // tcp-only rule, deny by default
		{"tcp", 5432, Restricted},
		{"tcp", 3306, Allowed}, // a deny for some sources does not block the rest
		{"tcp", 6379, Blocked}, // first matching rule wins
		{"tcp", 9000, Allowed},
		{"tcp", 8080, Blocked},
	}
	for _, c := range cases {
		if got := p.Verdict(c.proto, c.port); got != c.want {
			t.Errorf("%d/%s = %q, want %q", c.port, c.proto, got, c.want)
		}
	}

	if p, ok := parseUFW("Status: active\nDefault: allow (incoming), allow (outgoing)\n\nTo Action From\n-- ------ ----\n"); !ok || p.Verdict("tcp", 1) != Allowed {
		t.Error("default allow")
	}
	if p, ok := parseUFW("Status: inactive\n"); !ok || p.Verdict("tcp", 22) != Allowed {
		t.Error("inactive ufw")
	}
	if _, ok := parseUFW("ERROR: You need to be root to run this script\n"); ok {
		t.Error("accepted ufw's refusal as a status")
	}
}

const firewalldZone = `public (active)
  target: default
  icmp-block-inversion: no
  interfaces: eth0
  sources:
  services: dhcpv6-client ssh custom
  ports: 8080/tcp 60000-61000/udp
  protocols:
  forward: yes
  masquerade: no
  forward-ports:
  source-ports:
  icmp-blocks:
  rich rules:
`

func TestFirewalld(t *testing.T) {
	services := map[string][]portRange{"ssh": {{"tcp", 22, 22}}, "dhcpv6-client": {{"udp", 546, 546}}}
	p, ok := parseFirewalld(firewalldZone, func(name string) []portRange { return services[name] })
	if !ok {
		t.Fatal("zone not recognised")
	}
	for port, want := range map[uint16]Verdict{22: Allowed, 8080: Allowed, 5432: Blocked} {
		if got := p.Verdict("tcp", port); got != want {
			t.Errorf("%d/tcp = %q, want %q", port, got, want)
		}
	}
	if got := p.Verdict("udp", 60500); got != Allowed {
		t.Errorf("udp range = %q", got)
	}

	rich := firewalldZone + "\trule family=\"ipv4\" source address=\"10.0.0.0/8\" port port=\"5432\" protocol=\"tcp\" accept\n"
	if p, _ := parseFirewalld(rich, func(string) []portRange { return nil }); p.Verdict("tcp", 5432) != Unknown {
		t.Error("a port rich rules may open must stay unknown")
	}
	trusted := "trusted (active)\n  target: ACCEPT\n"
	if p, _ := parseFirewalld(trusted, nil); p.Verdict("tcp", 1) != Allowed {
		t.Error("target ACCEPT")
	}
}

func TestFirewalldServiceFile(t *testing.T) {
	dir := t.TempDir()
	xml := `<?xml version="1.0" encoding="utf-8"?>
<service><short>HTTP/HTTPS</short><port protocol="tcp" port="80"/><port protocol="tcp" port="443"/></service>`
	if err := os.WriteFile(filepath.Join(dir, "web.xml"), []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	old := FirewalldServiceDirs
	FirewalldServiceDirs = []string{dir}
	t.Cleanup(func() { FirewalldServiceDirs = old })
	ports := firewalldService("web")
	if len(ports) != 2 || !ports[1].covers("tcp", 443) {
		t.Fatalf("ports = %+v", ports)
	}
}

func TestOtherFirewalls(t *testing.T) {
	if nftPolicy("").Verdict("tcp", 22) != Allowed {
		t.Error("empty nftables ruleset")
	}
	if nftPolicy("table inet filter {\n\tchain input {\n\t\ttype filter hook input priority 0; policy drop;\n\t}\n}\n").Verdict("tcp", 22) != Unknown {
		t.Error("nftables input rules")
	}
	for out, want := range map[string]Verdict{
		"Firewall is disabled. (State = 0)":                            Allowed,
		"Firewall is enabled. (State = 1)":                             Unknown,
		"Firewall is blocking all non-essential services. (State = 2)": Blocked,
	} {
		if got := parseSocketFilterState(out).Verdict("tcp", 22); got != want {
			t.Errorf("%q = %q, want %q", out, got, want)
		}
	}
	for out, want := range map[string]Verdict{
		`[{"Name":"Domain","Enabled":0},{"Name":"Private","Enabled":0},{"Name":"Public","Enabled":0}]`: Allowed,
		`[{"Name":"Private","Enabled":1},{"Name":"Public","Enabled":0}]`:                               Unknown,
		`{"Name":"Public","Enabled":false}`:                                                            Allowed,
		`not json`:                                                                                     Unknown,
	} {
		if got := parseWindowsProfiles([]byte(out)).Verdict("tcp", 22); got != want {
			t.Errorf("%s = %q, want %q", out, got, want)
		}
	}
}
