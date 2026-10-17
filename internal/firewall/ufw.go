package firewall

import "strings"

// ufwRule is one line of `ufw status verbose`, in evaluation order.
type ufwRule struct {
	ports   []portRange
	action  string // ALLOW, DENY, REJECT or LIMIT
	anyFrom bool   // the rule applies to every source
}

type ufwPolicy struct {
	defaultIncoming string // allow, deny or reject
	rules           []ufwRule
}

// parseUFW reads `ufw status verbose`. ok is false when the output is not
// a status report (ufw missing, or run without root).
func parseUFW(out string) (Policy, bool) {
	lines := strings.Split(out, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "Status:") {
		return nil, false
	}
	if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "Status:")) != "active" {
		return allowAll{}, true
	}
	p := &ufwPolicy{defaultIncoming: "deny"}
	inRules := false
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Default:"):
			// Default: deny (incoming), allow (outgoing), disabled (routed)
			for _, part := range strings.Split(strings.TrimPrefix(line, "Default:"), ",") {
				if f := strings.Fields(part); len(f) == 2 && f[1] == "(incoming)" {
					p.defaultIncoming = f[0]
				}
			}
		case strings.HasPrefix(line, "--"):
			inRules = true
		case inRules && line != "":
			if r, ok := parseUFWRule(line); ok {
				p.rules = append(p.rules, r)
			}
		}
	}
	return p, true
}

// parseUFWRule reads "80,443/tcp ALLOW IN Anywhere" and its variants with
// a destination address ("10.0.0.5 22/tcp"), an interface ("on eth0") or
// a "(v6)" marker. Rules naming an application profile are skipped.
func parseUFWRule(line string) (ufwRule, bool) {
	fields := strings.Fields(line)
	actionAt := -1
	for i, f := range fields {
		switch f {
		case "ALLOW", "DENY", "REJECT", "LIMIT":
			actionAt = i
		}
		if actionAt >= 0 {
			break
		}
	}
	if actionAt < 1 {
		return ufwRule{}, false
	}
	var ports []portRange
	for _, f := range fields[:actionAt] {
		if pr, ok := parsePorts(f); ok {
			ports = pr
			break
		}
	}
	if ports == nil {
		return ufwRule{}, false
	}
	rest := fields[actionAt+1:]
	if len(rest) > 0 && (rest[0] == "IN" || rest[0] == "OUT" || rest[0] == "FWD") {
		if rest[0] != "IN" {
			return ufwRule{}, false
		}
		rest = rest[1:]
	}
	return ufwRule{ports: ports, action: fields[actionAt], anyFrom: len(rest) > 0 && rest[0] == "Anywhere"}, true
}

func (p *ufwPolicy) Verdict(proto string, port uint16) Verdict {
	for _, r := range p.rules {
		for _, pr := range r.ports {
			if !pr.covers(proto, port) {
				continue
			}
			switch r.action {
			case "ALLOW", "LIMIT":
				if r.anyFrom {
					return Allowed
				}
				return Restricted
			default:
				if r.anyFrom {
					return Blocked
				}
				// A deny for some sources says nothing about the rest:
				// keep looking, then fall back to the default.
			}
		}
	}
	if p.defaultIncoming == "allow" {
		return Allowed
	}
	return Blocked
}
