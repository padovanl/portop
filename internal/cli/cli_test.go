package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/padovan93/portop/internal/app"
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
