package systemdinfo

import "testing"

func TestUnitFromCgroupContentV2(t *testing.T) {
	content := "0::/system.slice/nginx.service\n"
	if got := unitFromCgroupContent(content); got != "nginx.service" {
		t.Errorf("got %q, want nginx.service", got)
	}
}

func TestUnitFromCgroupContentV1(t *testing.T) {
	content := "12:pids:/system.slice/docker.service\n" +
		"11:cpu,cpuacct:/system.slice/postgresql.service\n" +
		"1:name=systemd:/system.slice/postgresql.service\n"
	if got := unitFromCgroupContent(content); got != "postgresql.service" {
		t.Errorf("got %q, want postgresql.service", got)
	}
}

func TestUnitFromCgroupContentUserSlice(t *testing.T) {
	content := "0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-vscode.slice/vscode.scope\n"
	if got := unitFromCgroupContent(content); got != "vscode.scope" {
		t.Errorf("got %q, want vscode.scope", got)
	}
}

func TestUnitFromCgroupContentNoMatch(t *testing.T) {
	content := "0::/user.slice/user-1000.slice/session-3.scope-not-a-real-suffix\n"
	if got := unitFromCgroupContent(content); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestUnitForPIDMissingProc(t *testing.T) {
	ProcRoot = t.TempDir()
	if got := UnitForPID(999999); got != "" {
		t.Errorf("got %q, want empty for missing /proc entry", got)
	}
}
