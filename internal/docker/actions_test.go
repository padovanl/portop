package docker

import (
	"context"
	"encoding/binary"
	"testing"
)

func frame(stream byte, s string) []byte {
	h := []byte{stream, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(h[4:], uint32(len(s)))
	return append(h, s...)
}

func TestDemuxLogs(t *testing.T) {
	mux := append(frame(1, "listening on 5432\n"), frame(2, "warning: x\n")...)
	if got := string(demuxLogs(mux)); got != "listening on 5432\nwarning: x\n" {
		t.Errorf("demux = %q", got)
	}
	plain := []byte("a tty container\nprints plain text\n")
	if got := string(demuxLogs(plain)); got != string(plain) {
		t.Errorf("plain text was changed: %q", got)
	}
}

func TestActRefusesBadInput(t *testing.T) {
	c := NewClient()
	for _, bad := range []struct{ name, action string }{
		{"../containers/x", ActionStop},
		{"web?force=1", ActionStop},
		{"", ActionStop},
		{"web", "kill"},
	} {
		if err := c.Act(context.Background(), bad.name, bad.action); err == nil {
			t.Errorf("Act(%q, %q) was accepted", bad.name, bad.action)
		}
	}
	if _, err := c.Logs(context.Background(), "a/b", 10); err == nil {
		t.Error("Logs accepted a path as a name")
	}
}
