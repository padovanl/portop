package firewall

import "github.com/padovanl/portop/internal/hostcmd"

// load reads the active Windows Firewall profiles. Their rules are mostly
// per program and slow to enumerate, so only a firewall turned off in
// every active profile gives a verdict.
func load() Policy {
	out, err := hostcmd.Output("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"Get-NetFirewallProfile -PolicyStore ActiveStore | Select-Object Name,Enabled | ConvertTo-Json -Compress")
	if err != nil {
		return nil
	}
	return parseWindowsProfiles(out)
}
