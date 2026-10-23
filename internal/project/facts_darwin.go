package project

import (
	"strconv"
	"strings"

	"github.com/padovanl/portop/internal/hostcmd"
)

// processFacts asks ps and lsof about every PID at once: one launch of
// each per scan that sees new processes.
func processFacts(pids []int) map[int]Facts {
	list := make([]string, len(pids))
	for i, pid := range pids {
		list[i] = strconv.Itoa(pid)
	}
	joined := strings.Join(list, ",")
	out := map[int]Facts{}
	if b, err := hostcmd.Output("/bin/ps", "-ww", "-o", "pid=,command=", "-p", joined); err == nil {
		for pid, cmd := range parsePSCommands(string(b)) {
			out[pid] = Facts{Cmdline: cmd}
		}
	}
	// lsof exits 1 when some PIDs are gone or not readable but still
	// prints the others.
	b, _ := hostcmd.Output("/usr/sbin/lsof", "-a", "-d", "cwd", "-p", joined, "-Fpn")
	for pid, cwd := range parseLsofCwd(string(b)) {
		f := out[pid]
		f.Cwd = cwd
		out[pid] = f
	}
	return out
}
