package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/padovan93/portop/internal/app"
	"github.com/padovan93/portop/internal/baseline"
	"github.com/padovan93/portop/internal/scanner"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "portop") {
		t.Errorf("version output = %q, want it to mention portop", out.String())
	}
}

func TestRunHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(errOut.String(), "Usage:") {
		t.Errorf("help output = %q, want usage text", errOut.String())
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--not-a-real-flag"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunJSONFindsRealListener(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	var out, errOut bytes.Buffer
	code := Run([]string{"--json", "--no-dns", "--no-systemd", "--no-docker"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}

	var rows []jsonRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}

	found := false
	for _, r := range rows {
		if int(r.LocalPort) == port && r.State == "LISTEN" {
			found = true
		}
	}
	if !found {
		t.Errorf("listener on port %d not found in --json output (%d rows)", port, len(rows))
	}
}

func TestRunJSONFilterByPort(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	var out, errOut bytes.Buffer
	code := Run([]string{strconv.Itoa(port), "--json", "--no-dns", "--no-systemd", "--no-docker"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}

	var rows []jsonRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	for _, r := range rows {
		if int(r.LocalPort) != port {
			t.Errorf("row for port %d present despite filtering on %d", r.LocalPort, port)
		}
	}
	if len(rows) == 0 {
		t.Error("expected at least the matching listener row, got none")
	}
}

func TestBaselineSaveThenDiff(t *testing.T) {
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")
	commonFlags := []string{"--baseline-path", baselinePath, "--no-dns", "--no-systemd", "--no-docker"}

	lnKept, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lnKept.Close()
	keptPort := lnKept.Addr().(*net.TCPAddr).Port

	lnRemoved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	removedPort := lnRemoved.Addr().(*net.TCPAddr).Port

	var out, errOut bytes.Buffer
	code := Run(append([]string{"--save-baseline"}, commonFlags...), &out, &errOut)
	if code != 0 {
		t.Fatalf("--save-baseline exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), baselinePath) {
		t.Errorf("--save-baseline output = %q, want it to mention the path", out.String())
	}

	// No drift yet: both listeners are still up, matching the baseline.
	out.Reset()
	errOut.Reset()
	code = Run(append([]string{"--diff"}, commonFlags...), &out, &errOut)
	if code != 0 {
		t.Fatalf("--diff (no drift) exit code = %d, want 0, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "no changes") {
		t.Errorf("--diff (no drift) output = %q, want it to report no changes", out.String())
	}

	// Introduce drift: close one listener, open a new one.
	lnRemoved.Close()
	lnNew, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lnNew.Close()
	newPort := lnNew.Addr().(*net.TCPAddr).Port

	out.Reset()
	errOut.Reset()
	code = Run(append([]string{"--diff"}, commonFlags...), &out, &errOut)
	if code != 3 {
		t.Fatalf("--diff (drift) exit code = %d, want 3, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), strconv.Itoa(newPort)) {
		t.Errorf("--diff output missing newly added port %d: %s", newPort, out.String())
	}
	if !strings.Contains(out.String(), strconv.Itoa(removedPort)) {
		t.Errorf("--diff output missing removed port %d: %s", removedPort, out.String())
	}
	if strings.Contains(out.String(), strconv.Itoa(keptPort)+" ") {
		t.Errorf("--diff output should not list the unchanged port %d: %s", keptPort, out.String())
	}

	// Same drift, machine-readable.
	out.Reset()
	errOut.Reset()
	code = Run(append([]string{"--diff", "--json"}, commonFlags...), &out, &errOut)
	if code != 3 {
		t.Fatalf("--diff --json exit code = %d, want 3", code)
	}
	var report struct {
		Added   []baseline.Entry `json:"added"`
		Removed []baseline.Entry `json:"removed"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(report.Added) != 1 || int(report.Added[0].Port) != newPort {
		t.Errorf("Added = %+v, want just port %d", report.Added, newPort)
	}
	if len(report.Removed) != 1 || int(report.Removed[0].Port) != removedPort {
		t.Errorf("Removed = %+v, want just port %d", report.Removed, removedPort)
	}
}

func TestDiffWithoutBaselineFails(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--diff", "--baseline-path", filepath.Join(t.TempDir(), "missing.json")}, &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "save-baseline") {
		t.Errorf("stderr = %q, want a hint to run --save-baseline", errOut.String())
	}
}

func TestMatchesFilter(t *testing.T) {
	row := app.Row{LocalPort: 8080, PID: 4242, ProcessName: "nginx", Protocol: scanner.TCP}
	cases := map[string]bool{
		"8080":  true,
		"nginx": true,
		"4242":  true,
		"NGINX": true,
		"9999":  false,
		"redis": false,
	}
	for query, want := range cases {
		if got := matchesFilter(row, query); got != want {
			t.Errorf("matchesFilter(%q) = %v, want %v", query, got, want)
		}
	}
}
