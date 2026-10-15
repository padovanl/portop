package docker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// psLine is one line of `docker ps --format "{{json .}}"`. Ports and
// Labels are pre-formatted strings in this output, not structures.
type psLine struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Labels string `json:"Labels"`
	Ports  string `json:"Ports"`
	State  string `json:"State"`
}

// parsePSOutput turns `docker ps` JSON lines into PublishedPorts, the same
// shape the Engine API path produces. It is used on Windows, where the
// daemon is reached through a named pipe the docker CLI already knows how
// to use (including non-default contexts).
func parsePSOutput(out []byte) []PublishedPort {
	var result []PublishedPort
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var line psLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		labels := parseLabels(line.Labels)
		name := line.Names
		if i := strings.IndexByte(name, ','); i >= 0 {
			name = name[:i]
		}
		for _, spec := range strings.Split(line.Ports, ",") {
			for _, p := range parsePortSpec(strings.TrimSpace(spec)) {
				p.ContainerID = line.ID
				p.ContainerName = name
				p.Service = labels["com.docker.compose.service"]
				p.Project = labels["com.docker.compose.project"]
				p.State = line.State
				result = append(result, p)
			}
		}
	}
	return result
}

func parseLabels(s string) map[string]string {
	labels := map[string]string{}
	for _, kv := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			labels[k] = v
		}
	}
	return labels
}

// parsePortSpec parses one published mapping as printed by docker ps:
// "0.0.0.0:8080->80/tcp", "[::]:8080->80/tcp", ":::8080->80/tcp" or a
// range "127.0.0.1:5000-5002->5000-5002/udp". Unpublished ports ("80/tcp")
// yield nothing.
func parsePortSpec(spec string) []PublishedPort {
	host, container, ok := strings.Cut(spec, "->")
	if !ok {
		return nil
	}
	container, proto, _ := strings.Cut(container, "/")
	idx := strings.LastIndexByte(host, ':')
	if idx < 0 {
		return nil
	}
	ip := strings.Trim(host[:idx], "[]")
	hostLo, hostHi, ok := portRange(host[idx+1:])
	if !ok {
		return nil
	}
	ctrLo, ctrHi, ok := portRange(container)
	if !ok || ctrHi-ctrLo != hostHi-hostLo {
		return nil
	}
	var out []PublishedPort
	for i := 0; i <= hostHi-hostLo; i++ {
		out = append(out, PublishedPort{
			HostIP:      ip,
			PublicPort:  uint16(hostLo + i),
			PrivatePort: uint16(ctrLo + i),
			Type:        strings.ToUpper(proto),
		})
	}
	return out
}

func portRange(s string) (lo, hi int, ok bool) {
	a, b, isRange := strings.Cut(s, "-")
	lo, err := strconv.Atoi(a)
	if err != nil || lo <= 0 || lo > 65535 {
		return 0, 0, false
	}
	hi = lo
	if isRange {
		hi, err = strconv.Atoi(b)
		if err != nil || hi < lo || hi > 65535 {
			return 0, 0, false
		}
	}
	return lo, hi, true
}
