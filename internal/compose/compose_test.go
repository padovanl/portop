package compose

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/docker"
	"github.com/padovanl/portop/internal/scanner"
)

func TestLoadDirParsesComposePorts(t *testing.T) {
	dir := t.TempDir()
	content := `
services:
  web:
    container_name: demo-web
    ports:
      - "8080:80"
      - "127.0.0.1:8443:443/tcp"
      - "5300-5301:53-54/udp"
      - "9000"
  api:
    ports:
      - target: 8080
        published: "18080"
        host_ip: "127.0.0.1"
        protocol: tcp
`
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(summary.Files) != 1 || summary.Files[0] != "compose.yml" {
		t.Fatalf("Files = %+v, want compose.yml", summary.Files)
	}
	if len(summary.Results) != 6 {
		t.Fatalf("Results len = %d, want 6: %+v", len(summary.Results), summary.Results)
	}

	var haveDynamic bool
	for _, r := range summary.Results {
		if r.Service == "web" && r.Dynamic && r.Target == 9000 {
			haveDynamic = true
		}
	}
	if !haveDynamic {
		t.Errorf("dynamic single-port mapping not found: %+v", summary.Results)
	}
}

func TestAuditStatuses(t *testing.T) {
	configured := []Result{
		{ConfiguredPort: ConfiguredPort{Service: "web", Published: 8080, Target: 80, Protocol: "TCP"}},
		{ConfiguredPort: ConfiguredPort{Service: "db", Published: 5432, Target: 5432, Protocol: "TCP"}},
		{ConfiguredPort: ConfiguredPort{Service: "metrics", Published: 9090, Target: 9090, Protocol: "TCP"}},
		{ConfiguredPort: ConfiguredPort{Service: "dyn", Target: 8080, Protocol: "TCP", Dynamic: true}, Status: StatusDynamic},
	}
	rows := []app.Row{
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 5432, State: scanner.StateListen, PID: 123, ProcessName: "postgres"},
	}
	published := []docker.PublishedPort{
		{Service: "web", ContainerName: "demo-web-1", HostIP: "0.0.0.0", PublicPort: 8080, PrivatePort: 80, Type: "TCP", State: "running"},
	}

	results := Audit(configured, rows, published)
	if results[0].Status != StatusExpected || results[0].OwnerSource != "docker" {
		t.Errorf("web result = %+v, want expected from docker", results[0])
	}
	if results[1].Status != StatusConflict || results[1].OwnerProcess != "postgres" {
		t.Errorf("db result = %+v, want conflict from socket", results[1])
	}
	if results[2].Status != StatusUnused {
		t.Errorf("metrics result = %+v, want unused", results[2])
	}
	if results[3].Status != StatusDynamic {
		t.Errorf("dynamic result = %+v, want dynamic", results[3])
	}
}

func TestLoadDirMissingComposeFile(t *testing.T) {
	_, err := LoadDir(t.TempDir())
	if err == nil {
		t.Fatal("LoadDir unexpectedly succeeded without a Compose file")
	}
}
