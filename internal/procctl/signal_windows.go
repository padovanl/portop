//go:build windows

// Package procctl sends termination signals to processes by PID, for
// portop's "k" (kill) action.
package procctl

import "os"

// Terminate stops the process. Windows has no portable graceful-shutdown
// signal equivalent to SIGTERM reachable from Go's os package, so this
// is the same hard kill as Kill.
func Terminate(pid int) error {
	return Kill(pid)
}

// Kill forcibly terminates the process.
func Kill(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
