package scanner

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/padovanl/portop/internal/hostcmd"
)

// lsof supplies ownership along with socket records, so ResolveProcesses
// needs no /proc lookup (these connections have no Linux inode).
func Scan() ([]Connection, error) {
	out, err := hostcmd.Output("/usr/sbin/lsof", "-nP", "-iTCP", "-iUDP", "-F0pcuftPnT", "-Ts")
	if err != nil {
		// lsof uses status 1 for an empty selection as well as errors.
		if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 || len(out) != 0 || len(e.Stderr) != 0 {
			return nil, fmt.Errorf("scanner: lsof: %w", err)
		}
	}
	return parseLsof(out), nil
}

func readProcessTicks(pid int) (uint64, error) {
	out, err := hostcmd.Output("/bin/ps", "-p", strconv.Itoa(pid), "-o", "time=")
	if err != nil {
		return 0, err
	}
	return parseCPUTime(strings.TrimSpace(string(out)))
}
