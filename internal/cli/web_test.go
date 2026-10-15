package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/procinfo"
	"github.com/padovanl/portop/internal/scanner"
)

func TestWebHandler(t *testing.T) {
	rows := []app.Row{
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 8080, State: scanner.StateListen, PID: 42, ProcessName: "<script>"},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 8081, State: scanner.StateEstablished},
	}
	h := webHandler(func(context.Context, app.Options) ([]app.Row, error) { return rows, nil }, webConfig{Filter: "8080", ListenOnly: true, Token: "test-token"})
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/ports", nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response: %d, headers: %v", response.Code, response.Header())
	}
	var got []jsonRow
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil || len(got) != 1 || got[0].LocalPort != 8080 {
		t.Fatalf("rows: %+v, error: %v", got, err)
	}
	for _, path := range []string{"/", "/app.css", "/app.js", "/logo.png"} {
		asset := httptest.NewRecorder()
		h.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil))
		if asset.Code != http.StatusOK || asset.Body.Len() == 0 || strings.Contains(asset.Header().Get("Content-Security-Policy"), "unsafe-inline") {
			t.Errorf("asset %s: status %d, headers %v", path, asset.Code, asset.Header())
		}
	}
	request.Host = "attacker.example"
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unexpected host status: %d", response.Code)
	}
}

func TestWebAddress(t *testing.T) {
	tests := []struct {
		name         string
		address      string
		port         int
		addrSet      bool
		portSet      bool
		remote       bool
		want         string
		wantLoopback bool
		wantErr      bool
	}{
		{name: "default", address: "127.0.0.1:8088", port: 8088, want: "127.0.0.1:8088", wantLoopback: true},
		{name: "custom port", address: "127.0.0.1:8088", port: 9090, portSet: true, want: "127.0.0.1:9090", wantLoopback: true},
		{name: "IPv6 loopback", address: "[::1]:9090", addrSet: true, want: "[::1]:9090", wantLoopback: true},
		{name: "remote address", address: "0.0.0.0:8088", addrSet: true, wantErr: true},
		{name: "remote address with auth", address: "0.0.0.0:8088", addrSet: true, remote: true, want: "0.0.0.0:8088"},
		{name: "all interfaces with auth", address: ":8088", addrSet: true, remote: true, want: ":8088"},
		{name: "hostname with auth", address: "example.com:8088", addrSet: true, remote: true, wantErr: true},
		{name: "zero port", address: "127.0.0.1:8088", portSet: true, wantErr: true},
		{name: "large port", address: "127.0.0.1:8088", port: 65536, portSet: true, wantErr: true},
		{name: "conflicting flags", address: "127.0.0.1:9090", port: 9000, addrSet: true, portSet: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, loopback, err := webAddress(tt.address, tt.port, tt.addrSet, tt.portSet, tt.remote)
			if (err != nil) != tt.wantErr || got != tt.want || loopback != tt.wantLoopback {
				t.Fatalf("webAddress() = %q, %v, %v; want %q, %v, error=%v", got, loopback, err, tt.want, tt.wantLoopback, tt.wantErr)
			}
		})
	}
}

func TestWebServeOptions(t *testing.T) {
	base := webFlags{addr: "127.0.0.1:8088", port: 8088, user: "portop"}

	serve, err := webServeOptions(base)
	if err != nil || serve.Auth || serve.Address != "127.0.0.1:8088" {
		t.Fatalf("no auth: %+v, %v", serve, err)
	}

	f := base
	f.auth, f.authSet = true, true
	serve, err = webServeOptions(f)
	if err != nil || !serve.Auth || !serve.Generated || len(serve.Password) < 20 || serve.User != "portop" {
		t.Fatalf("generated password: %+v, %v", serve, err)
	}

	f.passwordEnv = "from-env"
	serve, err = webServeOptions(f)
	if err != nil || serve.Generated || serve.Password != "from-env" {
		t.Fatalf("environment password: %+v, %v", serve, err)
	}

	f = base
	f.user, f.userSet, f.password, f.passwordSet, f.passwordEnv = "admin", true, "secret", true, "ignored"
	f.addr, f.addrSet = "0.0.0.0:9000", true
	serve, err = webServeOptions(f)
	if err != nil || !serve.Auth || serve.User != "admin" || serve.Password != "secret" || serve.Loopback {
		t.Fatalf("explicit credentials: %+v, %v", serve, err)
	}

	for name, f := range map[string]webFlags{
		"credentials with auth off": {addr: base.addr, user: "admin", userSet: true, authSet: true},
		"empty password":            {addr: base.addr, user: "portop", auth: true, passwordSet: true},
		"empty user":                {addr: base.addr, user: " ", userSet: true, password: "x", passwordSet: true},
		"certificate without key":   {addr: base.addr, user: "portop", tlsCert: "cert.pem"},
		"remote without auth":       {addr: "0.0.0.0:8088", addrSet: true, user: "portop"},
	} {
		if _, err := webServeOptions(f); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestWebFlagsRejectInvalidAddress(t *testing.T) {
	for _, args := range [][]string{
		{"--web", "--web-addr", "0.0.0.0:8088"},
		{"--web", "--web-port", "0"},
		{"--web", "--web-port", "9090", "--web-addr", "127.0.0.1:9090"},
		{"--web", "--web-auth=false", "--web-password", "secret"},
		{"--web", "--web-auth", "--web-tls-key", "key.pem"},
	} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != 2 {
			t.Errorf("%v: exit %d, stderr %q", args, code, errOut.String())
		}
	}
}

func TestWebProcessActions(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start test process: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	pid := cmd.Process.Pid
	h := webHandler(func(context.Context, app.Options) ([]app.Row, error) {
		return []app.Row{{PID: pid}}, nil
	}, webConfig{Token: "test-token"})
	url := fmt.Sprintf("http://127.0.0.1/api/process/%d", pid)
	request := httptest.NewRequest(http.MethodGet, url, nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing token: %d", response.Code)
	}
	request.Header.Set("X-Portop-Token", "test-token")
	hidden := webHandler(func(context.Context, app.Options) ([]app.Row, error) {
		return nil, nil
	}, webConfig{Token: "test-token"})
	response = httptest.NewRecorder()
	hidden.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("process without a socket: %d", response.Code)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("inspect: %d: %s", response.Code, response.Body.String())
	}
	var info procinfo.Info
	if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil || info.PID != pid || info.StartTime.IsZero() {
		t.Fatalf("process details: %+v, %v", info, err)
	}
	signalURL := url + "/signal"
	signal := func(start string) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(map[string]any{"start_time": start, "force": false})
		req := httptest.NewRequest(http.MethodPost, signalURL, bytes.NewReader(payload))
		req.Header.Set("X-Portop-Token", "test-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if response := signal(info.StartTime.Add(-time.Second).Format(time.RFC3339Nano)); response.Code != http.StatusConflict {
		t.Fatalf("stale process signal: %d: %s", response.Code, response.Body.String())
	}
	if response := signal(info.StartTime.Format(time.RFC3339Nano)); response.Code != http.StatusNoContent {
		t.Fatalf("terminate: %d: %s", response.Code, response.Body.String())
	}
}
