package firewall

import (
	"os/exec"
	"strings"

	"github.com/padovanl/portop/internal/hostcmd"
)

// load reads ufw, then firewalld, then nftables. All three need root: run
// as another user they refuse, and the verdict stays unknown.
func load() Policy {
	if path, err := exec.LookPath("ufw"); err == nil {
		out, _ := hostcmd.Output(path, "status", "verbose")
		if p, ok := parseUFW(string(out)); ok {
			if _, inactive := p.(allowAll); !inactive {
				return p
			}
		}
	}
	if path, err := exec.LookPath("firewall-cmd"); err == nil {
		if state, _ := hostcmd.Output(path, "--state"); strings.TrimSpace(string(state)) == "running" {
			out, _ := hostcmd.Output(path, "--list-all")
			if p, ok := parseFirewalld(string(out), firewalldService); ok {
				return p
			}
		}
	}
	if path, err := exec.LookPath("nft"); err == nil {
		if out, err := hostcmd.Output(path, "list", "ruleset"); err == nil {
			return nftPolicy(string(out))
		}
	}
	return nil
}
