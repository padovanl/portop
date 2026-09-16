// Package compose audits Docker Compose published ports against the
// sockets currently listening on the host.
package compose

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/docker"
	"github.com/padovanl/portop/internal/scanner"
	"gopkg.in/yaml.v3"
)

var defaultFiles = []string{
	"compose.yaml",
	"compose.yml",
	"compose.override.yaml",
	"compose.override.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
	"docker-compose.override.yaml",
	"docker-compose.override.yml",
}

type ConfiguredPort struct {
	Service       string `json:"service"`
	ContainerName string `json:"container_name,omitempty"`
	HostIP        string `json:"host_ip,omitempty"`
	Published     uint16 `json:"published,omitempty"`
	Target        uint16 `json:"target,omitempty"`
	Protocol      string `json:"protocol"`
	Dynamic       bool   `json:"dynamic,omitempty"`
	File          string `json:"file,omitempty"`
	Raw           string `json:"raw,omitempty"`
}

type Status string

const (
	StatusExpected Status = "expected"
	StatusConflict Status = "conflict"
	StatusUnused   Status = "unused"
	StatusDynamic  Status = "dynamic"
)

type Result struct {
	ConfiguredPort
	Status            Status `json:"status"`
	OwnerProcess      string `json:"owner_process,omitempty"`
	OwnerPID          int    `json:"owner_pid,omitempty"`
	OwnerContainer    string `json:"owner_container,omitempty"`
	OwnerService      string `json:"owner_service,omitempty"`
	OwnerProject      string `json:"owner_project,omitempty"`
	OwnerSource       string `json:"owner_source,omitempty"`
	ExpectedContainer bool   `json:"expected_container"`
	Note              string `json:"note,omitempty"`
}

type Summary struct {
	Directory string   `json:"directory"`
	Files     []string `json:"files"`
	Results   []Result `json:"results"`
}

type composeFile struct {
	Services map[string]service `yaml:"services"`
}

type service struct {
	ContainerName string     `yaml:"container_name"`
	Ports         []portSpec `yaml:"ports"`
}

type portSpec struct {
	HostIP    string
	Published string
	Target    string
	Protocol  string
	Raw       string
}

func (p *portSpec) UnmarshalYAML(value *yaml.Node) error {
	p.Protocol = "tcp"
	switch value.Kind {
	case yaml.ScalarNode:
		p.Raw = strings.TrimSpace(value.Value)
		hostIP, published, target, proto := parseShortPort(p.Raw)
		p.HostIP = hostIP
		p.Published = published
		p.Target = target
		p.Protocol = proto
	case yaml.MappingNode:
		type longSyntax struct {
			Target    any    `yaml:"target"`
			Published any    `yaml:"published"`
			HostIP    string `yaml:"host_ip"`
			Protocol  string `yaml:"protocol"`
		}
		var l longSyntax
		if err := value.Decode(&l); err != nil {
			return err
		}
		p.HostIP = strings.TrimSpace(l.HostIP)
		p.Published = scalarString(l.Published)
		p.Target = scalarString(l.Target)
		if strings.TrimSpace(l.Protocol) != "" {
			p.Protocol = strings.ToLower(strings.TrimSpace(l.Protocol))
		}
	default:
		return fmt.Errorf("unsupported ports entry at line %d", value.Line)
	}
	return nil
}

func scalarString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case string:
		return strings.TrimSpace(x)
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func parseShortPort(raw string) (hostIP, published, target, proto string) {
	proto = "tcp"
	body := raw
	if before, after, ok := strings.Cut(raw, "/"); ok {
		body = before
		if strings.TrimSpace(after) != "" {
			proto = strings.ToLower(strings.TrimSpace(after))
		}
	}

	parts := splitPortParts(body)
	switch len(parts) {
	case 1:
		target = parts[0]
	case 2:
		published = parts[0]
		target = parts[1]
	default:
		hostIP = parts[0]
		published = parts[1]
		target = parts[2]
	}
	return strings.Trim(hostIP, "[]"), strings.TrimSpace(published), strings.TrimSpace(target), proto
}

func splitPortParts(s string) []string {
	var parts []string
	var b strings.Builder
	inIPv6 := false
	for _, r := range s {
		switch r {
		case '[':
			inIPv6 = true
			b.WriteRune(r)
		case ']':
			inIPv6 = false
			b.WriteRune(r)
		case ':':
			if inIPv6 {
				b.WriteRune(r)
			} else {
				parts = append(parts, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	parts = append(parts, b.String())
	if len(parts) > 3 {
		return parts[len(parts)-3:]
	}
	return parts
}

func LoadDir(dir string) (Summary, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return Summary{}, err
	}
	if !info.IsDir() {
		return Summary{}, fmt.Errorf("%s is not a directory", dir)
	}

	var files []string
	var ports []ConfiguredPort
	for _, name := range defaultFiles {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Summary{}, err
		}
		var cf composeFile
		if err := yaml.Unmarshal(data, &cf); err != nil {
			return Summary{}, fmt.Errorf("%s: %w", name, err)
		}
		files = append(files, name)
		for svcName, svc := range cf.Services {
			for _, ps := range svc.Ports {
				expanded, err := expandPortSpec(svcName, svc.ContainerName, name, ps)
				if err != nil {
					return Summary{}, fmt.Errorf("%s service %s port %q: %w", name, svcName, ps.Raw, err)
				}
				ports = append(ports, expanded...)
			}
		}
	}
	if len(files) == 0 {
		return Summary{}, errors.New("no compose.yaml, compose.yml or docker-compose.yml found")
	}

	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Service != ports[j].Service {
			return ports[i].Service < ports[j].Service
		}
		if ports[i].Protocol != ports[j].Protocol {
			return ports[i].Protocol < ports[j].Protocol
		}
		return ports[i].Published < ports[j].Published
	})
	return Summary{Directory: dir, Files: files, Results: portsToResults(ports)}, nil
}

func expandPortSpec(serviceName, containerName, file string, ps portSpec) ([]ConfiguredPort, error) {
	proto := strings.ToLower(strings.TrimSpace(ps.Protocol))
	if proto == "" {
		proto = "tcp"
	}
	if proto != "tcp" && proto != "udp" {
		return nil, fmt.Errorf("unsupported protocol %q", proto)
	}

	targets, err := expandRange(ps.Target)
	if err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}
	published, err := expandRange(ps.Published)
	if err != nil {
		return nil, fmt.Errorf("published: %w", err)
	}
	if len(targets) == 0 {
		return nil, errors.New("missing target port")
	}
	if len(published) > 1 && len(targets) > 1 && len(published) != len(targets) {
		return nil, fmt.Errorf("published and target ranges must have the same length")
	}

	n := len(targets)
	if len(published) > n {
		n = len(published)
	}
	out := make([]ConfiguredPort, 0, n)
	for i := 0; i < n; i++ {
		target := targets[min(i, len(targets)-1)]
		publishedPort := uint16(0)
		dynamic := len(published) == 0
		if !dynamic {
			publishedPort = published[min(i, len(published)-1)]
		}
		out = append(out, ConfiguredPort{
			Service:       serviceName,
			ContainerName: strings.TrimSpace(containerName),
			HostIP:        strings.Trim(ps.HostIP, "[]"),
			Published:     publishedPort,
			Target:        target,
			Protocol:      strings.ToUpper(proto),
			Dynamic:       dynamic,
			File:          file,
			Raw:           ps.Raw,
		})
	}
	return out, nil
}

func expandRange(s string) ([]uint16, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	startS, endS, hasRange := strings.Cut(s, "-")
	start, err := parsePort(startS)
	if err != nil {
		return nil, err
	}
	if !hasRange {
		return []uint16{start}, nil
	}
	end, err := parsePort(endS)
	if err != nil {
		return nil, err
	}
	if end < start {
		return nil, fmt.Errorf("range %d-%d is descending", start, end)
	}
	out := make([]uint16, 0, int(end-start)+1)
	for p := start; p <= end; p++ {
		out = append(out, p)
	}
	return out, nil
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	if n <= 0 || n > 65535 {
		return 0, fmt.Errorf("port %d out of range", n)
	}
	return uint16(n), nil
}

func portsToResults(ports []ConfiguredPort) []Result {
	results := make([]Result, 0, len(ports))
	for _, p := range ports {
		status := StatusUnused
		note := ""
		if p.Dynamic {
			status = StatusDynamic
			note = "Compose will choose a host port at runtime; no fixed port to check."
		}
		results = append(results, Result{ConfiguredPort: p, Status: status, Note: note})
	}
	return results
}

type AuditOptions struct {
	ResolveDocker bool
}

func AuditDir(ctx context.Context, dir string, opts AuditOptions) (Summary, error) {
	summary, err := LoadDir(dir)
	if err != nil {
		return Summary{}, err
	}

	collector := app.NewCollector()
	rows, err := collector.Collect(ctx, app.Options{ResolveDocker: opts.ResolveDocker})
	if err != nil {
		return Summary{}, err
	}

	var published []docker.PublishedPort
	if opts.ResolveDocker {
		client := docker.NewClient()
		if client.Available() {
			published = client.PublishedPorts(ctx)
		}
	}

	summary.Results = Audit(summary.Results, rows, published)
	return summary, nil
}

func Audit(results []Result, rows []app.Row, published []docker.PublishedPort) []Result {
	out := make([]Result, len(results))
	copy(out, results)
	for i := range out {
		if out[i].Dynamic {
			continue
		}
		if p := findPublished(out[i].ConfiguredPort, published); p != nil {
			fillFromDocker(&out[i], *p)
			continue
		}
		if row := findListener(out[i].ConfiguredPort, rows); row != nil {
			fillFromRow(&out[i], *row)
			continue
		}
		out[i].Status = StatusUnused
		out[i].Note = "No matching listener found on this host."
	}
	return out
}

func findPublished(port ConfiguredPort, published []docker.PublishedPort) *docker.PublishedPort {
	for i := range published {
		p := published[i]
		if p.State != "" && p.State != "running" {
			continue
		}
		if p.PublicPort != port.Published || strings.ToUpper(p.Type) != port.Protocol {
			continue
		}
		if !hostMatches(port.HostIP, p.HostIP) {
			continue
		}
		return &published[i]
	}
	return nil
}

func findListener(port ConfiguredPort, rows []app.Row) *app.Row {
	proto := scanner.Protocol(port.Protocol)
	state := scanner.StateListen
	if proto == scanner.UDP {
		state = scanner.StateUnconn
	}
	for i := range rows {
		r := rows[i]
		if r.Protocol != proto || r.LocalPort != port.Published || r.State != state {
			continue
		}
		if !hostMatches(port.HostIP, r.LocalAddr.String()) {
			continue
		}
		return &rows[i]
	}
	return nil
}

func fillFromDocker(result *Result, published docker.PublishedPort) {
	result.OwnerContainer = published.ContainerName
	result.OwnerService = published.Service
	result.OwnerProject = published.Project
	result.OwnerSource = "docker"
	result.ExpectedContainer = ownerMatches(*result, published.Service, published.ContainerName)
	if result.ExpectedContainer {
		result.Status = StatusExpected
		result.Note = "Docker reports this published port on the expected Compose service."
	} else {
		result.Status = StatusConflict
		result.Note = "Docker reports this port on a different container or Compose service."
	}
}

func fillFromRow(result *Result, row app.Row) {
	result.OwnerProcess = row.ProcessName
	result.OwnerPID = row.PID
	result.OwnerContainer = row.ContainerName
	result.OwnerSource = "socket"
	result.ExpectedContainer = ownerMatches(*result, "", row.ContainerName)
	if result.ExpectedContainer {
		result.Status = StatusExpected
		result.Note = "A listener matches the expected container name."
	} else {
		result.Status = StatusConflict
		result.Note = "A listener is using this port, but Docker did not identify it as the expected Compose service."
	}
}

func ownerMatches(result Result, service, container string) bool {
	if service != "" && service == result.Service {
		return true
	}
	if result.ContainerName != "" && container == result.ContainerName {
		return true
	}
	if container != "" && (container == result.Service || strings.Contains(container, "_"+result.Service+"_") || strings.Contains(container, "-"+result.Service+"-")) {
		return true
	}
	return false
}

func hostMatches(configHost, ownerHost string) bool {
	if configHost == "" || configHost == "0.0.0.0" || configHost == "::" {
		return true
	}
	if ownerHost == "" || ownerHost == "0.0.0.0" || ownerHost == "::" {
		return true
	}
	configIP := net.ParseIP(configHost)
	ownerIP := net.ParseIP(ownerHost)
	if configIP == nil || ownerIP == nil {
		return configHost == ownerHost
	}
	return configIP.Equal(ownerIP)
}
