package docker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"
)

func TestContainerIDFromCgroupContentV1(t *testing.T) {
	content := "12:pids:/docker/8fa60d61e626c774e5cceff343cd3c3b2642ecd91b9efad77849abfeff0d78aa\n"
	want := "8fa60d61e626c774e5cceff343cd3c3b2642ecd91b9efad77849abfeff0d78aa"
	if got := containerIDFromCgroupContent(content); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestContainerIDFromCgroupContentSystemdScope(t *testing.T) {
	content := "0::/system.slice/docker-8fa60d61e626c774e5cceff343cd3c3b2642ecd91b9efad77849abfeff0d78aa.scope\n"
	want := "8fa60d61e626c774e5cceff343cd3c3b2642ecd91b9efad77849abfeff0d78aa"
	if got := containerIDFromCgroupContent(content); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestContainerIDFromCgroupContentNoContainer(t *testing.T) {
	content := "0::/user.slice/user-1000.slice\n"
	if got := containerIDFromCgroupContent(content); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestContainerIDForPIDMissingProc(t *testing.T) {
	ProcRoot = t.TempDir()
	if got := ContainerIDForPID(999999); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// TestClientContainerName spins up a fake Docker daemon on a real unix
// socket to exercise the HTTP-over-unix-socket client end to end.
func TestClientContainerName(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "docker.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/containers/abc123/json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"Name": "/my-app"})
	})
	mux.HandleFunc("/containers/missing/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()

	origSock := SocketPath
	SocketPath = sockPath
	defer func() { SocketPath = origSock }()

	c := NewClient()
	if !c.Available() {
		t.Fatal("Available() = false, want true (socket file exists)")
	}

	name := c.ContainerName(context.Background(), "abc123")
	if name != "my-app" {
		t.Errorf("ContainerName = %q, want my-app", name)
	}

	// Cached second call should still work and return the same value.
	if name := c.ContainerName(context.Background(), "abc123"); name != "my-app" {
		t.Errorf("cached ContainerName = %q, want my-app", name)
	}

	if name := c.ContainerName(context.Background(), "missing"); name != "" {
		t.Errorf("ContainerName(missing) = %q, want empty", name)
	}

	if name := c.ContainerName(context.Background(), ""); name != "" {
		t.Errorf("ContainerName(\"\") = %q, want empty", name)
	}
}

func TestClientPublishedPorts(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "docker.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("all") != "1" {
			t.Errorf("all query = %q, want 1", r.URL.Query().Get("all"))
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"Id":    "abc123",
				"Names": []string{"/web_1"},
				"Labels": map[string]string{
					"com.docker.compose.project": "demo",
					"com.docker.compose.service": "web",
				},
				"State": "running",
				"Ports": []map[string]any{
					{"IP": "0.0.0.0", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"},
					{"IP": "127.0.0.1", "PrivatePort": 53, "PublicPort": 5353, "Type": "udp"},
					{"PrivatePort": 443, "Type": "tcp"},
				},
			},
		})
	})

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()

	old := SocketPath
	SocketPath = sockPath
	defer func() { SocketPath = old }()

	c := NewClient()
	ports := c.PublishedPorts(context.Background())
	if len(ports) != 2 {
		t.Fatalf("PublishedPorts returned %d entries, want 2: %+v", len(ports), ports)
	}
	if ports[0].ContainerName != "web_1" || ports[0].Service != "web" || ports[0].Project != "demo" {
		t.Errorf("first port metadata = %+v", ports[0])
	}
	if ports[0].PublicPort != 8080 || ports[0].PrivatePort != 80 || ports[0].Type != "TCP" {
		t.Errorf("first port = %+v, want 8080:80/TCP", ports[0])
	}
	if ports[1].HostIP != "127.0.0.1" || ports[1].Type != "UDP" {
		t.Errorf("second port = %+v, want localhost UDP mapping", ports[1])
	}
}

func TestClientAvailableFalseWhenNoSocket(t *testing.T) {
	origSock := SocketPath
	SocketPath = filepath.Join(t.TempDir(), "no-such.sock")
	defer func() { SocketPath = origSock }()

	c := NewClient()
	if c.Available() {
		t.Error("Available() = true, want false when socket file absent")
	}
}
