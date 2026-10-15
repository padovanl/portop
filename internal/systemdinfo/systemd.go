// Package systemdinfo associates a process with the systemd unit that
// owns it, by inspecting the process's cgroup membership. This works
// without talking to D-Bus or requiring any special privileges beyond
// being able to read /proc/<pid>/cgroup. On Windows the same slot holds
// the Windows service(s) hosted by the process (see unit_windows.go).
package systemdinfo

import (
	"regexp"
	"strings"
)

// ProcRoot is the root of the /proc filesystem, overridable in tests.
var ProcRoot = "/proc"

// unitPattern matches the last "<name>.<type>" segment of a cgroup path
// where type is one of the systemd unit suffixes.
var unitPattern = regexp.MustCompile(`([^/]+\.(service|scope|socket|timer|mount))$`)

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
