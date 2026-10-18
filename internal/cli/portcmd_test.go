package cli

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/procctl"
	"github.com/padovanl/portop/internal/scanner"
)

func listener(port uint16, pid int, name string) app.Row {
	return app.Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: port,
		State: scanner.StateListen, PID: pid, ProcessName: name}
}

// fakeEnv serves one scan result per call (the last one repeats) on a
// virtual clock that advances with every sleep.
type fakeEnv struct {
	scans   [][]app.Row
	calls   int
	clock   time.Time
	signals []string
}

func (f *fakeEnv) env(stdin string, tty bool) portEnv {
	f.clock = time.Unix(1700000000, 0)
	return portEnv{
		scan: func(context.Context) ([]app.Row, error) {
			rows := f.scans[min(f.calls, len(f.scans)-1)]
			f.calls++
			return rows, nil
		},
		startTime: func(pid int) (time.Time, error) { return time.Unix(int64(pid), 0), nil },
		signal: func(pid int, start time.Time, force bool) error {
			kind := "TERM"
			if force {
				kind = "KILL"
			}
			f.signals = append(f.signals, kind+" "+start.Format("05"))
			return nil
		},
		sleep: func(d time.Duration) { f.clock = f.clock.Add(d) },
		now:   func() time.Time { return f.clock },
		stdin: strings.NewReader(stdin),
		tty:   tty,
	}
}

func TestParsePortSpec(t *testing.T) {
	for in, want := range map[string]string{"8080": ":8080/tcp", "53/udp": ":53/udp", "443/TCP": ":443/tcp"} {
		got, err := parsePortSpec(in)
		if err != nil || got.String() != want {
			t.Errorf("parsePortSpec(%q) = %v, %v; want %s", in, got, err, want)
		}
	}
	for _, in := range []string{"", "0", "65536", "http", "80/sctp"} {
		if _, err := parsePortSpec(in); err == nil {
			t.Errorf("parsePortSpec(%q) accepted", in)
		}
	}
}

func TestWaitSucceedsOnceThePortListens(t *testing.T) {
	f := &fakeEnv{scans: [][]app.Row{nil, nil, {listener(5432, 7, "postgres")}}}
	var out, errOut bytes.Buffer
	if code := runWait(&out, &errOut, "5432", 30*time.Second, f.env("", false)); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if f.calls != 3 {
		t.Fatalf("scanned %d times, want 3", f.calls)
	}
}

func TestWaitTimesOut(t *testing.T) {
	f := &fakeEnv{scans: [][]app.Row{{listener(8080, 7, "other")}}}
	var out, errOut bytes.Buffer
	if code := runWait(&out, &errOut, "5432", 2*time.Second, f.env("", false)); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "within 2s") {
		t.Fatalf("stderr %q", errOut.String())
	}
}

// UDP is only matched when asked for: a TCP listener on 53 does not
// satisfy --wait 53/udp.
func TestWaitMatchesTheProtocol(t *testing.T) {
	f := &fakeEnv{scans: [][]app.Row{{listener(53, 7, "dns")}}}
	var out, errOut bytes.Buffer
	if code := runWait(&out, &errOut, "53/udp", time.Second, f.env("", false)); code != 1 {
		t.Fatalf("exit %d, want a timeout", code)
	}
}

func TestFreeSignalsEveryOwnerWithItsIdentity(t *testing.T) {
	// Two processes share the listener (a master and a worker), then the
	// port is released.
	held := []app.Row{listener(8080, 41, "node"), listener(8080, 42, "node"), listener(9090, 50, "other")}
	f := &fakeEnv{scans: [][]app.Row{held, held, {listener(9090, 50, "other")}}}
	var out, errOut bytes.Buffer
	if code := runFree(&out, &errOut, "8080", false, false, 5*time.Second, f.env("", false)); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if strings.Join(f.signals, ",") != "TERM 41,TERM 42" {
		t.Fatalf("signals = %v, want SIGTERM to 41 and 42 with their start times", f.signals)
	}
	if !strings.Contains(out.String(), ":8080/tcp is free") {
		t.Fatalf("stdout %q", out.String())
	}
}

func TestFreeAsksOnATerminal(t *testing.T) {
	held := []app.Row{listener(8080, 41, "node")}
	f := &fakeEnv{scans: [][]app.Row{held}}
	var out, errOut bytes.Buffer
	if code := runFree(&out, &errOut, "8080", true, false, time.Second, f.env("n\n", true)); code != 1 || len(f.signals) != 0 {
		t.Fatalf("exit %d, signals %v: a refused confirmation must not signal", code, f.signals)
	}

	f = &fakeEnv{scans: [][]app.Row{held, nil}}
	out.Reset()
	if code := runFree(&out, &errOut, "8080", true, false, time.Second, f.env("y\n", true)); code != 0 || strings.Join(f.signals, ",") != "KILL 41" {
		t.Fatalf("exit %d, signals %v, want SIGKILL after y", code, f.signals)
	}
}

func TestFreeReportsWhatItCannotDo(t *testing.T) {
	var out, errOut bytes.Buffer

	f := &fakeEnv{scans: [][]app.Row{nil}}
	if code := runFree(&out, &errOut, "8080", false, true, time.Second, f.env("", false)); code != 0 || !strings.Contains(out.String(), "nothing is listening") {
		t.Fatalf("free port: exit %d, stdout %q", code, out.String())
	}

	hidden := listener(8080, 0, "")
	f = &fakeEnv{scans: [][]app.Row{{hidden}}}
	if code := runFree(&out, &errOut, "8080", false, true, time.Second, f.env("", false)); code != 1 || len(f.signals) != 0 {
		t.Fatalf("unresolved owner: exit %d, signals %v", code, f.signals)
	}

	stubborn := []app.Row{listener(8080, 41, "node")}
	f = &fakeEnv{scans: [][]app.Row{stubborn}}
	errOut.Reset()
	if code := runFree(&out, &errOut, "8080", false, true, time.Second, f.env("", false)); code != 1 || !strings.Contains(errOut.String(), "--force") {
		t.Fatalf("still busy: exit %d, stderr %q", code, errOut.String())
	}

	env := (&fakeEnv{scans: [][]app.Row{stubborn}}).env("", false)
	env.signal = func(int, time.Time, bool) error { return procctl.ErrProcessChanged }
	if code := runFree(&out, &errOut, "8080", false, true, time.Second, env); code != 1 {
		t.Fatalf("changed process: exit %d, want 1", code)
	}
}

// SIGKILL to a master leaves its workers listening, so --force reaches
// every process holding the socket; SIGTERM goes to the master only.
func TestFreeForceReachesWorkers(t *testing.T) {
	master := listener(8080, 41, "nginx")
	master.Workers = []int{43, 42}
	f := &fakeEnv{scans: [][]app.Row{{master}, nil}}
	var out, errOut bytes.Buffer
	if code := runFree(&out, &errOut, "8080", true, true, time.Second, f.env("", false)); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if strings.Join(f.signals, ",") != "KILL 41,KILL 42,KILL 43" {
		t.Fatalf("signals = %v", f.signals)
	}

	f = &fakeEnv{scans: [][]app.Row{{master}, nil}}
	if code := runFree(&out, &errOut, "8080", false, true, time.Second, f.env("", false)); code != 0 || strings.Join(f.signals, ",") != "TERM 41" {
		t.Fatalf("exit %d, signals %v, want SIGTERM to the master only", code, f.signals)
	}
}
