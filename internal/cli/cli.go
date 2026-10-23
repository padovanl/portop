// Package cli parses flags and runs the TUI, web server, or snapshot commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/baseline"
	"github.com/padovanl/portop/internal/compose"
	"github.com/padovanl/portop/internal/config"
	"github.com/padovanl/portop/internal/procinfo"
	"github.com/padovanl/portop/internal/terminaltext"
	"github.com/padovanl/portop/internal/ui"
)

// Version is set at build time via -ldflags "-X .../cli.Version=...".
var Version = "dev"

const usage = `portop — what's really using your ports?

Usage:
  portop                 launch the TUI (shows LISTEN + ESTABLISHED)
  portop <port|name>     launch the TUI pre-filtered on a port or process
  portop --listen         show only listening sockets (TCP LISTEN, bound UDP)
  portop --json           print a JSON snapshot and exit
  portop --save-baseline  remember which ports are currently listening
  portop --diff           compare live ports against the saved baseline
                           (exit code 3 if something changed — handy in
                           a cron job or systemd timer)
  portop --compose DIR     audit published ports from a Docker Compose folder
  portop --wait 5432       wait until something listens on a port (exit 1
                           after --timeout, default 30s; 53/udp for UDP)
  portop --free 8080       stop whatever listens on a port and wait until
                           it is released (--force for SIGKILL, --yes to
                           skip the confirmation)
  portop --web             serve a local dashboard at http://127.0.0.1:8088
  portop --web --web-port 9090  choose the dashboard port
  portop --web --web-auth       require sign-in (password printed if not set)
  portop --web --web-user admin --web-password secret --web-addr 0.0.0.0:8088
                           sign in with your own credentials; reachable
                           from other machines (add --web-tls-cert/-key)
  portop --init-config     write a default config.yml and exit

config.yml (optional, see --init-config) sets default flag values, the
color theme, and keybinding overrides. See CONTRIBUTING or the README
for its format.

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

	listenOnly := fs.Bool("listen", false, "show only listening sockets (TCP LISTEN and bound UDP)")
	jsonMode := fs.Bool("json", false, "print a JSON snapshot and exit (non-interactive)")
	webMode := fs.Bool("web", false, "serve a local web dashboard")
	webPort := fs.Int("web-port", 8088, "web dashboard port (loopback only)")
	webAddr := fs.String("web-addr", "127.0.0.1:8088", "web dashboard listen address (loopback only unless --web-auth is set)")
	webAuthOn := fs.Bool("web-auth", false, "require a username and password to open the web dashboard")
	webUser := fs.String("web-user", "portop", "web dashboard username (implies --web-auth)")
	webPassword := fs.String("web-password", "", "web dashboard password (implies --web-auth; default: $PORTOP_WEB_PASSWORD, or generated and printed)")
	webTLSCert := fs.String("web-tls-cert", "", "serve the web dashboard over HTTPS with this PEM certificate")
	webTLSKey := fs.String("web-tls-key", "", "PEM private key for --web-tls-cert")
	noDNS := fs.Bool("no-dns", false, "disable reverse DNS lookups on ESTABLISHED connections")
	noSystemd := fs.Bool("no-systemd", false, "disable systemd unit association")
	noDocker := fs.Bool("no-docker", false, "disable Docker container association")
	noFirewall := fs.Bool("no-firewall", false, "do not read the host firewall for exposed ports")
	noTraffic := fs.Bool("no-traffic", false, "do not read per-connection traffic")
	noProjects := fs.Bool("no-projects", false, "do not detect the project and framework behind listening ports")
	watchNew := fs.Bool("watch-new", false, "send a desktop notification when a new listening port appears")
	interval := fs.Duration("interval", 2*time.Second, "TUI refresh interval")
	showVersion := fs.Bool("version", false, "print the version and exit")
	saveBaseline := fs.Bool("save-baseline", false, "record which ports are currently listening")
	diffBaseline := fs.Bool("diff", false, "compare live listening ports against the saved baseline")
	baselinePath := fs.String("baseline-path", "", "baseline file path (default: OS config dir)/portop/baseline.json")
	composeDir := fs.String("compose", "", "audit published ports from a Docker Compose project directory")
	configPath := fs.String("config", "", "config file path (default: OS config dir)/portop/config.yml")
	initConfig := fs.Bool("init-config", false, "write a default config.yml and exit")
	inspectPID := fs.Int("inspect-pid", 0, "print process details as JSON")
	signalPID := fs.Int("signal-pid", 0, "signal a process after checking its start time")
	expectedStart := fs.String("expected-start", "", "start time from --inspect-pid")
	forceSignal := fs.Bool("force", false, "use SIGKILL with --signal-pid or --free instead of SIGTERM")
	waitPort := fs.String("wait", "", "wait until something listens on PORT[/udp]")
	freePort := fs.String("free", "", "stop whatever listens on PORT[/udp]")
	timeout := fs.Duration("timeout", 0, "how long --wait (default 30s) or --free (default 5s) waits; 0 uses the default")
	assumeYes := fs.Bool("yes", false, "do not ask for confirmation with --free")

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

	cfgPath := *configPath
	if cfgPath == "" {
		if p, err := config.DefaultPath(); err == nil {
			cfgPath = p
		}
	}

	if *initConfig {
		if cfgPath == "" {
			fmt.Fprintln(stderr, "portop: could not determine a config path (try --config)")
			return 1
		}
		if err := config.WriteDefault(cfgPath); err != nil {
			fmt.Fprintln(stderr, "portop: writing config: "+err.Error())
			return 1
		}
		fmt.Fprintln(stdout, "wrote default config to "+cfgPath)
		return 0
	}

	fileCfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}

	// config.yml only supplies a *default* for a flag that wasn't
	// explicitly passed on the command line — flags always win.
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	if !explicit["listen"] && fileCfg.ShowEstablished != nil {
		*listenOnly = !*fileCfg.ShowEstablished
	}
	if !explicit["no-dns"] && fileCfg.ResolveDNS != nil && !*fileCfg.ResolveDNS {
		*noDNS = true
	}
	if !explicit["no-systemd"] && fileCfg.ResolveSystemd != nil && !*fileCfg.ResolveSystemd {
		*noSystemd = true
	}
	if !explicit["no-docker"] && fileCfg.ResolveDocker != nil && !*fileCfg.ResolveDocker {
		*noDocker = true
	}
	if !explicit["no-firewall"] && fileCfg.ResolveFirewall != nil && !*fileCfg.ResolveFirewall {
		*noFirewall = true
	}
	if !explicit["no-traffic"] && fileCfg.ResolveTraffic != nil && !*fileCfg.ResolveTraffic {
		*noTraffic = true
	}
	if !explicit["no-projects"] && fileCfg.ResolveProjects != nil && !*fileCfg.ResolveProjects {
		*noProjects = true
	}
	if !explicit["watch-new"] && fileCfg.WatchNew != nil {
		*watchNew = *fileCfg.WatchNew
	}
	if !explicit["interval"] && fileCfg.RefreshInterval != "" {
		d, err := time.ParseDuration(fileCfg.RefreshInterval)
		if err != nil {
			fmt.Fprintln(stderr, "portop: config refresh_interval: "+err.Error())
			return 1
		}
		*interval = d
	}

	themeName := "default"
	if fileCfg.Theme != "" {
		palette, ok := ui.Themes[fileCfg.Theme]
		if !ok {
			fmt.Fprintf(stderr, "portop: unknown theme %q in config (available: %s)\n", fileCfg.Theme, strings.Join(ui.ThemeNames, ", "))
			return 1
		}
		ui.ApplyPalette(palette)
		themeName = fileCfg.Theme
	}
	if len(fileCfg.Keybindings) > 0 {
		known := make(map[string]bool, len(ui.KeyActions))
		for _, a := range ui.KeyActions {
			known[a] = true
		}
		for action := range fileCfg.Keybindings {
			if !known[action] {
				fmt.Fprintf(stderr, "portop: unknown keybindings action %q in config (valid: %s)\n",
					action, strings.Join(ui.KeyActions, ", "))
				return 1
			}
		}
	}
	ui.ApplyKeyBindings(fileCfg.Keybindings)

	if *inspectPID != 0 {
		info, err := procinfo.Load(*inspectPID)
		if err != nil {
			fmt.Fprintln(stderr, "portop: "+err.Error())
			return 1
		}
		if err := json.NewEncoder(stdout).Encode(info); err != nil {
			fmt.Fprintln(stderr, "portop: "+err.Error())
			return 1
		}
		return 0
	}
	if *signalPID != 0 {
		if err := signalProcess(*signalPID, *expectedStart, *forceSignal); err != nil {
			fmt.Fprintln(stderr, "portop: "+err.Error())
			return 1
		}
		return 0
	}

	if *waitPort != "" {
		wait := *timeout
		if wait <= 0 {
			wait = 30 * time.Second
		}
		return runWait(stdout, stderr, *waitPort, wait, systemPortEnv())
	}
	if *freePort != "" {
		wait := *timeout
		if wait <= 0 {
			wait = 5 * time.Second
		}
		return runFree(stdout, stderr, *freePort, *forceSignal, *assumeYes, wait, systemPortEnv())
	}

	filter := strings.Join(positional, " ")

	opts := app.Options{
		ResolveSystemd:  !*noSystemd,
		ResolveDocker:   !*noDocker,
		ResolveDNS:      !*noDNS,
		ResolveFirewall: !*noFirewall,
		ResolveTraffic:  !*noTraffic,
		ResolveProjects: !*noProjects,
	}

	blPath := *baselinePath
	if blPath == "" {
		if p, err := baseline.DefaultPath(); err == nil {
			blPath = p
		}
	}

	switch {
	case *webMode:
		serve, err := webServeOptions(webFlags{
			addr: *webAddr, port: *webPort, addrSet: explicit["web-addr"], portSet: explicit["web-port"],
			auth: *webAuthOn, authSet: explicit["web-auth"],
			user: *webUser, userSet: explicit["web-user"],
			password: *webPassword, passwordSet: explicit["web-password"], passwordEnv: os.Getenv("PORTOP_WEB_PASSWORD"),
			tlsCert: *webTLSCert, tlsKey: *webTLSKey,
		})
		if err != nil {
			fmt.Fprintln(stderr, "portop: "+err.Error())
			return 2
		}
		return runWeb(stdout, stderr, serve, webConfig{Filter: filter, ListenOnly: *listenOnly, Options: opts})
	case *composeDir != "":
		return runComposeAudit(stdout, stderr, *composeDir, *jsonMode, opts)
	case *saveBaseline:
		return runSaveBaseline(stdout, stderr, blPath, opts)
	case *diffBaseline:
		return runDiffBaseline(stdout, stderr, blPath, *jsonMode, opts)
	case *jsonMode:
		return runJSON(stdout, stderr, filter, *listenOnly, opts)
	}

	uiCfg := ui.Config{
		InitialFilter:   filter,
		ShowEstablished: !*listenOnly,
		RefreshInterval: *interval,
		ResolveDNS:      opts.ResolveDNS,
		ResolveSystemd:  opts.ResolveSystemd,
		ResolveDocker:   opts.ResolveDocker,
		ResolveFirewall: opts.ResolveFirewall,
		ResolveTraffic:  opts.ResolveTraffic,
		ResolveProjects: opts.ResolveProjects,
		NotifyOnNewPort: *watchNew,
		ConfigPath:      cfgPath,
		Theme:           themeName,
		KeyOverrides:    fileCfg.Keybindings,
	}

	p := tea.NewProgram(ui.New(uiCfg), tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}
	return 0
}

func collectRows(opts app.Options) ([]app.Row, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return app.NewCollector().Collect(ctx, opts)
}

func runJSON(stdout, stderr io.Writer, filter string, listenOnly bool, opts app.Options) int {
	rows, err := collectRows(opts)
	if err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}

	out := make([]jsonRow, 0, len(rows))
	for _, r := range rows {
		if listenOnly && !r.Listening() {
			continue
		}
		if !r.Matches(filter) {
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
	rows, err := collectRows(opts)
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

// runDiffBaseline compares the live listening ports against a previously
// saved baseline and reports what changed. It exits 3 if anything
// changed (added or removed), so it composes with cron/systemd timers
// and `&&`/`||` shell chains without extra parsing.
func runDiffBaseline(stdout, stderr io.Writer, path string, jsonOut bool, opts app.Options) int {
	if path == "" {
		fmt.Fprintln(stderr, "portop: could not determine a baseline path (try --baseline-path)")
		return 1
	}
	saved, savedAt, err := baseline.Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintln(stderr, "portop: no baseline found at "+path+" — run 'portop --save-baseline' first")
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, terminaltext.Sanitize("portop: reading baseline: "+err.Error()))
		return 1
	}

	rows, err := collectRows(opts)
	if err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}

	current := baseline.FromRows(rows)
	added, removed := baseline.Diff(saved, current)

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(struct {
			BaselineSavedAt string           `json:"baseline_saved_at"`
			Added           []baseline.Entry `json:"added"`
			Removed         []baseline.Entry `json:"removed"`
		}{BaselineSavedAt: savedAt.Format(time.RFC3339), Added: orEmpty(added), Removed: orEmpty(removed)}); err != nil {
			fmt.Fprintln(stderr, "portop: "+err.Error())
			return 1
		}
	} else {
		fmt.Fprintf(stdout, "baseline saved %s (%d ports)\n", savedAt.Format("2006-01-02 15:04:05"), len(saved))
		if len(added) == 0 && len(removed) == 0 {
			fmt.Fprintln(stdout, "no changes — every listening port matches the baseline")
		}
		for _, e := range added {
			fmt.Fprintln(stdout, styleDiffAdded.Render(fmt.Sprintf("+ :%d %s%s (%s) — new, not in baseline", e.Port, e.Protocol, scopeLabel(e.Scope), terminaltext.Sanitize(orDash(e.Process)))))
		}
		for _, e := range removed {
			fmt.Fprintln(stdout, styleDiffRemoved.Render(fmt.Sprintf("- :%d %s%s (%s) — in baseline, not listening anymore", e.Port, e.Protocol, scopeLabel(e.Scope), terminaltext.Sanitize(orDash(e.Process)))))
		}
	}

	if len(added) > 0 || len(removed) > 0 {
		return 3
	}
	return 0
}

// scopeLabel says who can reach a baseline entry's port.
func scopeLabel(scope string) string {
	switch scope {
	case baseline.ScopeLoopback:
		return " on loopback"
	case baseline.ScopeAll:
		return " on all interfaces"
	case baseline.ScopeAddress:
		return " on one address"
	}
	return ""
}

func runComposeAudit(stdout, stderr io.Writer, dir string, jsonOut bool, opts app.Options) int {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summary, err := compose.AuditDir(ctx, dir, compose.AuditOptions{ResolveDocker: opts.ResolveDocker})
	if err != nil {
		fmt.Fprintln(stderr, terminaltext.Sanitize("portop: compose audit: "+err.Error()))
		return 1
	}

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			fmt.Fprintln(stderr, "portop: "+err.Error())
			return 1
		}
	} else {
		printComposeSummary(stdout, summary)
	}

	for _, r := range summary.Results {
		if r.Status == compose.StatusUnused || r.Status == compose.StatusConflict {
			return 3
		}
	}
	return 0
}

func printComposeSummary(w io.Writer, summary compose.Summary) {
	fmt.Fprintf(w, "Compose folder: %s\n", terminaltext.Sanitize(summary.Directory))
	fmt.Fprintf(w, "Files: %s\n\n", terminaltext.Sanitize(strings.Join(summary.Files, ", ")))
	fmt.Fprintf(w, "%-10s %-18s %-12s %-22s %s\n", "STATUS", "SERVICE", "PORT", "OWNER", "NOTE")
	for _, r := range summary.Results {
		port := "dynamic"
		if !r.Dynamic {
			target := ""
			if r.Target != 0 {
				target = fmt.Sprintf("->%d", r.Target)
			}
			host := ""
			if r.HostIP != "" {
				host = r.HostIP + ":"
			}
			port = fmt.Sprintf("%s%s/%s%s", host, strconv.Itoa(int(r.Published)), strings.ToLower(r.Protocol), target)
		}
		owner := composeOwner(r)
		fmt.Fprintf(w, "%-10s %-18s %-12s %-22s %s\n",
			terminaltext.Sanitize(string(r.Status)), terminaltext.Sanitize(r.Service), terminaltext.Sanitize(port),
			terminaltext.Sanitize(owner), terminaltext.Sanitize(r.Note))
	}
}

func composeOwner(r compose.Result) string {
	if r.OwnerService != "" {
		if r.OwnerContainer != "" {
			return r.OwnerService + " (" + r.OwnerContainer + ")"
		}
		return r.OwnerService
	}
	if r.OwnerContainer != "" {
		return r.OwnerContainer
	}
	if r.OwnerProcess != "" {
		if r.OwnerPID != 0 {
			return fmt.Sprintf("%s pid=%d", r.OwnerProcess, r.OwnerPID)
		}
		return r.OwnerProcess
	}
	return "-"
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

type jsonRow struct {
	Protocol      string   `json:"protocol"`
	LocalAddress  string   `json:"local_address"`
	LocalPort     uint16   `json:"local_port"`
	RemoteAddress string   `json:"remote_address,omitempty"`
	RemotePort    uint16   `json:"remote_port,omitempty"`
	RemoteHost    string   `json:"remote_host,omitempty"`
	State         string   `json:"state"`
	IPv6          bool     `json:"ipv6"`
	PID           int      `json:"pid,omitempty"`
	Process       string   `json:"process,omitempty"`
	User          string   `json:"user,omitempty"`
	Workers       []int    `json:"workers,omitempty"`
	Firewall      string   `json:"firewall,omitempty"`
	RXBytes       *uint64  `json:"rx_bytes,omitempty"`
	TXBytes       *uint64  `json:"tx_bytes,omitempty"`
	RXRate        *float64 `json:"rx_bytes_per_second,omitempty"`
	TXRate        *float64 `json:"tx_bytes_per_second,omitempty"`
	Scope         string   `json:"scope"`
	CPUPercent    float64  `json:"cpu_percent"`
	SystemdUnit   string   `json:"systemd_unit,omitempty"`
	Container     string   `json:"container,omitempty"`
	Project       string   `json:"project,omitempty"`
	ProjectRoot   string   `json:"project_root,omitempty"`
	Language      string   `json:"language,omitempty"`
	Framework     string   `json:"framework,omitempty"`
	Category      string   `json:"category,omitempty"`
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
		User:         r.Username,
		Workers:      r.Workers,
		Firewall:     r.Firewall,
		Scope:        r.Scope(),
		CPUPercent:   r.CPUPercent,
		SystemdUnit:  r.SystemdUnit,
		Container:    r.ContainerName,
		Project:      r.Project.Name,
		ProjectRoot:  r.Project.Root,
		Language:     r.Project.Language,
		Framework:    r.Project.Framework,
		Category:     r.Category,
	}
	if r.TrafficKnown {
		jr.RXBytes, jr.TXBytes = &r.RXBytes, &r.TXBytes
	}
	if r.RateKnown {
		jr.RXRate, jr.TXRate = &r.RXRate, &r.TXRate
	}
	if r.RemotePort != 0 {
		jr.RemoteAddress = r.RemoteAddr.String()
		jr.RemotePort = r.RemotePort
		jr.RemoteHost = r.RemoteHost
	}
	return jr
}
