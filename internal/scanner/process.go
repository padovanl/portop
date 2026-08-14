package scanner

import (
	"os"
	"strconv"
	"strings"
)

// ResolveProcesses walks /proc/<pid>/fd for every running process and
// matches socket inodes to PIDs, filling in PID and ProcessName on the
// given connections. Connections whose inode cannot be attributed to any
// visible process (permission denied, or the socket belongs to another
// user/namespace) are left with PID 0.
func ResolveProcesses(conns []Connection) []Connection {
	byInode := make(map[uint64]int, len(conns))
	for i := range conns {
		if conns[i].Inode != 0 {
			byInode[conns[i].Inode] = i
		}
	}
	if len(byInode) == 0 {
		return conns
	}

	entries, err := os.ReadDir(ProcRoot)
	if err != nil {
		return conns
	}

	nameCache := make(map[int]string)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fdDir := ProcRoot + "/" + e.Name() + "/fd"
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // no permission or process exited
		}
		for _, fd := range fds {
			target, err := os.Readlink(fdDir + "/" + fd.Name())
			if err != nil {
				continue
			}
			inode, ok := parseSocketInode(target)
			if !ok {
				continue
			}
			idx, tracked := byInode[inode]
			if !tracked {
				continue
			}
			conns[idx].PID = pid
			name, ok := nameCache[pid]
			if !ok {
				name = readComm(pid)
				nameCache[pid] = name
			}
			conns[idx].ProcessName = name
		}
	}
	return conns
}

// parseSocketInode extracts the inode from a symlink target of the form
// "socket:[12345]".
func parseSocketInode(target string) (uint64, bool) {
	if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
		return 0, false
	}
	inode, err := strconv.ParseUint(target[len("socket:["):len(target)-1], 10, 64)
	if err != nil {
		return 0, false
	}
	return inode, true
}

func readComm(pid int) string {
	data, err := os.ReadFile(ProcRoot + "/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
