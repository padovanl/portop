package backend

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/padovanl/portop/internal/openurl"
	"github.com/padovanl/portop/internal/procctl"
	"github.com/padovanl/portop/internal/procinfo"
	"github.com/padovanl/portop/internal/project"
)

// ProcessInfo is the detail panel of a process.
type ProcessInfo struct {
	procinfo.Info
	Started   string `json:"started"` // start time, RFC 3339 with nanoseconds: the identity Stop checks
	Project   string `json:"project,omitempty"`
	Root      string `json:"project_root,omitempty"`
	Language  string `json:"language,omitempty"`
	Framework string `json:"framework,omitempty"`
	Own       bool   `json:"own"`         // owned by the user running the window
	Restart   bool   `json:"can_restart"` // Restart can run it again
}

// Process returns the details of pid.
func (b *Backend) Process(pid int) (ProcessInfo, error) {
	info, err := procinfo.Load(pid)
	// Another user's process hides its folder and command line; the
	// administrator helper can read them.
	if c := b.helper(); c != nil && (err != nil || !ownProcess(info.User)) {
		if full, herr := c.process(pid); herr == nil {
			info, err = full, nil
		}
	}
	if err != nil {
		return ProcessInfo{}, err
	}
	out := ProcessInfo{Info: info, Own: ownProcess(info.User)}
	if !info.StartTime.IsZero() {
		out.Started = info.StartTime.Format(time.RFC3339Nano)
	}
	p := project.Detect(project.Facts{Name: info.Name, Exe: info.Exe, Cmdline: info.Cmdline, Cwd: info.Cwd})
	out.Project, out.Root, out.Language, out.Framework = p.Name, p.Root, p.Language, p.Framework
	out.Restart = out.Own && info.Cmdline != "" && (info.Cwd != "" || p.Root != "")
	return out, nil
}

// Stop signals pid if it still has the start time the window showed.
// When the user may not signal it, the administrator helper does, if it
// runs; otherwise it asks for administrator rights (pkexec, the macOS
// password dialog or UAC) and runs portop's own --signal-pid. Both check
// the start time again.
func (b *Backend) Stop(pid int, started string, force bool) error {
	expected, err := time.Parse(time.RFC3339Nano, started)
	if err != nil {
		return errors.New("the process details are out of date; open them again")
	}
	err = procctl.SignalIfUnchanged(pid, expected, force)
	if err == nil || !errors.Is(err, os.ErrPermission) {
		b.Refresh()
		return friendly(err)
	}
	if c := b.helper(); c != nil {
		err = c.signal(pid, started, force)
		b.Refresh()
		return err
	}
	args := []string{"--signal-pid", strconv.Itoa(pid), "--expected-start", started}
	if force {
		args = append(args, "--force")
	}
	err = runElevated(args)
	b.Refresh()
	return err
}

// Target names one process for StopMany.
type Target struct {
	PID     int    `json:"pid"`
	Started string `json:"started"`
}

// StopMany terminates several processes ("stop every dev server") and
// returns how many it signalled.
func (b *Backend) StopMany(targets []Target) (int, error) {
	n := 0
	var errs []string
	for _, t := range targets {
		if err := b.Stop(t.PID, t.Started, false); err != nil {
			errs = append(errs, fmt.Sprintf("PID %d: %v", t.PID, err))
			continue
		}
		n++
	}
	if len(errs) > 0 {
		return n, errors.New(strings.Join(errs, "; "))
	}
	return n, nil
}

// Restart stops one of the user's own processes, waits for it to exit,
// and runs the same command again from the same directory in a new
// terminal window, where its output and errors can be seen.
func (b *Backend) Restart(pid int, started string) error {
	info, err := b.Process(pid)
	if err != nil {
		return err
	}
	if !info.Restart {
		return errors.New("portop can only restart your own processes whose command and directory it can read")
	}
	if info.Started != started {
		return friendly(procctl.ErrProcessChanged)
	}
	dir := info.Cwd
	if dir == "" {
		dir = info.Root
	}
	command := info.Cmdline
	if argv := processArgv(pid); len(argv) > 0 {
		command = shellJoin(argv)
	}
	if err := b.Stop(pid, started, false); err != nil {
		return err
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if start, err := procctl.StartTime(pid); err != nil || start.Format(time.RFC3339Nano) != started {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if start, err := procctl.StartTime(pid); err == nil && start.Format(time.RFC3339Nano) == started {
		return errors.New("the process did not exit within 8 seconds; stop it with Force kill first")
	}
	title := info.Name
	if info.Project != "" {
		title = info.Project
	}
	return openTerminal(title, dir, command)
}

// OpenFolder shows a project directory in the file manager.
func (b *Backend) OpenFolder(path string) error {
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("not a folder: %q", path)
	}
	return openurl.Open(path)
}

func friendly(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, procctl.ErrProcessChanged):
		return errors.New("the process has exited or changed since it was shown")
	}
	return err
}

// isLocalURL accepts http and https addresses of this machine only, so a
// page cannot make the window open arbitrary URLs.
func isLocalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
			return true
		}
	}
	return false
}
