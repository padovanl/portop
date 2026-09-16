package procinfo

import "testing"

func TestParsePSInfo(t *testing.T) {
	info, err := parsePSInfo(42, "alice 2048 Tue Sep  1 09:08:07 2026\n")
	if err != nil {
		t.Fatal(err)
	}
	if info.PID != 42 || info.User != "alice" || info.RSSBytes != 2097152 || info.StartTime.Day() != 1 || info.OpenFiles != -1 {
		t.Fatalf("%+v", info)
	}
	for _, bad := range []string{"", "alice invalid Tue Sep 1 09:08:07 2026", "alice 123 invalid date"} {
		if _, err := parsePSInfo(42, bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
