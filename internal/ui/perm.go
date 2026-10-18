package ui

import (
	"fmt"
	"os"
	"runtime"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/scanner"
)

// isRoot reports whether portop is likely able to see every process's
// /proc/<pid>/fd table. On Windows the permission model is different
// (and os.Geteuid is a stub returning -1 there), so we don't claim to
// know either way.
func isRoot() bool {
	return runtime.GOOS != "windows" && os.Geteuid() == 0
}

// closing reports whether a socket is in TIME_WAIT. The process that
// owned it has already closed it and the kernel keeps it alone for a while,
// so no privilege can attribute it to a process.
func closing(r app.Row) bool { return r.State == scanner.StateTimeWait }

// unresolvedHint explains why a row has no PID: almost always another
// user's process (docker-proxy, systemd-resolved, sshd before it drops
// privileges, ...) whose /proc/<pid>/fd table isn't readable without
// root — the same limitation lsof/ss -p have.
func unresolvedHint(row app.Row) string {
	if closing(row) {
		return "connection closing — no process owns it anymore"
	}
	if isRoot() {
		return "no process associated with this row"
	}
	if runtime.GOOS == "windows" {
		return "process unknown — try running portop from an Administrator terminal"
	}
	return "process unknown — likely owned by another user (try running portop with sudo)"
}

// countUnresolved returns how many rows have no PID that more privileges
// could resolve, i.e. how many are hidden from the current user.
func countUnresolved(rows []app.Row) int {
	n := 0
	for _, r := range rows {
		if r.PID == 0 && !closing(r) {
			n++
		}
	}
	return n
}

// unresolvedStatusHint counts the rows on screen, so a filter or the
// listen-only view never reports rows the user cannot see.
func unresolvedStatusHint(rows []app.Row) string {
	if isRoot() {
		return ""
	}
	n := countUnresolved(rows)
	if n == 0 {
		return ""
	}
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("(%d hidden — try an Administrator terminal)", n)
	}
	return fmt.Sprintf("(%d hidden — try sudo)", n)
}
