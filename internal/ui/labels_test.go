package ui

import "testing"

func TestLabelsFor(t *testing.T) {
	linux := labelsFor("linux")
	if linux.unitColumn != "SYSTEMD" || linux.terminate != "terminate (SIGTERM)" || linux.unitSort != "systemd" {
		t.Errorf("linux labels changed: %+v", linux)
	}
	if darwin := labelsFor("darwin"); darwin != linux {
		t.Errorf("darwin labels differ from linux: %+v", darwin)
	}
	win := labelsFor("windows")
	if win.unitColumn != "SERVICE" || win.unitField != "Service:" || win.terminate == linux.terminate {
		t.Errorf("windows labels: %+v", win)
	}
}
