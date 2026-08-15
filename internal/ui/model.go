// Package ui implements portop's Bubble Tea terminal UI: a live,
// sortable, filterable table of TCP/UDP sockets enriched with process,
// systemd and Docker context.
package ui

import (
	"context"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/notify"
	"github.com/padovanl/portop/internal/procctl"
	"github.com/padovanl/portop/internal/procinfo"
)

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeConfirmKill
	modeDetail
	modeHelp
)

type ipFilterMode int

const (
	ipAll ipFilterMode = iota
	ipv4Only
	ipv6Only
)

func (m ipFilterMode) String() string {
	switch m {
	case ipv4Only:
		return "IPv4"
	case ipv6Only:
		return "IPv6"
	default:
		return "IPv4+IPv6"
	}
}

type sortMode int

const (
	sortByPort sortMode = iota
	sortByProcess
	sortByPID
	sortByCPU
	sortModeCount
)

func (s sortMode) String() string {
	switch s {
	case sortByProcess:
		return "process"
	case sortByPID:
		return "PID"
	case sortByCPU:
		return "CPU"
	default:
		return "port"
	}
}

// Config configures a Model at startup, sourced from CLI flags/args.
type Config struct {
	InitialFilter   string
	ShowEstablished bool
	RefreshInterval time.Duration
	ResolveDNS      bool
	ResolveSystemd  bool
	ResolveDocker   bool
	NotifyOnNewPort bool
}

// Model is the root Bubble Tea model for portop.
type Model struct {
	cfg       Config
	collector *app.Collector

	rows     []app.Row
	filtered []app.Row
	cursor   int

	width, height int

	mode mode

	filterInput textinput.Model

	showEstablished bool
	ipFilter        ipFilterMode
	sort            sortMode

	newSeen map[app.Key]bool // ports flagged "new" since last acknowledgment (n)

	detailInfo *procinfo.Info
	detailErr  error

	killTarget app.Row

	statusMsg   string
	statusIsErr bool
	lastErr     error

	quitting bool
}

// New builds the initial model. The TUI starts collecting as soon as
// Init's command runs.
func New(cfg Config) Model {
	fi := textinput.New()
	fi.Placeholder = "port, process or PID..."
	fi.CharLimit = 64
	fi.SetValue(cfg.InitialFilter)

	return Model{
		cfg:             cfg,
		collector:       app.NewCollector(),
		filterInput:     fi,
		showEstablished: cfg.ShowEstablished,
		ipFilter:        ipAll,
		newSeen:         make(map[app.Key]bool),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.collectCmd(), tickCmd(m.cfg.RefreshInterval))
}

type collectResultMsg struct {
	rows []app.Row
	err  error
}

type tickMsg time.Time

type detailResultMsg struct {
	info procinfo.Info
	err  error
}

type killResultMsg struct {
	pid  int
	name string
	err  error
}

func tickCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) collectCmd() tea.Cmd {
	collector := m.collector
	opts := app.Options{
		ResolveSystemd: m.cfg.ResolveSystemd,
		ResolveDocker:  m.cfg.ResolveDocker,
		ResolveDNS:     m.cfg.ResolveDNS,
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rows, err := collector.Collect(ctx, opts)
		return collectResultMsg{rows: rows, err: err}
	}
}

func loadDetailCmd(pid int) tea.Cmd {
	return func() tea.Msg {
		info, err := procinfo.Load(pid)
		return detailResultMsg{info: info, err: err}
	}
}

func killCmd(row app.Row, force bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		if force {
			err = procctl.Kill(row.PID)
		} else {
			err = procctl.Terminate(row.PID)
		}
		return killResultMsg{pid: row.PID, name: row.ProcessName, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.collectCmd(), tickCmd(m.cfg.RefreshInterval))

	case collectResultMsg:
		m.lastErr = msg.err
		if msg.err == nil {
			m.applyRows(msg.rows)
		}
		return m, nil

	case detailResultMsg:
		if msg.err == nil {
			m.detailInfo = &msg.info
		}
		m.detailErr = msg.err
		return m, nil

	case killResultMsg:
		m.mode = modeNormal
		if msg.err != nil {
			m.setStatus("kill "+msg.name+" ("+strconv.Itoa(msg.pid)+") failed: "+msg.err.Error(), true)
		} else {
			m.setStatus("signal sent to "+msg.name+" ("+strconv.Itoa(msg.pid)+")", false)
		}
		return m, m.collectCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) applyRows(rows []app.Row) {
	for _, r := range rows {
		if r.FirstSeen {
			m.newSeen[keyForRow(r)] = true
			if m.cfg.NotifyOnNewPort && r.State == "LISTEN" {
				notify.Send("portop: new port listening", r.ProcessName+" on :"+strconv.Itoa(int(r.LocalPort)))
			}
		}
	}
	m.rows = rows
	m.refilter()
}

func keyForRow(r app.Row) app.Key {
	return app.Key{
		Protocol: r.Protocol,
		Local:    r.LocalAddr.String() + ":" + strconv.Itoa(int(r.LocalPort)),
		Remote:   r.RemoteAddr.String() + ":" + strconv.Itoa(int(r.RemotePort)),
	}
}

func (m *Model) setStatus(msg string, isErr bool) {
	m.statusMsg = msg
	m.statusIsErr = isErr
}

func (m *Model) selected() (app.Row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return app.Row{}, false
	}
	return m.filtered[m.cursor], true
}
