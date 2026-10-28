package cli

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/scanner"
)

func metricRows() []app.Row {
	return []app.Row{
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: 80, State: scanner.StateListen, PID: 10, ProcessName: `ng"inx`, Username: "www", Firewall: "blocked"},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 5432, State: scanner.StateListen, PID: 20, ProcessName: "postgres"},
		{Protocol: scanner.UDP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: 53, State: scanner.StateUnconn},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("10.0.0.2"), LocalPort: 51000, RemotePort: 443, State: scanner.StateEstablished, PID: 30},
	}
}

func TestWriteMetrics(t *testing.T) {
	var b bytes.Buffer
	writeMetrics(&b, metricRows(), 1500*time.Microsecond)
	out := b.String()
	for _, want := range []string{
		`portop_sockets{protocol="tcp",state="ESTABLISHED"} 1`,
		`portop_sockets{protocol="tcp",state="LISTEN"} 2`,
		`portop_listening_ports{protocol="tcp",scope="all"} 1`,
		`portop_listening_ports{protocol="tcp",scope="loopback"} 1`,
		`portop_listening_ports{protocol="udp",scope="all"} 1`,
		`portop_listening_ports{protocol="udp",scope="address"} 0`,
		`portop_listening_port_info{protocol="tcp",port="80",address="0.0.0.0",scope="all",firewall="blocked",process="ng\"inx",pid="10",user="www",container=""} 1`,
		`portop_listening_port_info{protocol="udp",port="53",address="0.0.0.0",scope="all",firewall="",process="",pid="",user="",container=""} 1`,
		"portop_scrape_duration_seconds 0.001500",
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `port="51000"`) {
		t.Error("connections must not get a per-port series")
	}
}

func metricsHandler(auth *webAuth) http.Handler {
	return webHandler(func(context.Context, app.Options) ([]app.Row, error) { return metricRows(), nil },
		webConfig{Auth: auth})
}

func scrape(h http.Handler, user, password string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/metrics", nil)
	if user != "" {
		req.SetBasicAuth(user, password)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMetricsEndpoint(t *testing.T) {
	rec := scrape(metricsHandler(nil), "", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("no auth: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	auth := newWebAuth("admin", "correct horse", "host")
	h := metricsHandler(auth)
	if rec := scrape(h, "", ""); rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("missing credentials: %d", rec.Code)
	}
	if rec := scrape(h, "admin", "correct horse"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "portop_sockets") {
		t.Fatalf("basic auth: %d", rec.Code)
	}
	for i := 0; i < maxLoginFailures; i++ {
		if rec := scrape(h, "admin", "guess"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: %d", i, rec.Code)
		}
	}
	if rec := scrape(h, "admin", "correct horse"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after %d failures: %d, want the lockout", maxLoginFailures, rec.Code)
	}
}

// The read-only API takes Basic auth too (portop desktop connects that
// way); actions do not.
func TestReadOnlyAPIBasicAuth(t *testing.T) {
	h := metricsHandler(newWebAuth("admin", "correct horse", "host"))
	get := func(method, path string, auth bool) int {
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, nil)
		if auth {
			req.SetBasicAuth("admin", "correct horse")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := get(http.MethodGet, "/api/ports", false); code != http.StatusUnauthorized {
		t.Errorf("ports without credentials: %d", code)
	}
	if code := get(http.MethodGet, "/api/ports", true); code != http.StatusOK {
		t.Errorf("ports with Basic auth: %d", code)
	}
	if code := get(http.MethodPost, "/api/process/1/signal", true); code != http.StatusUnauthorized {
		t.Errorf("an action with Basic auth: %d, want refused", code)
	}

	// Process details are read with Basic auth too (portop desktop shows
	// another machine's processes so); this test's own process is listed.
	self := os.Getpid()
	h = webHandler(func(context.Context, app.Options) ([]app.Row, error) {
		return []app.Row{{Protocol: scanner.TCP, LocalAddr: net.IPv4(127, 0, 0, 1), LocalPort: 8080, State: scanner.StateListen, PID: self}}, nil
	}, webConfig{Auth: newWebAuth("admin", "correct horse", "host")})
	path := fmt.Sprintf("/api/process/%d", self)
	if code := get(http.MethodGet, path, false); code != http.StatusUnauthorized {
		t.Errorf("details without credentials: %d", code)
	}
	if code := get(http.MethodGet, path, true); code != http.StatusOK {
		t.Errorf("details with Basic auth: %d, want 200", code)
	}
}
