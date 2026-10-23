// Package project works out what a listening process is: the project it
// was started from (the nearest package.json, go.mod, Cargo.toml... above
// its working directory or script), the framework serving it (Vite,
// Django, Rails...) and a broad category (dev server, database, web
// server, system service).
package project

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Info describes the project behind a process. Every field may be empty.
type Info struct {
	Name      string // the project's own name (package.json "name", go.mod module...)
	Root      string // directory holding the manifest
	Language  string // Node.js, Go, Rust, Python...
	Framework string // Vite, Next.js, Django...
}

// Facts is what Detect needs to know about a process.
type Facts struct {
	Name    string // process name, as the scanner reports it
	Exe     string // executable path, if known
	Cmdline string // command line, arguments joined by spaces
	Cwd     string // working directory, if readable
}

// maxDepth bounds the walk up from the working directory.
const maxDepth = 8

// Detect finds the project of a process from its working directory, or
// failing that from a script path on its command line, and the framework
// from its command line or the project's dependencies.
func Detect(f Facts) Info {
	var info Info
	for _, dir := range startDirs(f) {
		if m, ok := findManifest(dir); ok {
			info = m
			break
		}
	}
	if fw := frameworkFromCmdline(f.Cmdline); fw != "" {
		info.Framework = fw
		if info.Language == "" {
			info.Language = languageOf(fw)
		}
	}
	return info
}

// startDirs lists the directories to search from: the working directory,
// then the directory of every absolute path on the command line (a server
// started as "node /srv/app/server.js" from / still belongs to /srv/app).
func startDirs(f Facts) []string {
	var dirs []string
	add := func(dir string) {
		dir = trimDependencyDir(filepath.Clean(dir))
		if dir == "" || isSystemDir(dir) {
			return
		}
		for _, d := range dirs {
			if d == dir {
				return
			}
		}
		dirs = append(dirs, dir)
	}
	if f.Cwd != "" {
		add(f.Cwd)
	}
	for _, arg := range splitArgs(f.Cmdline) {
		arg = strings.Trim(arg, `"'`)
		if !filepath.IsAbs(arg) || arg == f.Exe {
			continue
		}
		if st, err := os.Stat(arg); err == nil {
			if st.IsDir() {
				add(arg)
			} else {
				add(filepath.Dir(arg))
			}
		}
	}
	return dirs
}

// trimDependencyDir cuts a path inside node_modules, site-packages, a
// virtualenv or a vendor directory back to the project that owns it, and
// drops paths inside installed applications and tools (~/.vscode-server,
// AppData, an .app bundle), whose package.json is not the user's project.
func trimDependencyDir(dir string) string {
	parts := strings.Split(filepath.ToSlash(dir), "/")
	for i, p := range parts {
		switch {
		case len(p) > 1 && p[0] == '.' && p != ".." && !isDependencyDir(p),
			p == "Library", p == "Applications", strings.HasSuffix(p, ".app"),
			strings.EqualFold(p, "AppData") && installedUnderAppData(parts[i+1:]):
			return ""
		}
	}
	for i, p := range parts {
		if isDependencyDir(p) {
			if i == 0 {
				return ""
			}
			return filepath.FromSlash(strings.Join(parts[:i], "/"))
		}
	}
	return dir
}

// installedUnderAppData is true for the parts of AppData where Windows
// programs install themselves (Roaming, Local\Programs, Local\Microsoft),
// not for AppData\Local\Temp and the like.
func installedUnderAppData(rest []string) bool {
	if len(rest) == 0 {
		return false
	}
	if strings.EqualFold(rest[0], "Roaming") {
		return true
	}
	return len(rest) > 1 && strings.EqualFold(rest[0], "Local") &&
		(strings.EqualFold(rest[1], "Programs") || strings.EqualFold(rest[1], "Microsoft"))
}

func isDependencyDir(name string) bool {
	switch name {
	case "node_modules", "site-packages", "dist-packages", ".venv", "venv", "vendor", ".bin":
		return true
	}
	return false
}

// isSystemDir is true for directories that never hold a user's project,
// where daemons usually run from.
func isSystemDir(dir string) bool {
	if home, err := os.UserHomeDir(); err == nil && strings.EqualFold(filepath.Clean(home), filepath.Clean(dir)) {
		return true // a server started from ~ is not "the home project"
	}
	d := strings.ToLower(filepath.ToSlash(dir))
	if runtime.GOOS == "windows" {
		if len(d) <= 3 { // C:/
			return true
		}
		for _, p := range []string{"c:/windows", "c:/program files", "c:/programdata"} {
			if d == p || strings.HasPrefix(d, p+"/") || strings.HasPrefix(d, p+" (x86)") {
				return true
			}
		}
		return false
	}
	if d == "/" {
		return true
	}
	for _, p := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc", "/proc", "/sys", "/dev", "/run", "/var/lib", "/var/run", "/system", "/library", "/private/var", "/snap"} {
		if d == p || strings.HasPrefix(d, p+"/") {
			return true
		}
	}
	return false
}

// findManifest walks up from dir to the nearest directory with a project
// manifest.
func findManifest(dir string) (Info, bool) {
	for i := 0; i < maxDepth && dir != ""; i++ {
		if isSystemDir(dir) {
			return Info{}, false
		}
		if info, ok := readManifests(dir); ok {
			return info, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return Info{}, false
}
