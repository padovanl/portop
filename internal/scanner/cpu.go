package scanner

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// clockTicksPerSec is sysconf(_SC_CLK_TCK). It is 100 on essentially every
// Linux distribution/architecture in practice, and there is no portable
// way to read sysconf from pure Go without cgo, so we hardcode it like
// several other process-monitoring tools do.
const clockTicksPerSec = 100

type cpuSample struct {
	ticks uint64
	at    time.Time
}

// CPUTracker computes per-process CPU usage percentages by sampling
// /proc/<pid>/stat over time, the same technique top(1) uses.
type CPUTracker struct {
	mu   sync.Mutex
	prev map[int]cpuSample
}

// NewCPUTracker returns a ready-to-use tracker.
func NewCPUTracker() *CPUTracker {
	return &CPUTracker{prev: make(map[int]cpuSample)}
}

// Sample returns the CPU usage percentage for pid since the last call to
// Sample for that same pid. The first observation of a pid returns
// (0, false) since there is no prior sample to diff against.
func (t *CPUTracker) Sample(pid int) (percent float64, ok bool) {
	ticks, err := readProcTicks(pid)
	if err != nil {
		return 0, false
	}
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()
	prev, seen := t.prev[pid]
	t.prev[pid] = cpuSample{ticks: ticks, at: now}
	if !seen || ticks < prev.ticks {
		return 0, false
	}

	elapsed := now.Sub(prev.at).Seconds()
	if elapsed <= 0 {
		return 0, false
	}
	deltaTicks := float64(ticks - prev.ticks)
	pct := (deltaTicks / (elapsed * clockTicksPerSec)) * 100
	return pct, true
}

// Forget discards any cached sample for a pid that has disappeared, so a
// future reused pid does not inherit a stale baseline.
func (t *CPUTracker) Forget(pid int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.prev, pid)
}

func readProcTicks(pid int) (uint64, error) {
	data, err := os.ReadFile(ProcRoot + "/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	// comm is the process name in parens and may itself contain ')'
	// or spaces, so split on the last ')' before parsing fixed fields.
	s := string(data)
	close := strings.LastIndexByte(s, ')')
	if close == -1 || close+2 >= len(s) {
		return 0, os.ErrInvalid
	}
	rest := strings.Fields(s[close+2:])
	// rest[0] = state, rest[1] = ppid, ... rest[11] = utime, rest[12] = stime
	// (fields 3.. of /proc/pid/stat, 0-indexed here from state).
	const utimeIdx, stimeIdx = 11, 12
	if len(rest) <= stimeIdx {
		return 0, os.ErrInvalid
	}
	utime, err := strconv.ParseUint(rest[utimeIdx], 10, 64)
	if err != nil {
		return 0, err
	}
	stime, err := strconv.ParseUint(rest[stimeIdx], 10, 64)
	if err != nil {
		return 0, err
	}
	return utime + stime, nil
}
