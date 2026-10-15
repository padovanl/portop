package systemdinfo

import (
	"strings"
	"testing"
)

// RpcSs (Remote Procedure Call) runs on every Windows installation.
func TestWindowsRunningServicesIncludesRPC(t *testing.T) {
	for _, names := range runningServices() {
		for _, n := range strings.Split(names, ",") {
			if strings.EqualFold(n, "RpcSs") {
				return
			}
		}
	}
	t.Fatal("RpcSs not found among running services")
}
