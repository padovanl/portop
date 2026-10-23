package docker

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/hostcmd"
)

// On Windows, Docker Desktop's daemon listens on a named pipe and its
// containers have no host-visible cgroups, so container names cannot be
// derived from a PID. Instead the collector matches listening ports
// against the published ports reported here (see app.Collector). The
// docker CLI is used rather than dialing the pipe directly: it is always
// installed with Docker Desktop and honours the user's docker context.

const psCacheTTL = 3 * time.Second

var (
	cliOnce sync.Once
	cliPath string

	psMu      sync.Mutex
	psAt      time.Time
	psResults []PublishedPort
)

func dockerCLI() string {
	cliOnce.Do(func() { cliPath, _ = exec.LookPath("docker") })
	return cliPath
}

// Available reports whether the docker CLI is installed.
func (c *Client) Available() bool { return dockerCLI() != "" }

// PublishedPorts returns the host ports published by running and stopped
// containers. Results are cached briefly because the TUI refreshes every
// second and each docker ps invocation costs a process launch.
func (c *Client) PublishedPorts(ctx context.Context) []PublishedPort {
	cli := dockerCLI()
	if cli == "" {
		return nil
	}
	psMu.Lock()
	defer psMu.Unlock()
	if time.Since(psAt) < psCacheTTL {
		return psResults
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := hostcmd.Hide(exec.CommandContext(ctx, cli, "ps", "--all", "--no-trunc", "--format", "{{json .}}"))
	hideWindow(cmd)
	out, err := cmd.Output()
	psAt = time.Now()
	if err != nil {
		psResults = nil
		return nil
	}
	psResults = parsePSOutput(out)
	return psResults
}
