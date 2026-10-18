package firewall

import "strings"

// parseSocketFilterState reads `socketfilterfw --getglobalstate`:
// "Firewall is disabled. (State = 0)", "... enabled. (State = 1)" or
// "... blocking all non-essential services. (State = 2)".
func parseSocketFilterState(out string) Policy {
	switch {
	case strings.Contains(out, "State = 0"):
		return allowAll{}
	case strings.Contains(out, "State = 2"):
		return blockAll{}
	default:
		return unknownPolicy{}
	}
}
