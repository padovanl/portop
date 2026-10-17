//go:build e2e

// Package e2e black-box tests the real, compiled portop binary: it
// builds the binary once, opens real listening sockets, execs portop
// against the live system, and asserts on what actually comes back.
// This is what CI runs as the required "e2e" check.
package e2e

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type row struct {
	Protocol  string `json:"protocol"`
	LocalPort int    `json:"local_port"`
	State     string `json:"state"`
	PID       int    `json:"pid"`
	Process   string `json:"process"`
}

func buildBinary(t *testing.T) string {
	t.Helper()
	name := "portop"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/portop")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/portop failed: %v\n%s", err, out)
	}
	return bin
}

func TestJSONSnapshotFindsRealListener(t *testing.T) {
	bin := buildBinary(t)

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	out, err := exec.Command(bin, "--json", "--no-dns", "--no-systemd", "--no-docker").Output()
	if err != nil {
		t.Fatalf("portop --json failed: %v", err)
	}

	var rows []row
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("invalid JSON from portop --json: %v\n%s", err, out)
	}

	found := false
	for _, r := range rows {
		if r.LocalPort == port && r.State == "LISTEN" && r.Protocol == "TCP" {
			found = true
		}
	}
	if !found {
		t.Fatalf("listener on 127.0.0.1:%d not found among %d rows returned by the real binary", port, len(rows))
	}
}

func TestListenFlagExcludesEstablished(t *testing.T) {
	bin := buildBinary(t)

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()

	conn, err := net.DialTimeout("tcp4", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	select {
	case c := <-accepted:
		defer c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("connection was not accepted in time")
	}

	out, err := exec.Command(bin, "--json", "--listen", "--no-dns", "--no-systemd", "--no-docker").Output()
	if err != nil {
		t.Fatalf("portop --json --listen failed: %v", err)
	}

	var rows []row
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, r := range rows {
		if r.State != "LISTEN" && !(r.Protocol == "UDP" && r.State == "UNCONN") {
			t.Errorf("--listen leaked a non-listening row: %+v", r)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	bin := buildBinary(t)
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatalf("portop --version failed: %v", err)
	}
	if !strings.Contains(string(out), "portop") {
		t.Errorf("--version output = %q, want it to mention portop", out)
	}
}

// startWeb launches the dashboard and returns its base URL once the first
// line of output (the URL) has been printed.
func startWeb(t *testing.T, bin string, args ...string) (string, []string) {
	t.Helper()
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	cmd := exec.Command(bin, append([]string{"--web", "--web-port", strconv.Itoa(port), "--no-dns", "--no-systemd", "--no-docker"}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	lines := make(chan string, 8)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	var printed []string
	select {
	case line := <-lines:
		printed = append(printed, line)
	case <-time.After(10 * time.Second):
		t.Fatal("portop --web printed nothing")
	}
	// The remaining lines (user, generated password) follow immediately.
collect:
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				break collect
			}
			printed = append(printed, line)
		case <-time.After(300 * time.Millisecond):
			break collect
		}
	}
	scheme := "http"
	if strings.Contains(printed[0], "https://") {
		scheme = "https"
	}
	return fmt.Sprintf("%s://127.0.0.1:%d", scheme, port), printed
}

func TestWebAuthSignInWithRealBinary(t *testing.T) {
	bin := buildBinary(t)
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	listenPort := ln.Addr().(*net.TCPAddr).Port

	base, printed := startWeb(t, bin, "--web-user", "e2e", "--web-password", "e2e-secret")
	if strings.Contains(strings.Join(printed, "\n"), "e2e-secret") {
		t.Fatalf("a password passed on the command line must not be echoed: %q", printed)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	resp, err := client.Get(base + "/api/ports")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ports without signing in: %d", resp.StatusCode)
	}

	resp, err = client.PostForm(base+"/login", url.Values{"username": {"e2e"}, "password": {"wrong"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}

	resp, err = client.PostForm(base+"/login", url.Values{"username": {"e2e"}, "password": {"e2e-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/" {
		t.Fatalf("sign in should land on the dashboard: %d %s", resp.StatusCode, resp.Request.URL)
	}

	resp, err = client.Get(base + "/api/ports")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var rows []row
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatalf("ports after signing in: %d, %v", resp.StatusCode, err)
	}
	for _, r := range rows {
		if r.LocalPort == listenPort && r.State == "LISTEN" {
			return
		}
	}
	t.Fatalf("listener on :%d not in the signed-in dashboard's %d rows", listenPort, len(rows))
}

func TestWebAuthGeneratedPasswordOverTLS(t *testing.T) {
	bin := buildBinary(t)
	certFile, keyFile := selfSignedCert(t)

	base, printed := startWeb(t, bin, "--web-auth", "--web-tls-cert", certFile, "--web-tls-key", keyFile)
	if !strings.HasPrefix(base, "https://") {
		t.Fatalf("expected an https URL, got %q", printed)
	}
	var password string
	for _, line := range printed {
		if rest, ok := strings.CutPrefix(line, "portop web: password: "); ok {
			password, _, _ = strings.Cut(rest, " ")
		}
	}
	if password == "" {
		t.Fatalf("no generated password printed: %q", printed)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // self-signed test certificate
	}}
	resp, err := client.PostForm(base+"/login", url.Values{"username": {"portop"}, "password": {password}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/" {
		t.Fatalf("sign in over TLS: %d %s", resp.StatusCode, resp.Request.URL)
	}
	for _, c := range jar.Cookies(resp.Request.URL) {
		if c.Name == "portop_session" {
			return
		}
	}
	t.Fatal("no session cookie after signing in over TLS")
}

func selfSignedCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
