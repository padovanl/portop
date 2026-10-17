//go:build !windows

package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	SocketPath = findSocket(os.Getenv, func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	})
}

// findSocket locates the Engine API socket the way the docker CLI would:
// DOCKER_HOST when it names a unix socket, then the system socket, then
// the per-user sockets of rootless Docker and Docker Desktop (which on
// recent macOS versions no longer creates /var/run/docker.sock), then
// Podman's Docker-compatible sockets.
func findSocket(getenv func(string) string, exists func(string) bool) string {
	if host := getenv("DOCKER_HOST"); strings.HasPrefix(host, "unix://") {
		return strings.TrimPrefix(host, "unix://")
	}
	const system = "/var/run/docker.sock"
	candidates := []string{system}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "docker.sock"))
	}
	if home := getenv("HOME"); home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".docker", "run", "docker.sock"),
			filepath.Join(home, ".docker", "desktop", "docker.sock"))
	}
	// Podman serves the same API, rootful and rootless.
	candidates = append(candidates, "/run/podman/podman.sock")
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "podman", "podman.sock"))
	}
	for _, path := range candidates {
		if exists(path) {
			return path
		}
	}
	return system
}

// Available reports whether the Docker socket is reachable at all.
func (c *Client) Available() bool {
	_, err := os.Stat(SocketPath)
	return err == nil
}

// PublishedPorts returns Docker's view of published host ports, or nothing
// when Docker is unavailable.
func (c *Client) PublishedPorts(ctx context.Context) []PublishedPort {
	return c.publishedPortsAPI(ctx)
}
