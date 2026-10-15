package procctl

import (
	"errors"
	"testing"
	"time"
)

func stubSignals(t *testing.T, start time.Time) *[]string {
	t.Helper()
	oldLoad, oldTerm, oldKill := loadStart, sendTerminate, sendKill
	t.Cleanup(func() { loadStart, sendTerminate, sendKill = oldLoad, oldTerm, oldKill })
	var sent []string
	loadStart = func(int) (time.Time, error) { return start, nil }
	sendTerminate = func(int) error { sent = append(sent, "TERM"); return nil }
	sendKill = func(int) error { sent = append(sent, "KILL"); return nil }
	return &sent
}

func TestSignalIfUnchangedSendsWhenIdentityMatches(t *testing.T) {
	start := time.Unix(1700000000, 0)
	sent := stubSignals(t, start)
	if err := SignalIfUnchanged(42, start, false); err != nil {
		t.Fatal(err)
	}
	if err := SignalIfUnchanged(42, start, true); err != nil {
		t.Fatal(err)
	}
	if len(*sent) != 2 || (*sent)[0] != "TERM" || (*sent)[1] != "KILL" {
		t.Fatalf("signals sent = %v", *sent)
	}
}

func TestSignalIfUnchangedRefusesReusedPID(t *testing.T) {
	sent := stubSignals(t, time.Unix(1700000500, 0))
	err := SignalIfUnchanged(42, time.Unix(1700000000, 0), true)
	if !errors.Is(err, ErrProcessChanged) || len(*sent) != 0 {
		t.Fatalf("err=%v sent=%v, want ErrProcessChanged and no signal", err, *sent)
	}
}

func TestSignalIfUnchangedFailsClosed(t *testing.T) {
	sent := stubSignals(t, time.Time{})
	if err := SignalIfUnchanged(42, time.Unix(1700000000, 0), false); err == nil {
		t.Fatal("signalled a process whose start time is unknown")
	}
	if err := SignalIfUnchanged(42, time.Time{}, false); err == nil {
		t.Fatal("signalled without an expected identity")
	}
	loadStart = func(int) (time.Time, error) { return time.Time{}, errors.New("gone") }
	if err := SignalIfUnchanged(42, time.Unix(1700000000, 0), false); err == nil {
		t.Fatal("signalled a process that disappeared")
	}
	if len(*sent) != 0 {
		t.Fatalf("signals sent = %v", *sent)
	}
}
