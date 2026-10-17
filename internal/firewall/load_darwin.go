package firewall

import "github.com/padovanl/portop/internal/hostcmd"

// load reads the macOS application firewall. It decides per application
// rather than per port, so only its global states give a verdict.
func load() Policy {
	out, err := hostcmd.Output("/usr/libexec/ApplicationFirewall/socketfilterfw", "--getglobalstate")
	if err != nil {
		return nil
	}
	return parseSocketFilterState(string(out))
}
