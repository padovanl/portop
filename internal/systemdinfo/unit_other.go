//go:build !windows

package systemdinfo

import (
	"os"
	"strconv"
)

// UnitForPID returns the systemd unit name (e.g. "nginx.service") owning
// the given process, or "" if it could not be determined (process not
// under a recognizable systemd cgroup, cgroup v1 without a systemd
// controller, permission denied, container process, etc).
func UnitForPID(pid int) string {
	data, err := os.ReadFile(ProcRoot + "/" + strconv.Itoa(pid) + "/cgroup")
	if err != nil {
		return ""
	}
	return unitFromCgroupContent(string(data))
}
