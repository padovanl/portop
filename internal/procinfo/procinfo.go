// Package procinfo reads detailed, on-demand information about a single
// process for the TUI's detail view (Enter on a row): command line,
// executable path, working directory, owning user, memory usage and
// start time.
package procinfo

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"
)

// ProcRoot is the root of the /proc filesystem, overridable in tests.
var ProcRoot = "/proc"

// bootTime and clockTicksPerSec are used to convert /proc/<pid>/stat's
// starttime (in clock ticks since boot) into a wall-clock time.
const clockTicksPerSec = 100

// Info is everything portop's detail view shows about a process.
type Info struct {
	PID        int
	Name       string
	Cmdline    string // full command line, args space-joined
	Exe        string // resolved path of the executable, if readable
	Cwd        string // resolved working directory, if readable
	User       string // resolved username, or the raw uid if unresolvable
	RSSBytes   uint64 // resident set size
	StartTime  time.Time
	NumThreads int
	OpenFiles  int // number of entries in /proc/<pid>/fd, -1 if unreadable
}

// Load gathers everything available for pid. Fields that cannot be read
// (permission denied, process gone, exotic /proc layout) are left at
// their zero value rather than making the whole call fail: partial
// information is still useful to show.
func Load(pid int) (Info, error) {
	base := ProcRoot + "/" + strconv.Itoa(pid)
	if _, err := os.Stat(base); err != nil {
		return Info{}, fmt.Errorf("procinfo: pid %d not found: %w", pid, err)
	}

	info := Info{PID: pid, OpenFiles: -1}
	info.Name = readComm(base)
	info.Cmdline = readCmdline(base)
	info.Exe, _ = os.Readlink(base + "/exe")
	info.Cwd, _ = os.Readlink(base + "/cwd")
	info.User = readUser(base)

	if fds, err := os.ReadDir(base + "/fd"); err == nil {
		info.OpenFiles = len(fds)
	}

	rss, threads, startTicks := readStat(base)
	info.RSSBytes = rss * uint64(os.Getpagesize())
	info.NumThreads = threads
	if bt, err := bootTime(); err == nil {
		info.StartTime = bt.Add(time.Duration(startTicks) * time.Second / clockTicksPerSec)
	}

	return info, nil
}

func readComm(base string) string {
	data, err := os.ReadFile(base + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readCmdline(base string) string {
	data, err := os.ReadFile(base + "/cmdline")
	if err != nil {
		return ""
	}
	// Arguments are NUL-separated with a trailing NUL.
	parts := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	return strings.Join(parts, " ")
}

func readUser(base string) string {
	data, err := os.ReadFile(base + "/status")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return ""
		}
		uid := fields[1]
		if u, err := user.LookupId(uid); err == nil {
			return u.Username
		}
		return uid
	}
	return ""
}

// readStat returns (rss in pages, numThreads, starttime in clock ticks
// since boot), best-effort.
func readStat(base string) (rss uint64, numThreads int, startTicks uint64) {
	data, err := os.ReadFile(base + "/stat")
	if err != nil {
		return 0, 0, 0
	}
	s := string(data)
	closeParen := strings.LastIndexByte(s, ')')
	if closeParen == -1 || closeParen+2 >= len(s) {
		return 0, 0, 0
	}
	fields := strings.Fields(s[closeParen+2:])
	// Fields here are 0-indexed starting at "state" (field 3 overall).
	// numThreads is field 20 overall -> index 17.
	// starttime is field 22 overall -> index 19.
	// rss is field 24 overall -> index 21.
	get := func(idx int) uint64 {
		if idx < 0 || idx >= len(fields) {
			return 0
		}
		v, _ := strconv.ParseUint(fields[idx], 10, 64)
		return v
	}
	numThreads = int(get(17))
	startTicks = get(19)
	rss = get(21)
	return rss, numThreads, startTicks
}

func bootTime() (time.Time, error) {
	data, err := os.ReadFile(ProcRoot + "/stat")
	if err != nil {
		return time.Time{}, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "btime ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return time.Time{}, fmt.Errorf("procinfo: malformed btime line")
		}
		secs, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(secs, 0), nil
	}
	return time.Time{}, fmt.Errorf("procinfo: btime not found")
}
