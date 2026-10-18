//go:build !windows

package docker

import "testing"

func TestFindSocket(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	only := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, want := range paths {
				if p == want {
					return true
				}
			}
			return false
		}
	}
	cases := []struct {
		name   string
		vars   map[string]string
		exists func(string) bool
		want   string
	}{
		{"DOCKER_HOST wins", map[string]string{"DOCKER_HOST": "unix:///custom/docker.sock"}, only("/var/run/docker.sock"), "/custom/docker.sock"},
		{"tcp DOCKER_HOST is not a socket", map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2376"}, only("/var/run/docker.sock"), "/var/run/docker.sock"},
		{"system socket", map[string]string{"HOME": "/Users/a"}, only("/var/run/docker.sock", "/Users/a/.docker/run/docker.sock"), "/var/run/docker.sock"},
		{"rootless Docker", map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, only("/run/user/1000/docker.sock"), "/run/user/1000/docker.sock"},
		{"Docker Desktop on macOS", map[string]string{"HOME": "/Users/a"}, only("/Users/a/.docker/run/docker.sock"), "/Users/a/.docker/run/docker.sock"},
		{"rootful Podman", map[string]string{}, only("/run/podman/podman.sock"), "/run/podman/podman.sock"},
		{"rootless Podman", map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, only("/run/user/1000/podman/podman.sock"), "/run/user/1000/podman/podman.sock"},
		{"Docker before Podman", map[string]string{}, only("/var/run/docker.sock", "/run/podman/podman.sock"), "/var/run/docker.sock"},
		{"nothing found", map[string]string{"HOME": "/Users/a"}, only(), "/var/run/docker.sock"},
	}
	for _, c := range cases {
		if got := findSocket(env(c.vars), c.exists); got != c.want {
			t.Errorf("%s: findSocket = %q, want %q", c.name, got, c.want)
		}
	}
}
