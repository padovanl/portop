package backend

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/events"
	"github.com/padovanl/portop/internal/project"
	"github.com/padovanl/portop/internal/scanner"
)

// Remote hosts are other machines running `portop --web`. The window
// shows them read-only: it reads GET /api/ports and GET /api/process/{pid}
// with the dashboard's sign-in as HTTP Basic auth. Passwords stay in
// memory for the session and are never written to the settings file.

func validHostURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

// UntrustedCertError carries the fingerprint of a certificate no system
// authority vouches for, so the user can decide to trust it.
type UntrustedCertError struct{ Fingerprint string }

func (e *UntrustedCertError) Error() string {
	return "the server's certificate is not trusted by this system (SHA-256 " + e.Fingerprint + ")"
}

type remoteState struct {
	mu        sync.Mutex
	passwords map[string]string
}

func fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// remoteClient checks the server's certificate against the system's
// authorities, or, when the host has a pinned fingerprint, against that.
func remoteClient(h Host) *http.Client {
	tlsConf := &tls.Config{MinVersion: tls.VersionTLS12}
	if h.Fingerprint != "" {
		pinned := h.Fingerprint
		tlsConf.InsecureSkipVerify = true // replaced by the pin check below
		tlsConf.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 || fingerprint(cs.PeerCertificates[0]) != pinned {
				return errors.New("the server's certificate changed since you trusted it")
			}
			return nil
		}
	}
	return &http.Client{
		Timeout:   8 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsConf, Proxy: http.ProxyFromEnvironment},
		// Credentials must not follow a redirect to another host.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (b *Backend) host(name string) (Host, string, error) {
	for _, h := range b.settings.get().Hosts {
		if h.Name == name {
			b.remote.mu.Lock()
			pw := b.remote.passwords[name]
			b.remote.mu.Unlock()
			return h, pw, nil
		}
	}
	return Host{}, "", fmt.Errorf("no host named %q", name)
}

func (b *Backend) remoteGet(ctx context.Context, h Host, password, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL+path, nil)
	if err != nil {
		return err
	}
	if h.User != "" {
		req.SetBasicAuth(h.User, password)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := remoteClient(h).Do(req)
	if err != nil {
		var unknown x509.UnknownAuthorityError
		var invalid x509.CertificateInvalidError
		var hostname x509.HostnameError
		if errors.As(err, &unknown) && unknown.Cert != nil {
			return &UntrustedCertError{Fingerprint: fingerprint(unknown.Cert)}
		}
		if errors.As(err, &invalid) && invalid.Cert != nil {
			return &UntrustedCertError{Fingerprint: fingerprint(invalid.Cert)}
		}
		if errors.As(err, &hostname) && hostname.Certificate != nil {
			return &UntrustedCertError{Fingerprint: fingerprint(hostname.Certificate)}
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return fmt.Errorf("%s did not answer in time", h.URL)
		}
		return fmt.Errorf("cannot reach %s: %v", h.URL, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return errors.New("wrong user name or password")
	case http.StatusTooManyRequests:
		return errors.New("too many failed sign-ins; the dashboard is locked for a while")
	case http.StatusForbidden:
		return errors.New("the dashboard refused the request: start it with --web-auth to reach it from other machines")
	default:
		return fmt.Errorf("%s answered %s", h.URL, resp.Status)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("%s is not a portop dashboard", h.URL)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
}

// remoteRow is portop's JSON row (portop --json and the web API).
type remoteRow struct {
	Protocol      string   `json:"protocol"`
	LocalAddress  string   `json:"local_address"`
	LocalPort     uint16   `json:"local_port"`
	RemoteAddress string   `json:"remote_address"`
	RemotePort    uint16   `json:"remote_port"`
	RemoteHost    string   `json:"remote_host"`
	State         string   `json:"state"`
	IPv6          bool     `json:"ipv6"`
	PID           int      `json:"pid"`
	Process       string   `json:"process"`
	User          string   `json:"user"`
	Workers       []int    `json:"workers"`
	Firewall      string   `json:"firewall"`
	RXBytes       *uint64  `json:"rx_bytes"`
	TXBytes       *uint64  `json:"tx_bytes"`
	RXRate        *float64 `json:"rx_bytes_per_second"`
	TXRate        *float64 `json:"tx_bytes_per_second"`
	CPU           float64  `json:"cpu_percent"`
	Unit          string   `json:"systemd_unit"`
	Container     string   `json:"container"`
	Project       string   `json:"project"`
	ProjectRoot   string   `json:"project_root"`
	Language      string   `json:"language"`
	Framework     string   `json:"framework"`
	Category      string   `json:"category"`
}

func (r remoteRow) toApp() app.Row {
	row := app.Row{
		Protocol: scanner.Protocol(r.Protocol), LocalAddr: net.ParseIP(r.LocalAddress), LocalPort: r.LocalPort,
		RemoteAddr: net.ParseIP(r.RemoteAddress), RemotePort: r.RemotePort, RemoteHost: r.RemoteHost,
		State: scanner.State(r.State), IPv6: r.IPv6, PID: r.PID, ProcessName: r.Process, Username: r.User,
		Workers: r.Workers, Firewall: r.Firewall, CPUPercent: r.CPU, SystemdUnit: r.Unit, ContainerName: r.Container,
		Project:  project.Info{Name: r.Project, Root: r.ProjectRoot, Language: r.Language, Framework: r.Framework},
		Category: r.Category,
	}
	if row.RemoteAddr == nil {
		row.RemoteAddr = net.IPv4zero
	}
	if r.RXBytes != nil && r.TXBytes != nil {
		row.RXBytes, row.TXBytes, row.TrafficKnown = *r.RXBytes, *r.TXBytes, true
	}
	if r.RXRate != nil && r.TXRate != nil {
		row.RXRate, row.TXRate, row.RateKnown = *r.RXRate, *r.TXRate, true
	}
	return row
}

// Connect checks that a host answers with these credentials and keeps
// the password for this session.
func (b *Backend) Connect(name, password string) error {
	h, _, err := b.host(name)
	if err != nil {
		return err
	}
	var rows []remoteRow
	if err := b.remoteGet(context.Background(), h, password, "/api/ports", &rows); err != nil {
		return err
	}
	b.remote.mu.Lock()
	b.remote.passwords[name] = password
	b.remote.mu.Unlock()
	return nil
}

// Disconnect forgets a host's password.
func (b *Backend) Disconnect(name string) {
	b.remote.mu.Lock()
	delete(b.remote.passwords, name)
	b.remote.mu.Unlock()
}

// RemoteSnapshot reads a host's sockets, as Snapshot does for this
// machine (without history or events).
func (b *Backend) RemoteSnapshot(name string) (Snapshot, error) {
	h, pw, err := b.host(name)
	if err != nil {
		return Snapshot{}, err
	}
	var rows []remoteRow
	if err := b.remoteGet(context.Background(), h, pw, "/api/ports", &rows); err != nil {
		return Snapshot{}, err
	}
	appRows := make([]app.Row, len(rows))
	for i, r := range rows {
		appRows[i] = r.toApp()
	}
	snap := Snapshot{Time: time.Now(), Events: []events.Event{}, Process: map[int][]TrafficPoint{}, Traffic: []TrafficPoint{}}
	snap.Rows, snap.Summary, snap.Hidden = convert(appRows)
	return snap, nil
}

// RemoteProcess reads one process's details from a host.
func (b *Backend) RemoteProcess(name string, pid int) (ProcessInfo, error) {
	h, pw, err := b.host(name)
	if err != nil {
		return ProcessInfo{}, err
	}
	var info ProcessInfo
	if err := b.remoteGet(context.Background(), h, pw, fmt.Sprintf("/api/process/%d", pid), &info); err != nil {
		return ProcessInfo{}, err
	}
	if !info.StartTime.IsZero() {
		info.Started = info.StartTime.Format(time.RFC3339Nano)
	}
	return info, nil
}
