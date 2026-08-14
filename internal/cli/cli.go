// Package cli parses portop's command-line interface and dispatches to
// either the interactive TUI or the non-interactive --json snapshot
// mode. It is kept separate from cmd/portop/main.go so it can be
// exercised by tests without spawning a real process.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/padovan93/portop/internal/app"
	"github.com/padovan93/portop/internal/scanner"
	"github.com/padovan93/portop/internal/ui"
)

// Version is set at build time via -ldflags "-X .../cli.Version=...".
var Version = "dev"

const usage = `portop — what's really using your ports?

Usage:
  portop                 launch the TUI (shows LISTEN + ESTABLISHED)
  portop <port|name>     launch the TUI pre-filtered on a port or process
  portop --listen         show only listening (LISTEN) sockets
  portop --json           print a JSON snapshot and exit

Options:
`

// Run parses args (as in os.Args[1:]) and executes portop, writing any
// non-interactive output to stdout/stderr. It returns the process exit
// code.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("portop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}

	listenOnly := fs.Bool("listen", false, "show only sockets in LISTEN state")
	jsonMode := fs.Bool("json", false, "print a JSON snapshot and exit (non-interactive)")
	noDNS := fs.Bool("no-dns", false, "disable reverse DNS lookups on ESTABLISHED connections")
	noSystemd := fs.Bool("no-systemd", false, "disable systemd unit association")
	noDocker := fs.Bool("no-docker", false, "disable Docker container association")
	watchNew := fs.Bool("watch-new", false, "send a desktop notification when a new LISTEN port appears")
	interval := fs.Duration("interval", 2*time.Second, "TUI refresh interval")
	showVersion := fs.Bool("version", false, "print the version and exit")

	// The stdlib flag package stops parsing at the first non-flag
	// argument, which would break "portop 8080 --json" (flag package
	// would treat "--json" as a second positional argument instead of
	// a flag). Loop so flags and positional arguments (the port/process
	// filter) can appear in any order.
	var positional []string
	remaining := args
	for {
		if err := fs.Parse(remaining); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return 2
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		remaining = rest[1:]
	}

	if *showVersion {
		fmt.Fprintln(stdout, "portop "+Version)
		return 0
	}

	filter := strings.Join(positional, " ")

	opts := app.Options{
		ResolveSystemd: !*noSystemd,
		ResolveDocker:  !*noDocker,
		ResolveDNS:     !*noDNS,
	}

	if *jsonMode {
		return runJSON(stdout, stderr, filter, *listenOnly, opts)
	}

	cfg := ui.Config{
		InitialFilter:   filter,
		ShowEstablished: !*listenOnly,
		RefreshInterval: *interval,
		ResolveDNS:      opts.ResolveDNS,
		ResolveSystemd:  opts.ResolveSystemd,
		ResolveDocker:   opts.ResolveDocker,
		NotifyOnNewPort: *watchNew,
	}

	p := tea.NewProgram(ui.New(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}
	return 0
}

func runJSON(stdout, stderr io.Writer, filter string, listenOnly bool, opts app.Options) int {
	collector := app.NewCollector()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := collector.Collect(ctx, opts)
	if err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}

	out := make([]jsonRow, 0, len(rows))
	for _, r := range rows {
		if listenOnly && r.State != scanner.StateListen {
			continue
		}
		if filter != "" && !matchesFilter(r, filter) {
			continue
		}
		out = append(out, toJSONRow(r))
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}
	return 0
}

func matchesFilter(r app.Row, filter string) bool {
	filter = strings.ToLower(filter)
	if strings.Contains(strconv.Itoa(int(r.LocalPort)), filter) {
		return true
	}
	if strings.Contains(strings.ToLower(r.ProcessName), filter) {
		return true
	}
	if strings.Contains(strconv.Itoa(r.PID), filter) {
		return true
	}
	return false
}

type jsonRow struct {
	Protocol      string  `json:"protocol"`
	LocalAddress  string  `json:"local_address"`
	LocalPort     uint16  `json:"local_port"`
	RemoteAddress string  `json:"remote_address,omitempty"`
	RemotePort    uint16  `json:"remote_port,omitempty"`
	RemoteHost    string  `json:"remote_host,omitempty"`
	State         string  `json:"state"`
	IPv6          bool    `json:"ipv6"`
	PID           int     `json:"pid,omitempty"`
	Process       string  `json:"process,omitempty"`
	CPUPercent    float64 `json:"cpu_percent"`
	SystemdUnit   string  `json:"systemd_unit,omitempty"`
	Container     string  `json:"container,omitempty"`
}

func toJSONRow(r app.Row) jsonRow {
	jr := jsonRow{
		Protocol:     string(r.Protocol),
		LocalAddress: r.LocalAddr.String(),
		LocalPort:    r.LocalPort,
		State:        string(r.State),
		IPv6:         r.IPv6,
		PID:          r.PID,
		Process:      r.ProcessName,
		CPUPercent:   r.CPUPercent,
		SystemdUnit:  r.SystemdUnit,
		Container:    r.ContainerName,
	}
	if r.RemotePort != 0 {
		jr.RemoteAddress = r.RemoteAddr.String()
		jr.RemotePort = r.RemotePort
		jr.RemoteHost = r.RemoteHost
	}
	return jr
}
