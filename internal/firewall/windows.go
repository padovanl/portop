package firewall

import (
	"bytes"
	"encoding/json"
)

// parseWindowsProfiles reads Get-NetFirewallProfile as JSON: one object or
// an array, with Enabled as a boolean or as 1/0.
func parseWindowsProfiles(out []byte) Policy {
	out = bytes.TrimSpace(out)
	if len(out) > 0 && out[0] == '{' {
		out = append(append([]byte{'['}, out...), ']')
	}
	var profiles []struct {
		Name    string
		Enabled json.RawMessage
	}
	if json.Unmarshal(out, &profiles) != nil || len(profiles) == 0 {
		return unknownPolicy{}
	}
	for _, p := range profiles {
		if v := string(bytes.TrimSpace(p.Enabled)); v != "false" && v != "0" {
			return unknownPolicy{}
		}
	}
	return allowAll{}
}
