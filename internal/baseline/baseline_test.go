package baseline

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/padovan93/portop/internal/app"
	"github.com/padovan93/portop/internal/scanner"
)

func TestFromRowsFiltersToListenOnly(t *testing.T) {
	rows := []app.Row{
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: 22, State: scanner.StateListen, ProcessName: "sshd"},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 51000, State: scanner.StateEstablished, ProcessName: "curl"},
		{Protocol: scanner.UDP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: 53, State: scanner.StateListen, ProcessName: "systemd-resolved"},
	}
	entries := FromRows(rows)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (LISTEN only): %+v", len(entries), entries)
	}
	if entries[0].Port != 22 || entries[1].Port != 53 {
		t.Errorf("entries not sorted by port: %+v", entries)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "baseline.json")
	entries := []Entry{{Protocol: "TCP", Port: 22, Process: "sshd"}, {Protocol: "TCP", Port: 80, Process: "nginx"}}

	if err := Save(path, entries); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, savedAt, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if savedAt.IsZero() {
		t.Error("SavedAt is zero, want a real timestamp")
	}
	if len(loaded) != 2 || loaded[0] != entries[0] || loaded[1] != entries[1] {
		t.Errorf("loaded = %+v, want %+v", loaded, entries)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, _, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Error("expected error loading a missing baseline, got nil")
	}
}

func TestDiff(t *testing.T) {
	base := []Entry{
		{Protocol: "TCP", Port: 22, Process: "sshd"},
		{Protocol: "TCP", Port: 80, Process: "nginx"},
	}
	current := []Entry{
		{Protocol: "TCP", Port: 22, Process: "sshd"},
		{Protocol: "TCP", Port: 4444, Process: "nc"}, // new, unexpected
		// :80 has stopped listening
	}

	added, removed := Diff(base, current)
	if len(added) != 1 || added[0].Port != 4444 {
		t.Errorf("added = %+v, want just :4444", added)
	}
	if len(removed) != 1 || removed[0].Port != 80 {
		t.Errorf("removed = %+v, want just :80", removed)
	}
}

func TestDiffNoChanges(t *testing.T) {
	entries := []Entry{{Protocol: "TCP", Port: 22, Process: "sshd"}}
	added, removed := Diff(entries, entries)
	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("expected no diff, got added=%+v removed=%+v", added, removed)
	}
}
