package procctl

import (
	"errors"
	"time"

	"github.com/padovanl/portop/internal/procinfo"
)

// ErrProcessChanged is returned when the PID no longer refers to the process
// that was selected, or its identity cannot be confirmed.
var ErrProcessChanged = errors.New("process changed since it was selected; refresh before trying again")

// Hooks overridable in tests.
var (
	loadStart = func(pid int) (time.Time, error) {
		info, err := procinfo.Load(pid)
		return info.StartTime, err
	}
	sendTerminate = Terminate
	sendKill      = Kill
)

// StartTime returns the start time identifying pid, or an error when the
// process is gone or its start time cannot be read.
func StartTime(pid int) (time.Time, error) {
	start, err := loadStart(pid)
	if err != nil {
		return time.Time{}, err
	}
	if start.IsZero() {
		return time.Time{}, ErrProcessChanged
	}
	return start, nil
}

// SignalIfUnchanged re-reads pid's start time and signals it only if it still
// matches expected. A PID can be reused by an unrelated process between the
// moment a row is shown and the moment the user confirms, so the PID alone is
// not enough. It fails closed when the identity cannot be confirmed.
func SignalIfUnchanged(pid int, expected time.Time, force bool) error {
	if pid <= 0 || expected.IsZero() {
		return ErrProcessChanged
	}
	start, err := StartTime(pid)
	if err != nil {
		return err
	}
	if !start.Equal(expected) {
		return ErrProcessChanged
	}
	if force {
		return sendKill(pid)
	}
	return sendTerminate(pid)
}
