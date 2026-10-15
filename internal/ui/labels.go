package ui

import "runtime"

// uiLabels holds the user-visible wording that differs by platform. The
// column keys ("SYSTEMD") and sort modes stay the same everywhere; only
// what is shown changes, so Linux and macOS output is untouched.
type uiLabels struct {
	unitColumn string // header of the systemd/service column
	unitField  string // label in the process detail view
	unitSort   string // sort mode name in the status bar
	terminate  string // "y" in the kill confirmation
	force      string // "f" in the kill confirmation
	killHelp   string // help overlay entry for the kill key
}

var platformLabels = labelsFor(runtime.GOOS)

func labelsFor(goos string) uiLabels {
	if goos == "windows" {
		// Windows has no SIGTERM: both actions end the process
		// (TerminateProcess), see internal/procctl.
		return uiLabels{
			unitColumn: "SERVICE",
			unitField:  "Service:",
			unitSort:   "service",
			terminate:  "end process",
			force:      "end process (force)",
			killHelp:   "end process (then y or f)",
		}
	}
	return uiLabels{
		unitColumn: "SYSTEMD",
		unitField:  "Systemd:",
		unitSort:   "systemd",
		terminate:  "terminate (SIGTERM)",
		force:      "force (SIGKILL)",
		killHelp:   "kill process (then y=SIGTERM, f=SIGKILL)",
	}
}

// headerTitle maps an internal column key to the header text shown.
func headerTitle(key string) string {
	if key == "SYSTEMD" {
		return platformLabels.unitColumn
	}
	return key
}
