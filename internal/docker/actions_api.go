//go:build !windows

package docker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Act stops, starts or restarts a container through the Engine API.
func (c *Client) Act(ctx context.Context, name, action string) error {
	if err := checkAction(name, action); err != nil {
		return err
	}
	if SocketPath == "" {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/containers/"+name+"/"+action+"?t=10", nil)
	if err != nil {
		return err
	}
	client := *c.httpClient
	client.Timeout = 0 // stopping waits up to 10 s; the context bounds it
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusNotModified:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("no container named %q", name)
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("docker %s %s: %s", action, name, http.StatusText(resp.StatusCode)+" "+string(msg))
}

// Logs returns the last lines of a container's output.
func (c *Client) Logs(ctx context.Context, name string, tail int) (string, error) {
	if err := checkAction(name, ActionStop); err != nil {
		return "", err
	}
	if SocketPath == "" {
		return "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://unix/containers/%s/logs?stdout=1&stderr=1&timestamps=0&tail=%d", name, tail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := *c.httpClient
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("docker logs %s: %s", name, http.StatusText(resp.StatusCode))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return string(demuxLogs(b)), nil
}
