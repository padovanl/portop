package events

import (
	"net"
	"testing"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/scanner"
)

func listen(port uint16, pid int, addr string) app.Row {
	return app.Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP(addr), LocalPort: port, State: scanner.StateListen, PID: pid, ProcessName: "node"}
}

func TestLogRecordsOpenedAndClosed(t *testing.T) {
	l := NewLog(100)
	t0 := time.Unix(1000, 0)
	if got := l.Update([]app.Row{listen(22, 1, "0.0.0.0")}, t0); len(got) != 0 {
		t.Fatalf("the first scan produced events: %+v", got)
	}

	// A dev server binding 127.0.0.1 and ::1 is one opened port; the
	// established connection is not an event.
	vite := listen(5173, 40, "127.0.0.1")
	vite.Project.Name, vite.Project.Framework, vite.Category = "shop", "Vite", "dev"
	conn := app.Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 5173, RemotePort: 50000, State: scanner.StateEstablished, PID: 40}
	got := l.Update([]app.Row{listen(22, 1, "0.0.0.0"), vite, listen(5173, 40, "::1"), conn}, t0.Add(time.Second))
	if len(got) != 1 || got[0].Kind != Opened || got[0].Port != 5173 || got[0].Project != "shop" || got[0].Framework != "Vite" || got[0].Scope != app.ScopeLoopback {
		t.Fatalf("opened: %+v", got)
	}

	// Restarted under a new PID: closed, then opened.
	got = l.Update([]app.Row{listen(22, 1, "0.0.0.0"), listen(5173, 41, "0.0.0.0"), listen(5173, 41, "127.0.0.1")}, t0.Add(2*time.Second))
	if len(got) != 2 || got[0].Kind != Closed || got[0].PID != 40 || got[1].Kind != Opened || got[1].PID != 41 || got[1].Scope != app.ScopeAll {
		t.Fatalf("restart: %+v", got)
	}
	if got[0].ID >= got[1].ID || !got[1].Time.Equal(t0.Add(2*time.Second)) {
		t.Errorf("ids or time: %+v", got)
	}
	if n := len(l.Events()); n != 3 {
		t.Errorf("%d events recorded, want 3", n)
	}
}

func TestLogKeepsTheNewest(t *testing.T) {
	l := NewLog(3)
	l.Update(nil, time.Now())
	for port := uint16(1); port <= 5; port++ {
		l.Update([]app.Row{listen(port, int(port), "0.0.0.0")}, time.Now())
	}
	got := l.Events()
	if len(got) != 3 || got[len(got)-1].Port != 5 || got[len(got)-1].Kind != Opened {
		t.Fatalf("events: %+v", got)
	}
}
