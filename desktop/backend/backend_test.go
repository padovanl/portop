package backend

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/events"
	"github.com/padovanl/portop/internal/project"
	"github.com/padovanl/portop/internal/scanner"
)

func TestSettingsAreCleanedAndSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portop", "desktop.json")
	b := New(path)
	got, err := b.SaveSettings(Settings{Theme: "neon", IntervalMS: 10, Watched: []uint16{8080, 0, 3000, 8080}, Pinned: nil})
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "system" || got.IntervalMS != 500 || len(got.Watched) != 2 || got.Watched[0] != 3000 || got.Pinned == nil {
		t.Fatalf("cleaned settings: %+v", got)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil || st.Mode().Perm()&0o077 != 0 {
			t.Errorf("settings file mode: %v %v", st.Mode(), err)
		}
	}
	again := New(path).Settings()
	if again.IntervalMS != 500 || len(again.Watched) != 2 {
		t.Errorf("reloaded settings: %+v", again)
	}
	if New(filepath.Join(t.TempDir(), "missing.json")).Settings().IntervalMS != 2000 {
		t.Error("defaults are not used when there is no file")
	}
}

func TestIsLocalURL(t *testing.T) {
	for url, want := range map[string]bool{
		"http://localhost:3000":         true,
		"https://127.0.0.1:8443/admin":  true,
		"http://[::1]:5173":             true,
		"http://example.com":            false,
		"file:///etc/passwd":            false,
		"javascript:alert(1)":           false,
		"http://user:pw@localhost:3000": false,
		"http://8.8.8.8:53":             false,
	} {
		if got := isLocalURL(url); got != want {
			t.Errorf("isLocalURL(%q) = %v, want %v", url, got, want)
		}
	}
}

func row(port uint16, state scanner.State, pid int, addr string) app.Row {
	return app.Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP(addr), LocalPort: port, State: state, PID: pid, ProcessName: "node"}
}

func TestConvertSummarisesRows(t *testing.T) {
	dev := row(5173, scanner.StateListen, 10, "127.0.0.1")
	dev.Category, dev.Project = "dev", project.Info{Name: "shop", Framework: "Vite"}
	dev6 := row(5173, scanner.StateListen, 10, "::1")
	dev6.Category = "dev"
	exposed := row(8080, scanner.StateListen, 11, "0.0.0.0")
	exposed.Category = "web"
	hidden := row(22, scanner.StateListen, 0, "0.0.0.0")
	conn := row(5173, scanner.StateEstablished, 10, "127.0.0.1")
	conn.RemoteAddr, conn.RemotePort = net.ParseIP("127.0.0.1"), 50000
	conn.RXRate, conn.TXRate, conn.RateKnown = 100, 2000, true

	rows, s, nHidden := convert([]app.Row{conn, dev, dev6, exposed, hidden})
	if s.Listening != 4 || s.Established != 1 || s.Processes != 2 || s.DevServers != 1 || s.Exposed != 2 || s.RXRate != 100 || s.TXRate != 2000 || nHidden != 1 {
		t.Errorf("summary: %+v, hidden %d", s, nHidden)
	}
	if !rows[0].Listening || rows[len(rows)-1].Listening {
		t.Error("listening rows do not come first")
	}
	for _, r := range rows {
		if r.LocalPort == 5173 && r.Listening && r.LocalAddress == "127.0.0.1" && (r.URL != "http://localhost:5173" || r.Project != "shop") {
			t.Errorf("dev row: %+v", r)
		}
		if !r.Listening && (r.RXRate == nil || *r.TXRate != 2000) {
			t.Errorf("connection rates: %+v", r)
		}
	}
}

func TestAlertNotifiesWatchedPorts(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "s.json"))
	if _, err := b.SaveSettings(Settings{Watched: []uint16{3000}}); err != nil {
		t.Fatal(err)
	}
	var got []string
	b.OnNotify = func(title, body string) { got = append(got, title+" | "+body) }
	b.alert([]events.Event{
		{Kind: events.Opened, Port: 3000, Protocol: "TCP", Process: "node", Project: "shop"},
		{Kind: events.Opened, Port: 9999, Protocol: "TCP", Process: "nc"},
		{Kind: events.Closed, Port: 3000, Protocol: "TCP", Process: "node"},
	})
	if len(got) != 2 || !strings.Contains(got[0], "Port 3000 is now open | shop is listening") || !strings.Contains(got[1], "Port 3000 closed") {
		t.Fatalf("notifications: %q", got)
	}
	b.SaveSettings(Settings{NotifyNew: true})
	got = nil
	b.alert([]events.Event{{Kind: events.Opened, Port: 9999, Protocol: "TCP"}, {Kind: events.Closed, Port: 9999, Protocol: "TCP"}})
	if len(got) != 1 || !strings.Contains(got[0], "an unknown process") {
		t.Errorf("notify_new: %q", got)
	}
}

func TestShellQuoteSurvivesTheShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available")
	}
	args := []string{"printf", "%s|", "plain", "two words", "it's", `$HOME`, "a;b", ""}
	out, err := exec.Command("sh", "-c", shellJoin(args)).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "plain|two words|it's|$HOME|a;b||"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
	line := keepOpen("/tmp/my dir", "echo hi")
	if !strings.HasPrefix(line, "cd '/tmp/my dir' && echo hi;") {
		t.Errorf("keepOpen = %q", line)
	}
}

func TestAppleScriptString(t *testing.T) {
	if got := appleScriptString(`say "hi" \ bye`); got != `"say \"hi\" \\ bye"` {
		t.Errorf("appleScriptString = %s", got)
	}
}

func TestStopRejectsAStaleStartTime(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "s.json"))
	if err := b.Stop(os.Getpid(), "not a time", false); err == nil {
		t.Fatal("a malformed start time was accepted")
	}
	// The test process itself, with a start time that is not its own:
	// nothing may be signalled.
	if err := b.Stop(os.Getpid(), time.Unix(1, 0).Format(time.RFC3339Nano), false); err == nil {
		t.Fatal("a process was signalled with the wrong start time")
	}
}

func TestProcessDescribesTheTestProcess(t *testing.T) {
	b := New(filepath.Join(t.TempDir(), "s.json"))
	info, err := b.Process(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if !info.Own || info.Started == "" || info.Name == "" {
		t.Errorf("process info: %+v", info)
	}
}
