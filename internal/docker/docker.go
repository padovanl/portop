// Package docker associates a process with the Docker container that
// owns it (via the process's cgroup) and looks up that container's name
// through the Docker Engine API over its unix socket. No official SDK is
// used: containers.json-ish info is fetched with a tiny hand-rolled
// HTTP-over-unix-socket client to avoid pulling in a heavy dependency
// for what is a handful of read-only calls.
package docker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProcRoot is the root of the /proc filesystem, overridable in tests.
var ProcRoot = "/proc"

// SocketPath is the Docker Engine API unix socket, overridable in tests
// (e.g. to point at a fake HTTP-over-unix-socket server).
var SocketPath = "/var/run/docker.sock"

// containerIDPattern matches a 64-hex-char (or the shorter, still-valid
// 12+ char) container ID appearing anywhere in a cgroup path segment,
// which covers both cgroup v1 (".../docker/<id>") and cgroup v2 with
// systemd-managed containers (".../docker-<id>.scope").
var containerIDPattern = regexp.MustCompile(`[0-9a-f]{64}`)

// ContainerIDForPID returns the full container ID owning pid, or "" if
// the process is not running inside a Docker container (or that could
// not be determined from its cgroup membership).
func ContainerIDForPID(pid int) string {
	data, err := os.ReadFile(ProcRoot + "/" + strconv.Itoa(pid) + "/cgroup")
	if err != nil {
		return ""
	}
	return containerIDFromCgroupContent(string(data))
}

func containerIDFromCgroupContent(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if m := containerIDPattern.FindString(line); m != "" {
			return m
		}
	}
	return ""
}

// Client is a minimal read-only Docker Engine API client, talking to the
// daemon over its unix socket.
type Client struct {
	httpClient *http.Client

	mu        sync.Mutex
	nameCache map[string]string // container ID -> friendly name
}

// NewClient returns a client for the Docker daemon at SocketPath. It does
// not verify the daemon is reachable; that is discovered lazily on first
// use so the TUI can start fine on machines without Docker installed.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					d := net.Dialer{}
					return d.DialContext(ctx, "unix", SocketPath)
				},
			},
		},
		nameCache: make(map[string]string),
	}
}

// Available reports whether the Docker socket is reachable at all.
func (c *Client) Available() bool {
	_, err := os.Stat(SocketPath)
	return err == nil
}

type containerInspect struct {
	Name string `json:"Name"`
}

// ContainerName returns the human-readable name (without the leading
// slash Docker prepends) for a container ID, caching results since the
// same container backs many connections. Returns "" if the daemon is
// unreachable, the container is unknown, or the request errors/times out.
func (c *Client) ContainerName(ctx context.Context, containerID string) string {
	if containerID == "" {
		return ""
	}

	c.mu.Lock()
	if name, ok := c.nameCache[containerID]; ok {
		c.mu.Unlock()
		return name
	}
	c.mu.Unlock()

	url := "http://unix/containers/" + containerID + "/json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var info containerInspect
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return ""
	}
	name := strings.TrimPrefix(info.Name, "/")

	c.mu.Lock()
	c.nameCache[containerID] = name
	c.mu.Unlock()
	return name
}
