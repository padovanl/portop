package ui

import (
	"fmt"
	"os"
	"runtime"

	"github.com/padovanl/portop/internal/app"
)

// isRoot reports whether portop is likely able to see every process's
// /proc/<pid>/fd table. On Windows the permission model is different
// (and os.Geteuid is a stub returning -1 there), so we don't claim to
// know either way.
func isRoot() bool {
	return runtime.GOOS != "windows" && os.Geteuid() == 0
}

// unresolvedHint explains why a row has no PID: on Linux/macOS this is
// almost always another user's process (docker-proxy, systemd-resolved,
// sshd before it drops privileges, ...) whose /proc/<pid>/fd table isn't
// readable without root — the same limitation lsof/ss -p have.
func unresolvedHint() string {
	if isRoot() {
		return "no process associated with this row"
	}
	return "process unknown — likely owned by another user (try running portop with sudo)"
}

// countUnresolved returns how many LISTEN rows have no PID, i.e. how
// many are hidden from the current (non-root) user.
func countUnresolved(rows []app.Row) int {
	n := 0
	for _, r := range rows {
		if r.PID == 0 {
			n++
		}
	}
	return n
}

func unresolvedStatusHint(rows []app.Row) string {
	if isRoot() {
		return ""
	}
	n := countUnresolved(rows)
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("(%d hidden — try sudo)", n)
}
