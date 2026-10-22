package project

import "github.com/padovanl/portop/internal/procinfo"

func processFacts(pids []int) map[int]Facts {
	out := make(map[int]Facts, len(pids))
	for _, pid := range pids {
		info, err := procinfo.Load(pid)
		if err != nil {
			continue
		}
		out[pid] = Facts{Name: info.Name, Exe: info.Exe, Cmdline: info.Cmdline, Cwd: info.Cwd}
	}
	return out
}
