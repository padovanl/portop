package backend

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/padovanl/portop/internal/baseline"
	"github.com/padovanl/portop/internal/compose"
	"github.com/padovanl/portop/internal/hostcmd"
)

// ---- Open in editor ----

// editorCommands are tried in order when no editor is set. Each is a
// command line tool that opens a folder given as its argument.
var editorCommands = []struct{ cmd, name string }{
	{"code", "Visual Studio Code"}, {"cursor", "Cursor"}, {"windsurf", "Windsurf"}, {"zed", "Zed"},
	{"subl", "Sublime Text"}, {"idea", "IntelliJ IDEA"}, {"webstorm", "WebStorm"}, {"goland", "GoLand"},
	{"pycharm", "PyCharm"}, {"fleet", "Fleet"}, {"codium", "VSCodium"},
}

// macApps are opened with `open -a` when their command line tool is not
// on the PATH, as is common on macOS.
var macApps = []string{"Visual Studio Code", "Cursor", "Windsurf", "Zed", "Sublime Text", "IntelliJ IDEA", "WebStorm", "GoLand", "PyCharm", "VSCodium"}

// Editor is one way to open a folder.
type Editor struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// Editors lists the editors found on this machine, for the setting.
func (b *Backend) Editors() []Editor {
	out := []Editor{}
	for _, e := range editorCommands {
		if _, err := exec.LookPath(e.cmd); err == nil {
			out = append(out, Editor{Name: e.name, Command: e.cmd})
		}
	}
	if runtime.GOOS == "darwin" {
		for _, app := range macApps {
			if _, err := os.Stat("/Applications/" + app + ".app"); err == nil {
				out = append(out, Editor{Name: app, Command: "open -a \"" + app + "\""})
			}
		}
	}
	return out
}

// splitCommand splits a command setting into words, keeping a
// double-quoted word whole: open -a "Visual Studio Code".
func splitCommand(s string) []string {
	var out []string
	var cur strings.Builder
	quoted, any := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quoted, any = !quoted, true
		case r == ' ' && !quoted:
			if any {
				out = append(out, cur.String())
				cur.Reset()
				any = false
			}
		default:
			cur.WriteRune(r)
			any = true
		}
	}
	if any {
		out = append(out, cur.String())
	}
	return out
}

// OpenInEditor opens a project folder in the editor from the settings,
// or the first one found.
func (b *Backend) OpenInEditor(dir string) error {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("not a folder: %q", dir)
	}
	command := b.settings.get().Editor
	if command == "" {
		found := b.Editors()
		if len(found) == 0 {
			return errors.New("no editor found; choose one in Settings")
		}
		command = found[0].Command
	}
	words := splitCommand(command)
	if len(words) == 0 {
		return errors.New("the editor setting is empty")
	}
	cmd := hostcmd.Hide(exec.Command(words[0], append(words[1:], dir)...))
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start %s: %v", words[0], err)
	}
	go cmd.Wait()
	return nil
}

// ---- Free port nearby ----

// FreePortsNear returns up to three free TCP ports from port upwards:
// nothing listens on them and this machine can bind them.
func (b *Backend) FreePortsNear(port int) []int {
	if port < 1 || port > 65535 {
		return []int{}
	}
	busy := map[int]bool{}
	for _, r := range b.Snapshot().Rows {
		if r.Listening {
			busy[int(r.LocalPort)] = true
		}
	}
	out := []int{}
	for p := port; p <= 65535 && p < port+500 && len(out) < 3; p++ {
		if busy[p] || !bindable(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func bindable(port int) bool {
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// ---- Export ----

// ExportResult is a written file, or the content when there is no save
// dialog (the browser development server).
type ExportResult struct {
	Path      string `json:"path,omitempty"`
	Name      string `json:"name,omitempty"`
	Content   string `json:"content,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// Export writes the current sockets as JSON (the same fields as portop
// --json) or CSV.
func (b *Backend) Export(format string) (ExportResult, error) {
	snap := b.Snapshot()
	var data []byte
	var err error
	switch format {
	case "json":
		data, err = json.MarshalIndent(snap.Rows, "", "  ")
		data = append(data, '\n')
	case "csv":
		data, err = rowsCSV(snap.Rows)
	default:
		return ExportResult{}, fmt.Errorf("unknown format %q", format)
	}
	if err != nil {
		return ExportResult{}, err
	}
	host, _ := os.Hostname()
	name := fmt.Sprintf("portop-%s-%s.%s", host, time.Now().Format("2006-01-02-150405"), format)
	if b.SaveDialog == nil {
		return ExportResult{Name: name, Content: string(data)}, nil
	}
	path, err := b.SaveDialog("Export ports", name)
	if err != nil {
		return ExportResult{}, err
	}
	if path == "" {
		return ExportResult{Cancelled: true}, nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: path}, nil
}

func rowsCSV(rows []Row) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"protocol", "local_address", "local_port", "remote_address", "remote_port", "remote_host", "state", "scope",
		"pid", "process", "user", "category", "project", "framework", "unit", "container", "firewall", "rx_bytes_per_second", "tx_bytes_per_second"})
	f := func(p *float64) string {
		if p == nil {
			return ""
		}
		return strconv.FormatFloat(*p, 'f', 0, 64)
	}
	for _, r := range rows {
		port := func(p uint16) string {
			if p == 0 {
				return ""
			}
			return strconv.Itoa(int(p))
		}
		pid := ""
		if r.PID != 0 {
			pid = strconv.Itoa(r.PID)
		}
		_ = w.Write([]string{r.Protocol, r.LocalAddress, port(r.LocalPort), r.RemoteAddress, port(r.RemotePort), csvSafe(r.RemoteHost), r.State, r.Scope,
			pid, csvSafe(r.Process), csvSafe(r.User), r.Category, csvSafe(r.Project), r.Framework, csvSafe(r.Unit), csvSafe(r.Container), r.Firewall, f(r.RXRate), f(r.TXRate)})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// csvSafe keeps a spreadsheet from reading a process name as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// ---- Compose audit ----

// PickComposeFolder asks for a Compose project folder.
func (b *Backend) PickComposeFolder() (string, error) {
	if b.FolderDialog == nil {
		return "", errors.New("folders can only be picked in the app")
	}
	return b.FolderDialog("Choose a Docker Compose project")
}

// ComposeAudit checks a Compose project's published ports against what
// listens, as portop --compose does, and remembers the folder.
func (b *Backend) ComposeAudit(dir string) (compose.Summary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	summary, err := compose.AuditDir(ctx, dir, compose.AuditOptions{ResolveDocker: true})
	if err != nil {
		return compose.Summary{}, err
	}
	s := b.settings.get()
	s.ComposeDirs = append([]string{summary.Directory}, s.ComposeDirs...)
	_, _ = b.settings.set(s)
	return summary, nil
}

// ---- Baseline ----

// BaselineStatus compares the sockets with the saved baseline, the same
// file portop --save-baseline and --diff use.
type BaselineStatus struct {
	Path    string           `json:"path"`
	Exists  bool             `json:"exists"`
	SavedAt time.Time        `json:"saved_at"`
	Added   []baseline.Entry `json:"added"`
	Removed []baseline.Entry `json:"removed"`
}

func (b *Backend) baselinePath() (string, error) {
	if b.BaselinePath != "" {
		return b.BaselinePath, nil
	}
	return baseline.DefaultPath()
}

// Baseline reports what changed since the baseline was saved.
func (b *Backend) Baseline() (BaselineStatus, error) {
	path, err := b.baselinePath()
	if err != nil {
		return BaselineStatus{}, err
	}
	st := BaselineStatus{Path: path, Added: []baseline.Entry{}, Removed: []baseline.Entry{}}
	saved, at, err := baseline.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	b.mu.RLock()
	rows := b.lastRows
	b.mu.RUnlock()
	added, removed := baseline.Diff(saved, baseline.FromRows(rows))
	st.Exists, st.SavedAt = true, at
	if added != nil {
		st.Added = added
	}
	if removed != nil {
		st.Removed = removed
	}
	return st, nil
}

// SaveBaseline records the listening ports as the expected state.
func (b *Backend) SaveBaseline() (BaselineStatus, error) {
	path, err := b.baselinePath()
	if err != nil {
		return BaselineStatus{}, err
	}
	b.mu.RLock()
	rows := b.lastRows
	b.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return BaselineStatus{}, err
	}
	if err := baseline.Save(path, baseline.FromRows(rows)); err != nil {
		return BaselineStatus{}, err
	}
	return b.Baseline()
}

// ---- Containers ----

// ContainerAction stops, starts or restarts a container by name.
func (b *Backend) ContainerAction(name, action string) error {
	err := b.docker.Act(context.Background(), name, action)
	b.Refresh()
	return err
}

// ContainerLogs returns a container's last 300 lines of output.
func (b *Backend) ContainerLogs(name string) (string, error) {
	return b.docker.Logs(context.Background(), name, 300)
}
