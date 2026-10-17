package firewall

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

type firewalldPolicy struct {
	acceptAll bool
	open      []portRange
	rich      bool // rich rules exist; a port they may open stays unknown
}

// FirewalldServiceDirs hold service definitions, user ones first.
var FirewalldServiceDirs = []string{"/etc/firewalld/services", "/usr/lib/firewalld/services"}

// parseFirewalld reads `firewall-cmd --list-all` for the default zone.
// Services are resolved to ports with lookupService.
func parseFirewalld(out string, lookupService func(string) []portRange) (Policy, bool) {
	p := &firewalldPolicy{}
	sawTarget := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "rule ") {
			p.rich = true // listed under "rich rules:", one per line
			continue
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "target":
			sawTarget = true
			p.acceptAll = value == "ACCEPT"
		case "services":
			for _, svc := range strings.Fields(value) {
				p.open = append(p.open, lookupService(svc)...)
			}
		case "ports":
			for _, spec := range strings.Fields(value) {
				if pr, ok := parsePorts(spec); ok {
					p.open = append(p.open, pr...)
				}
			}
		case "rich rules":
			p.rich = value != ""
		}
	}
	return p, sawTarget
}

func (p *firewalldPolicy) Verdict(proto string, port uint16) Verdict {
	if p.acceptAll {
		return Allowed
	}
	for _, pr := range p.open {
		if pr.covers(proto, port) {
			return Allowed
		}
	}
	if p.rich {
		return Unknown
	}
	return Blocked
}

// firewalldService reads a service definition's ports.
func firewalldService(name string) []portRange {
	for _, dir := range FirewalldServiceDirs {
		data, err := os.ReadFile(filepath.Join(dir, name+".xml"))
		if err != nil {
			continue
		}
		var svc struct {
			Ports []struct {
				Protocol string `xml:"protocol,attr"`
				Port     string `xml:"port,attr"`
			} `xml:"port"`
		}
		if xml.Unmarshal(data, &svc) != nil {
			return nil
		}
		var out []portRange
		for _, p := range svc.Ports {
			if pr, ok := parsePorts(p.Port + "/" + p.Protocol); ok {
				out = append(out, pr...)
			}
		}
		return out
	}
	return nil
}
