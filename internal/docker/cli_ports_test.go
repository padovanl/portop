package docker

import "testing"

func TestParsePSOutput(t *testing.T) {
	out := []byte(`{"ID":"abc123","Names":"web-1","Labels":"com.docker.compose.project=shop,com.docker.compose.service=web","Ports":"0.0.0.0:8080->80/tcp, :::8080->80/tcp, 443/tcp","State":"running"}
not json
{"ID":"def456","Names":"dns","Labels":"","Ports":"127.0.0.1:5300-5301->53-54/udp","State":"exited"}
`)
	got := parsePSOutput(out)
	if len(got) != 4 {
		t.Fatalf("got %d ports, want 4: %+v", len(got), got)
	}
	web := got[0]
	if web.ContainerID != "abc123" || web.ContainerName != "web-1" || web.Service != "web" || web.Project != "shop" ||
		web.HostIP != "0.0.0.0" || web.PublicPort != 8080 || web.PrivatePort != 80 || web.Type != "TCP" || web.State != "running" {
		t.Errorf("first mapping = %+v", web)
	}
	if got[1].HostIP != "::" || got[1].PublicPort != 8080 {
		t.Errorf("IPv6 mapping = %+v", got[1])
	}
	if got[2].PublicPort != 5300 || got[2].PrivatePort != 53 || got[3].PublicPort != 5301 || got[3].PrivatePort != 54 ||
		got[3].Type != "UDP" || got[3].HostIP != "127.0.0.1" {
		t.Errorf("range mappings = %+v %+v", got[2], got[3])
	}
}

func TestParsePortSpecRejectsMalformed(t *testing.T) {
	for _, spec := range []string{"", "80/tcp", "0.0.0.0:x->80/tcp", "0.0.0.0:1-3->1-2/tcp", "[::]:70000->1/tcp"} {
		if got := parsePortSpec(spec); got != nil {
			t.Errorf("parsePortSpec(%q) = %+v, want nil", spec, got)
		}
	}
}
