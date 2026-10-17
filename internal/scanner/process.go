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
	owners := make(map[int][]int) // conns index -> every PID holding it
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
			if len(owners[idx]) > 0 && owners[idx][len(owners[idx])-1] == pid {
				continue // the same process holds it through another fd
			}
			owners[idx] = append(owners[idx], pid)
			conns[idx].PID = pid
			name, ok := nameCache[pid]
			if !ok {
				name = readComm(pid)
				nameCache[pid] = name
			}
			conns[idx].ProcessName = name
		}
	}
	for idx, pids := range owners {
		if len(pids) > 1 {
			conns[idx].setOwners(pids, readParent, readComm)
		}
	}
	return conns
}

// readParent returns pid's parent from /proc/<pid>/stat, or 0.
func readParent(pid int) int {
	data, err := os.ReadFile(ProcRoot + "/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	s := string(data)
	end := strings.LastIndexByte(s, ')')
	if end < 0 || end+2 >= len(s) {
		return 0
	}
	fields := strings.Fields(s[end+2:]) // state, ppid, ...
	if len(fields) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(fields[1])
	return ppid
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
