package cli

import (
	"fmt"
	"time"

	"github.com/padovanl/portop/internal/procctl"
)

// signalProcess checks the process start time before sending a signal. A PID
// can be reused after the row was displayed, so the PID alone is not enough.
func signalProcess(pid int, expectedStart string, force bool) error {
	if pid <= 0 || expectedStart == "" {
		return fmt.Errorf("a PID and process start time are required")
	}
	expected, err := time.Parse(time.RFC3339Nano, expectedStart)
	if err != nil {
		return fmt.Errorf("invalid process start time")
	}
	return procctl.SignalIfUnchanged(pid, expected, force)
}
