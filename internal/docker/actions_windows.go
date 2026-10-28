package docker

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/padovanl/portop/internal/hostcmd"
)

// Act stops, starts or restarts a container with the docker CLI, which
// talks to Docker Desktop's named pipe and honours the user's context.
func (c *Client) Act(ctx context.Context, name, action string) error {
	if err := checkAction(name, action); err != nil {
		return err
	}
	cli := dockerCLI()
	if cli == "" {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := hostcmd.Hide(exec.CommandContext(ctx, cli, action, name)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s %s: %s", action, name, strings.TrimSpace(string(out)))
	}
	return nil
}

// Logs returns the last lines of a container's output.
func (c *Client) Logs(ctx context.Context, name string, tail int) (string, error) {
	if err := checkAction(name, ActionStop); err != nil {
		return "", err
	}
	cli := dockerCLI()
	if cli == "" {
		return "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := hostcmd.Hide(exec.CommandContext(ctx, cli, "logs", "--tail", strconv.Itoa(tail), name)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker logs %s: %s", name, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
