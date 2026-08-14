//go:build !windows

// Package procctl sends termination signals to processes by PID, for
// portop's "k" (kill) action.
package procctl

import (
	"os"
	"syscall"
)

// Terminate asks the process to shut down gracefully (SIGTERM).
func Terminate(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

// Kill forcibly terminates the process (SIGKILL).
func Kill(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGKILL)
}
