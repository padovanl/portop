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
	"github.com/charmbracelet/lipgloss"

	"github.com/padovan93/portop/internal/app"
	"github.com/padovan93/portop/internal/baseline"
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
  portop --save-baseline  remember which ports are currently listening
  portop --diff           compare live ports against the saved baseline
                           (exit code 3 if something changed — handy in
                           a cron job or systemd timer)

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
	saveBaseline := fs.Bool("save-baseline", false, "record which ports are currently listening")
	diffBaseline := fs.Bool("diff", false, "compare live listening ports against the saved baseline")
	baselinePath := fs.String("baseline-path", "", "baseline file path (default: OS config dir)/portop/baseline.json")

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

	path := *baselinePath
	if path == "" {
		if p, err := baseline.DefaultPath(); err == nil {
			path = p
		}
	}

	switch {
	case *saveBaseline:
		return runSaveBaseline(stdout, stderr, path, opts)
	case *diffBaseline:
		return runDiffBaseline(stdout, stderr, path, *jsonMode, opts)
	case *jsonMode:
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

func runSaveBaseline(stdout, stderr io.Writer, path string, opts app.Options) int {
	if path == "" {
		fmt.Fprintln(stderr, "portop: could not determine a baseline path (try --baseline-path)")
		return 1
	}
	collector := app.NewCollector()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := collector.Collect(ctx, opts)
	if err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}

	entries := baseline.FromRows(rows)
	if err := baseline.Save(path, entries); err != nil {
		fmt.Fprintln(stderr, "portop: saving baseline: "+err.Error())
		return 1
	}
	fmt.Fprintf(stdout, "saved baseline of %d listening port(s) to %s\n", len(entries), path)
	return 0
}

var (
	styleDiffAdded   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B4530A", Dark: "#FFB454"}).Bold(true)
	styleDiffRemoved = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8A93A6"})
)

// runDiffBaseline compares the live LISTEN ports against a previously
// saved baseline and reports what changed. It exits 3 if anything
// changed (added or removed), so it composes with cron/systemd timers
// and `&&`/`||` shell chains without extra parsing.
func runDiffBaseline(stdout, stderr io.Writer, path string, jsonOut bool, opts app.Options) int {
	if path == "" {
		fmt.Fprintln(stderr, "portop: could not determine a baseline path (try --baseline-path)")
		return 1
	}
	saved, savedAt, err := baseline.Load(path)
	if err != nil {
		fmt.Fprintln(stderr, "portop: no baseline found at "+path+" — run 'portop --save-baseline' first")
		return 1
	}

	collector := app.NewCollector()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := collector.Collect(ctx, opts)
	if err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}

	current := baseline.FromRows(rows)
	added, removed := baseline.Diff(saved, current)

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(struct {
			BaselineSavedAt string           `json:"baseline_saved_at"`
			Added           []baseline.Entry `json:"added"`
			Removed         []baseline.Entry `json:"removed"`
		}{BaselineSavedAt: savedAt.Format(time.RFC3339), Added: orEmpty(added), Removed: orEmpty(removed)})
	} else {
		fmt.Fprintf(stdout, "baseline saved %s (%d ports)\n", savedAt.Format("2006-01-02 15:04:05"), len(saved))
		if len(added) == 0 && len(removed) == 0 {
			fmt.Fprintln(stdout, "no changes — every listening port matches the baseline")
		}
		for _, e := range added {
			fmt.Fprintln(stdout, styleDiffAdded.Render(fmt.Sprintf("+ :%d %s (%s) — new, not in baseline", e.Port, e.Protocol, orDash(e.Process))))
		}
		for _, e := range removed {
			fmt.Fprintln(stdout, styleDiffRemoved.Render(fmt.Sprintf("- :%d %s (%s) — in baseline, not listening anymore", e.Port, e.Protocol, orDash(e.Process))))
		}
	}

	if len(added) > 0 || len(removed) > 0 {
		return 3
	}
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func orEmpty(e []baseline.Entry) []baseline.Entry {
	if e == nil {
		return []baseline.Entry{}
	}
	return e
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
