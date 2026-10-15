package services

import "testing"

func TestWindowsServicesDatabase(t *testing.T) {
	if name, ok := Lookup(80, "TCP"); !ok || name != "http" {
		t.Fatalf("Lookup(80, TCP) = %q, %v; want http from %s", name, ok, Path)
	}
}
