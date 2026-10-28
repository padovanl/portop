package backend

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/procinfo"
	"github.com/padovanl/portop/internal/project"
	"github.com/padovanl/portop/internal/scanner"
)

func TestAtRisk(t *testing.T) {
	cases := []struct {
		scope, category, firewall string
		want                      bool
	}{
		{app.ScopeAll, "database", "allowed", true},
		{app.ScopeAll, "database", "", true}, // firewall unknown
		{app.ScopeAddress, "dev", "restricted", true},
		{app.ScopeAll, "database", "blocked", false},
		{app.ScopeLoopback, "database", "allowed", false},
		{app.ScopeAll, "web", "allowed", false},
		{app.ScopeAll, "app", "allowed", false},
	}
	for _, c := range cases {
		if got := atRisk(c.scope, c.category, c.firewall); got != c.want {
			t.Errorf("atRisk(%s, %s, %s) = %v", c.scope, c.category, c.firewall, got)
		}
	}
}

func TestValidShortcutAndHostURL(t *testing.T) {
	for s, want := range map[string]bool{"CmdOrCtrl+Alt+P": true, "Ctrl+Shift+F12": true, "": true, "P": false, "Ctrl+": false, "Hyper+P": false, "Ctrl+Alt+Delete": false} {
		if got := validShortcut(s); got != want {
			t.Errorf("validShortcut(%q) = %v", s, got)
		}
	}
	for u, want := range map[string]bool{"https://web-01:8088": true, "http://10.0.0.5:8088": true, "ftp://x": false, "https://u:p@host": false, "host:8088": false, "https://h/?x=1": false} {
		if got := validHostURL(u); got != want {
			t.Errorf("validHostURL(%q) = %v", u, got)
		}
	}
	s := Settings{Hosts: []Host{{Name: "web", URL: "https://web-01:8088/"}, {Name: "web", URL: "https://other"}, {Name: "bad", URL: "ftp://x"}}, Shortcut: "nonsense"}.clean()
	if len(s.Hosts) != 1 || s.Hosts[0].URL != "https://web-01:8088" || s.Shortcut != "" {
		t.Errorf("clean: %+v", s)
	}
}

func devRow(port uint16, pid int, root string) app.Row {
	return app.Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: port, State: scanner.StateListen, PID: pid,
		Category: "dev", Project: project.Info{Name: "shop", Root: root, Framework: "Vite"}}
}

func TestRecentProjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recent.json")
	st := newRecentStore(path)
	reads := 0
	self, _ := procinfo.Load(os.Getpid()) // the test runs as the user portop sees as "own"
	info := func(pid int) (procinfo.Info, []string, error) {
		reads++
		return procinfo.Info{User: self.User, Cmdline: "node vite --port 5173", Cwd: dir}, []string{"node", "vite", "--port", "5173"}, nil
	}
	now := time.Now()
	st.observe([]app.Row{devRow(5173, 10, dir), devRow(5174, 10, dir)}, now, info)
	st.observe([]app.Row{devRow(5173, 10, dir), devRow(5174, 10, dir)}, now.Add(time.Second), info)
	list := st.list()
	if len(list) != 1 || !list[0].Running || list[0].PID != 10 || len(list[0].Ports) != 2 || list[0].Command != "node vite --port 5173" || reads != 1 {
		t.Fatalf("running: %+v (reads %d)", list, reads)
	}
	// Stopped: still listed, not running; saved and loaded again.
	st.observe(nil, now.Add(2*time.Second), info)
	if err := st.save(); err != nil {
		t.Fatal(err)
	}
	again := newRecentStore(path).list()
	if len(again) != 1 || again[0].Running || again[0].Dir != dir {
		t.Fatalf("reloaded: %+v", again)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(path); fi.Mode().Perm()&0o077 != 0 {
			t.Errorf("recent file mode %v", fi.Mode())
		}
	}
	// A restart under a new PID is the same project.
	st.observe([]app.Row{devRow(5173, 11, dir)}, now.Add(3*time.Second), info)
	if list := st.list(); len(list) != 1 || list[0].PID != 11 {
		t.Fatalf("restarted: %+v", list)
	}
	b := New(filepath.Join(dir, "desktop.json"))
	b.recent = st
	if err := b.Launch(list[0].ID); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Errorf("launching a running project: %v", err)
	}
	if err := b.Launch("nope"); err == nil {
		t.Error("an unknown project was launched")
	}
	if err := b.Forget(list[0].ID); err != nil || len(b.Recent()) != 0 {
		t.Errorf("forget: %v %+v", err, b.Recent())
	}
}

func TestLongHistoryBuckets(t *testing.T) {
	h := newLongHistory()
	t0 := time.Unix(0, 0)
	live := map[int]bool{7: true}
	for i := 0; i <= 15; i++ { // 2 s scans: one bucket closes at 30 s
		h.add(t0.Add(time.Duration(i)*2*time.Second), TrafficPoint{RX: 100, TX: 10}, map[int]TrafficPoint{7: {RX: 50}}, live)
	}
	if len(h.total) != 1 || h.total[0].RX != 100 || h.total[0].TX != 10 || len(h.byPID[7]) != 1 || h.byPID[7][0].RX != 50 {
		t.Fatalf("buckets: %+v %+v", h.total, h.byPID)
	}
	for i := 16; i <= 31; i++ { // the process is gone
		h.add(t0.Add(time.Duration(i)*2*time.Second), TrafficPoint{}, nil, map[int]bool{})
	}
	if _, ok := h.byPID[7]; ok {
		t.Error("a gone process kept its history")
	}
}

func TestFreePortsNear(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	busy := l.Addr().(*net.TCPAddr).Port
	b := New(filepath.Join(t.TempDir(), "s.json"))
	got := b.FreePortsNear(busy)
	if len(got) == 0 || got[0] == busy {
		t.Fatalf("FreePortsNear(%d) = %v", busy, got)
	}
	if len(b.FreePortsNear(0)) != 0 {
		t.Error("port 0 gave suggestions")
	}
}

func TestExportWithoutDialog(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "s.json"))
	b.snapshot.Rows = []Row{{Protocol: "TCP", LocalAddress: "0.0.0.0", LocalPort: 80, State: "LISTEN", Listening: true, Process: "=cmd|' /C calc'!A0", Category: "web"}}
	res, err := b.Export("csv")
	if err != nil || res.Content == "" || !strings.HasSuffix(res.Name, ".csv") {
		t.Fatalf("csv: %+v %v", res, err)
	}
	if !strings.Contains(res.Content, "'=cmd") {
		t.Errorf("a formula-like process name was not escaped: %s", res.Content)
	}
	res, err = b.Export("json")
	var rows []Row
	if err != nil || json.Unmarshal([]byte(res.Content), &rows) != nil || len(rows) != 1 {
		t.Fatalf("json: %v %s", err, res.Content)
	}
	if _, err := b.Export("xml"); err == nil {
		t.Error("an unknown format was accepted")
	}
	// With a dialog the file is written where the user chose.
	out := filepath.Join(t.TempDir(), "ports.json")
	b.SaveDialog = func(string, string) (string, error) { return out, nil }
	if res, err := b.Export("json"); err != nil || res.Path != out {
		t.Fatalf("dialog: %+v %v", res, err)
	}
	b.SaveDialog = func(string, string) (string, error) { return "", nil }
	if res, _ := b.Export("json"); !res.Cancelled {
		t.Error("a cancelled dialog was not reported")
	}
}

func TestSplitCommandAndEditor(t *testing.T) {
	if got := splitCommand(`open -a "Visual Studio Code"`); len(got) != 3 || got[2] != "Visual Studio Code" {
		t.Errorf("splitCommand = %q", got)
	}
	if runtime.GOOS == "windows" {
		return
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "opened")
	script := filepath.Join(dir, "fake-editor")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$1\" > "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := New(filepath.Join(dir, "s.json"))
	b.SaveSettings(Settings{Editor: script})
	project := filepath.Join(dir, "shop")
	os.Mkdir(project, 0o755)
	if err := b.OpenInEditor(project); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if got, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(got)) == project {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got, _ := os.ReadFile(marker); strings.TrimSpace(string(got)) != project {
		t.Errorf("the editor got %q", got)
	}
	if err := b.OpenInEditor(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing folder was opened")
	}
}

func TestBaselineSaveAndDiff(t *testing.T) {
	dir := t.TempDir()
	b := New(filepath.Join(dir, "s.json"))
	b.BaselinePath = filepath.Join(dir, "baseline.json")
	if st, err := b.Baseline(); err != nil || st.Exists {
		t.Fatalf("no baseline: %+v %v", st, err)
	}
	listen := func(port uint16) app.Row {
		return app.Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: port, State: scanner.StateListen, PID: 1, ProcessName: "nginx"}
	}
	b.lastRows = []app.Row{listen(80), listen(443)}
	if st, err := b.SaveBaseline(); err != nil || !st.Exists || len(st.Added)+len(st.Removed) != 0 {
		t.Fatalf("saved: %+v %v", st, err)
	}
	b.lastRows = []app.Row{listen(80), listen(8080)}
	st, err := b.Baseline()
	if err != nil || len(st.Added) != 1 || st.Added[0].Port != 8080 || len(st.Removed) != 1 || st.Removed[0].Port != 443 {
		t.Fatalf("diff: %+v %v", st, err)
	}
	if n := b.baselineChanges(b.lastRows); n != 2 {
		t.Errorf("baselineChanges = %d", n)
	}
}

func TestRemoteHost(t *testing.T) {
	rows := []map[string]any{{"protocol": "TCP", "local_address": "0.0.0.0", "local_port": 5432, "state": "LISTEN", "scope": "all", "pid": 90, "process": "postgres", "category": "database"}}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pw, ok := r.BasicAuth()
		if !ok || user != "admin" || pw != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rows)
	}))
	defer srv.Close()

	b := New(filepath.Join(t.TempDir(), "s.json"))
	if _, err := b.SaveSettings(Settings{Hosts: []Host{{Name: "db-01", URL: srv.URL, User: "admin"}}}); err != nil {
		t.Fatal(err)
	}
	// A self-signed certificate is refused, with its fingerprint.
	err := b.Connect("db-01", "secret")
	ue, ok := err.(*UntrustedCertError)
	if !ok || len(ue.Fingerprint) != 64 {
		t.Fatalf("untrusted certificate: %v", err)
	}
	s := b.Settings()
	s.Hosts[0].Fingerprint = ue.Fingerprint
	b.SaveSettings(s)
	if err := b.Connect("db-01", "wrong"); err == nil || !strings.Contains(err.Error(), "wrong user name or password") {
		t.Fatalf("wrong password: %v", err)
	}
	if err := b.Connect("db-01", "secret"); err != nil {
		t.Fatal(err)
	}
	snap, err := b.RemoteSnapshot("db-01")
	if err != nil || len(snap.Rows) != 1 || snap.Rows[0].Process != "postgres" || !snap.Rows[0].AtRisk || snap.Summary.AtRisk != 1 {
		t.Fatalf("snapshot: %+v %v", snap, err)
	}
	// A different certificate under the same pin is refused.
	s.Hosts[0].Fingerprint = strings.Repeat("0", 64)
	b.SaveSettings(s)
	if _, err := b.RemoteSnapshot("db-01"); err == nil {
		t.Error("a certificate that does not match the pin was accepted")
	}
	b.Disconnect("db-01")
}
