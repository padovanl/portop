package project

import (
	"strconv"
	"strings"
)

// parsePSCommands reads "ps -o pid=,command=" output.
func parsePSCommands(s string) map[int]string {
	out := map[int]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		id, cmd, _ := strings.Cut(line, " ")
		if pid, err := strconv.Atoi(id); err == nil {
			out[pid] = strings.TrimSpace(cmd)
		}
	}
	return out
}

// parseLsofCwd reads "lsof -d cwd -Fpn" output: a "p<pid>" line, then the
// "n<path>" of its cwd descriptor.
func parseLsofCwd(s string) map[int]string {
	out := map[int]string{}
	pid := 0
	for _, line := range strings.Split(s, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'n':
			if pid > 0 {
				out[pid] = line[1:]
			}
		}
	}
	return out
}
