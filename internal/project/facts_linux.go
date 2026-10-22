package project

import (
	"bytes"
	"os"
	"strconv"
	"strings"
)

// procRoot is /proc, overridable in tests.
var procRoot = "/proc"

func processFacts(pids []int) map[int]Facts {
	out := make(map[int]Facts, len(pids))
	for _, pid := range pids {
		base := procRoot + "/" + strconv.Itoa(pid)
		cmdline, err := os.ReadFile(base + "/cmdline")
		if err != nil {
			continue
		}
		f := Facts{Cmdline: strings.TrimSpace(string(bytes.ReplaceAll(cmdline, []byte{0}, []byte{' '})))}
		if comm, err := os.ReadFile(base + "/comm"); err == nil {
			f.Name = strings.TrimSpace(string(comm))
		}
		f.Exe, _ = os.Readlink(base + "/exe")
		f.Cwd, _ = os.Readlink(base + "/cwd")
		// A directory removed since the process entered it reads
		// "/path (deleted)"; walking up from there would find whatever
		// project contains the old path.
		if strings.HasSuffix(f.Cwd, " (deleted)") {
			f.Cwd = ""
		}
		out[pid] = f
	}
	return out
}
