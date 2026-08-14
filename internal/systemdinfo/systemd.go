// Package systemdinfo associates a process with the systemd unit that
// owns it, by inspecting the process's cgroup membership. This works
// without talking to D-Bus or requiring any special privileges beyond
// being able to read /proc/<pid>/cgroup.
package systemdinfo

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ProcRoot is the root of the /proc filesystem, overridable in tests.
var ProcRoot = "/proc"

// unitPattern matches the last "<name>.<type>" segment of a cgroup path
// where type is one of the systemd unit suffixes.
var unitPattern = regexp.MustCompile(`([^/]+\.(service|scope|socket|timer|mount))$`)

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

func unitFromCgroupContent(content string) string {
	var best string
	for _, line := range strings.Split(content, "\n") {
		// cgroup v2: "0::/path"; cgroup v1: "N:controller:/path"
		parts := strings.SplitN(line, ":", 3)
		path := line
		if len(parts) == 3 {
			path = parts[2]
		}
		if m := unitPattern.FindString(path); m != "" {
			best = m
		}
	}
	return best
}
