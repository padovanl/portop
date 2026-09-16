package procinfo

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/padovanl/portop/internal/hostcmd"
)

func Load(pid int) (Info, error) {
	if pid <= 0 {
		return Info{}, fmt.Errorf("procinfo: invalid pid %d", pid)
	}
	id := strconv.Itoa(pid)
	out, err := hostcmd.Output("/bin/ps", "-p", id, "-o", "user=,rss=,lstart=")
	if err != nil {
		return Info{}, fmt.Errorf("procinfo: pid %d: %w", pid, err)
	}
	info, err := parsePSInfo(pid, string(out))
	if err != nil {
		return Info{}, err
	}
	if out, err := hostcmd.Output("/bin/ps", "-ww", "-p", id, "-o", "command="); err == nil {
		info.Cmdline = strings.TrimSpace(string(out))
	}
	if out, err := hostcmd.Output("/bin/ps", "-ww", "-p", id, "-o", "comm="); err == nil {
		info.Exe = strings.TrimSpace(string(out))
		info.Name = filepath.Base(info.Exe)
	}
	// NUL fields preserve spaces/newlines in paths. Numeric descriptors
	// exclude cwd, executable mappings and other pseudo-descriptors.
	if out, err := hostcmd.Output("/usr/sbin/lsof", "-nP", "-a", "-p", id, "-F0fn"); err == nil {
		info.OpenFiles = 0
		fd := ""
		for _, field := range strings.Split(string(out), "\x00") {
			field = strings.TrimLeft(field, "\n")
			if field == "" {
				continue
			}
			switch field[0] {
			case 'f':
				fd = field[1:]
				if _, err := strconv.Atoi(fd); err == nil {
					info.OpenFiles++
				}
			case 'n':
				if fd == "cwd" {
					info.Cwd = field[1:]
				}
			}
		}
	}
	return info, nil
}
