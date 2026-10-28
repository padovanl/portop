package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/procinfo"
	"github.com/padovanl/portop/internal/userfile"
)

// maxRecent bounds the remembered projects.
const maxRecent = 40

// RecentProject is a dev server portop has seen, kept so it can be
// started again after it stopped.
type RecentProject struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Framework string    `json:"framework,omitempty"`
	Language  string    `json:"language,omitempty"`
	Root      string    `json:"root"`
	Dir       string    `json:"dir"`     // where the command ran
	Command   string    `json:"command"` // as the shell will run it again
	Ports     []uint16  `json:"ports"`
	LastSeen  time.Time `json:"last_seen"`
	Running   bool      `json:"running"`
	PID       int       `json:"pid,omitempty"`

	pids []int // processes of this session that ran it
}

func (p *RecentProject) pidMatches(pid int) bool {
	for _, x := range p.pids {
		if x == pid {
			return true
		}
	}
	return false
}

type recentStore struct {
	mu      sync.Mutex
	path    string
	items   map[string]*RecentProject
	checked map[int]bool // PIDs already looked at
	dirty   bool
}

func newRecentStore(path string) *recentStore {
	st := &recentStore{path: path, items: map[string]*RecentProject{}, checked: map[int]bool{}}
	if path == "" {
		return st
	}
	if b, err := os.ReadFile(path); err == nil {
		var list []RecentProject
		if json.Unmarshal(b, &list) == nil {
			for i := range list {
				p := list[i]
				p.Running, p.PID = false, 0
				st.items[p.ID] = &p
			}
		}
	}
	return st
}

func recentID(root, command string) string {
	sum := sha256.Sum256([]byte(root + "\x00" + command))
	return hex.EncodeToString(sum[:8])
}

// observe records the dev servers among rows. A process is read once:
// its command line and directory do not change while it lives.
func (st *recentStore) observe(rows []app.Row, now time.Time, read func(pid int) (procinfo.Info, []string, error)) {
	st.mu.Lock()
	defer st.mu.Unlock()
	live := map[int][]uint16{}
	devRow := map[int]app.Row{}
	for _, r := range rows {
		if r.Listening() && r.Category == "dev" && r.PID != 0 && r.Project.Root != "" {
			if !containsPort(live[r.PID], r.LocalPort) {
				live[r.PID] = append(live[r.PID], r.LocalPort)
			}
			devRow[r.PID] = r
		}
	}
	for _, p := range st.items {
		p.Running, p.PID = false, 0
	}
	for pid := range st.checked {
		if _, ok := live[pid]; !ok {
			delete(st.checked, pid)
		}
	}
	for pid, ports := range live {
		r := devRow[pid]
		var found *RecentProject
		for _, p := range st.items {
			if p.pidMatches(pid) {
				found = p
			}
		}
		if found == nil && !st.checked[pid] {
			st.checked[pid] = true
			info, argv, err := read(pid)
			if err != nil || !ownProcess(info.User) || info.Cmdline == "" {
				continue
			}
			command := info.Cmdline
			if len(argv) > 0 {
				command = shellJoin(argv)
			}
			dir := info.Cwd
			if dir == "" {
				dir = r.Project.Root
			}
			id := recentID(r.Project.Root, command)
			found = st.items[id]
			if found == nil {
				found = &RecentProject{ID: id, Root: r.Project.Root, Dir: dir, Command: command}
				st.items[id] = found
			}
			found.pids = append(found.pids, pid)
		}
		if found == nil {
			continue
		}
		sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })
		if found.Name != r.Project.Name || !slicesEqual(found.Ports, ports) || now.Sub(found.LastSeen) > time.Minute {
			st.dirty = true // written at most once a minute while it runs
		}
		found.Name, found.Framework, found.Language = r.Project.Name, r.Project.Framework, r.Project.Language
		found.Ports, found.LastSeen, found.Running, found.PID = ports, now, true, pid
	}
	if len(st.items) > maxRecent {
		list := st.sorted()
		for _, p := range list[maxRecent:] {
			delete(st.items, p.ID)
		}
	}
}

func slicesEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsPort(ports []uint16, p uint16) bool {
	for _, x := range ports {
		if x == p {
			return true
		}
	}
	return false
}

func (st *recentStore) sorted() []*RecentProject {
	list := make([]*RecentProject, 0, len(st.items))
	for _, p := range st.items {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Running != list[j].Running {
			return list[i].Running
		}
		return list[i].LastSeen.After(list[j].LastSeen)
	})
	return list
}

func (st *recentStore) list() []RecentProject {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := []RecentProject{}
	for _, p := range st.sorted() {
		c := *p
		c.pids = nil
		c.Ports = append([]uint16{}, p.Ports...)
		out = append(out, c)
	}
	return out
}

// save writes the list when it changed, readable by the user only.
func (st *recentStore) save() error {
	st.mu.Lock()
	if !st.dirty || st.path == "" {
		st.mu.Unlock()
		return nil
	}
	st.dirty = false
	list := []RecentProject{}
	for _, p := range st.sorted() {
		c := *p
		c.Running, c.PID, c.pids = false, 0, nil
		list = append(list, c)
	}
	st.mu.Unlock()
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := userfile.Write(st.path, append(b, '\n')); err != nil {
		return err
	}
	return os.Chmod(st.path, 0o600)
}

func (st *recentStore) get(id string) (RecentProject, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	p, ok := st.items[id]
	if !ok {
		return RecentProject{}, false
	}
	return *p, true
}

func (st *recentStore) forget(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.items[id]; ok {
		delete(st.items, id)
		st.dirty = true
	}
}

func defaultRecentPath(settingsPath string) string {
	if settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), "desktop-recent.json")
}

// Recent lists the remembered projects, running ones first.
func (b *Backend) Recent() []RecentProject { return b.recent.list() }

// Launch runs a remembered project's command again, from its directory,
// in a new terminal window.
func (b *Backend) Launch(id string) error {
	p, ok := b.recent.get(id)
	if !ok {
		return errors.New("that project is no longer in the list")
	}
	if p.Running {
		return errors.New(p.Name + " is already running")
	}
	if st, err := os.Stat(p.Dir); err != nil || !st.IsDir() {
		return errors.New("its folder is gone: " + p.Dir)
	}
	return openTerminal(p.Name, p.Dir, p.Command)
}

// Forget removes a project from the list.
func (b *Backend) Forget(id string) error {
	b.recent.forget(id)
	return b.recent.save()
}
