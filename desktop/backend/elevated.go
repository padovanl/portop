package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/cli"
	"github.com/padovanl/portop/internal/procinfo"
)

// The window never runs as administrator (a web view should not). To see
// and stop every user's processes it starts portop's elevated helper,
// this executable run again with --elevated-helper through pkexec, the
// macOS password dialog or UAC, and reads the scan from it: the same
// API as the web dashboard, on a loopback port, behind a token the
// helper writes to a private directory. The helper exits with the
// window.

type helperConn struct {
	cli.HelperInfo
	dir    string
	client *http.Client
	fails  int // scans in a row the helper could not answer; guarded by elevation.mu
}

// helperFailLimit is how many scans in a row may fail (a slow scan, a
// busy machine) before the window gives up on the helper.
const helperFailLimit = 3

type elevation struct {
	mu       sync.Mutex
	conn     *helperConn
	starting bool
}

// helperWait bounds how long Elevate waits for the password to be typed.
const helperWait = 3 * time.Minute

func (b *Backend) helper() *helperConn {
	b.elev.mu.Lock()
	defer b.elev.mu.Unlock()
	return b.elev.conn
}

// Elevate asks for administrator rights once and, from then on, shows
// every user's processes and stops them without asking again.
func (b *Backend) Elevate() error {
	if !elevateAvailable() {
		return errors.New("administrator rights cannot be asked for on this system")
	}
	b.elev.mu.Lock()
	if b.elev.conn != nil {
		b.elev.mu.Unlock()
		return nil
	}
	if b.elev.starting {
		b.elev.mu.Unlock()
		return errors.New("already waiting for the administrator password")
	}
	b.elev.starting = true
	b.elev.mu.Unlock()
	defer func() {
		b.elev.mu.Lock()
		b.elev.starting = false
		b.elev.mu.Unlock()
	}()

	dir, err := os.MkdirTemp("", "portop-helper-") // 0700: only this user (and the administrator)
	if err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- runElevated([]string{"--elevated-helper", dir, strconv.Itoa(os.Getpid())}) }()

	deadline := time.After(helperWait)
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-exited:
			os.RemoveAll(dir)
			if err == nil {
				err = errors.New("the administrator helper stopped before it started")
			}
			return err
		case <-deadline:
			os.RemoveAll(dir)
			return errors.New("no administrator password was given in time")
		case <-tick.C:
		}
		raw, err := os.ReadFile(filepath.Join(dir, cli.HelperFile))
		if err != nil {
			continue
		}
		var info cli.HelperInfo
		if json.Unmarshal(raw, &info) != nil || info.Token == "" {
			continue
		}
		conn := &helperConn{HelperInfo: info, dir: dir, client: &http.Client{Timeout: 25 * time.Second}}
		b.elev.mu.Lock()
		b.elev.conn = conn
		b.elev.mu.Unlock()
		go func() {
			<-exited
			b.dropHelper(conn)
		}()
		b.Refresh()
		return nil
	}
}

// DropElevation stops the helper: the window goes back to what this user
// may see.
func (b *Backend) DropElevation() {
	if c := b.helper(); c != nil {
		b.dropHelper(c)
	}
}

// dropHelper forgets the helper and asks it to quit, so it never outlives
// the window's interest in it, even when it is dropped for not answering.
func (b *Backend) dropHelper(c *helperConn) {
	b.elev.mu.Lock()
	if b.elev.conn != c {
		b.elev.mu.Unlock()
		return
	}
	b.elev.conn = nil
	b.elev.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = c.do(ctx, http.MethodPost, "/quit", nil, nil)
	cancel()
	os.RemoveAll(c.dir)
	b.Refresh()
}

// helperRows scans through the helper. A failure falls back to this
// user's scan for that round; only several in a row drop the helper.
func (b *Backend) helperRows(ctx context.Context, c *helperConn) ([]app.Row, bool) {
	rows, err := c.rows(ctx)
	b.elev.mu.Lock()
	if err == nil {
		c.fails = 0
	} else {
		c.fails++
	}
	give := err != nil && c.fails >= helperFailLimit
	b.elev.mu.Unlock()
	if give {
		b.dropHelper(c)
	}
	return rows, err == nil
}

func (c *helperConn) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("X-Portop-Token", c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s", bytes.TrimSpace(msg))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
}

func (c *helperConn) rows(ctx context.Context) ([]app.Row, error) {
	var rows []remoteRow
	if err := c.do(ctx, http.MethodGet, "/api/ports", nil, &rows); err != nil {
		return nil, err
	}
	out := make([]app.Row, len(rows))
	for i, r := range rows {
		out[i] = r.toApp()
	}
	return out, nil
}

func (c *helperConn) process(pid int) (procinfo.Info, error) {
	var info procinfo.Info
	err := c.do(context.Background(), http.MethodGet, fmt.Sprintf("/api/process/%d", pid), nil, &info)
	return info, err
}

func (c *helperConn) signal(pid int, started string, force bool) error {
	body := map[string]any{"start_time": started, "force": force}
	return c.do(context.Background(), http.MethodPost, fmt.Sprintf("/api/process/%d/signal", pid), body, nil)
}
