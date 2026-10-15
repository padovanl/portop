//go:build !windows

package docker

import (
	"context"
	"os"
)

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
