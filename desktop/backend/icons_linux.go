package backend

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Linux applications describe their icon in a .desktop entry; the icon
// itself is a name looked up in the icon themes, or a path.

func desktopDirs() []string {
	dirs := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local/share/applications"), filepath.Join(home, ".local/share/flatpak/exports/share/applications"))
	}
	return append(dirs, "/usr/share/applications", "/usr/local/share/applications",
		"/var/lib/flatpak/exports/share/applications", "/var/lib/snapd/desktop/applications")
}

func iconDirs() []string {
	dirs := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local/share/icons"), filepath.Join(home, ".icons"))
	}
	return append(dirs, "/usr/share/icons", "/usr/local/share/icons", "/var/lib/flatpak/exports/share/icons", "/var/lib/snapd/desktop/icons")
}

// desktopIcon finds the Icon= of the .desktop entry whose Exec runs exe
// (by base name) or whose StartupWMClass is the process name.
func desktopIcon(exe, name string) string {
	base := strings.ToLower(filepath.Base(exe))
	name = strings.ToLower(name)
	for _, dir := range desktopDirs() {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".desktop") {
				continue
			}
			icon, execName, wmClass := parseDesktop(filepath.Join(dir, e.Name()))
			if icon == "" {
				continue
			}
			if execName == base || (name != "" && (wmClass == name || execName == name)) {
				return icon
			}
		}
	}
	return ""
}

func parseDesktop(path string) (icon, execName, wmClass string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	inEntry := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "Icon":
			icon = v
		case "Exec":
			fields := strings.Fields(v)
			// Skip launchers such as env VAR=x or flatpak run.
			for _, f := range fields {
				if f == "env" || strings.Contains(f, "=") {
					continue
				}
				execName = strings.ToLower(filepath.Base(strings.Trim(f, `"`)))
				break
			}
		case "StartupWMClass":
			wmClass = strings.ToLower(v)
		}
	}
	return
}

// themeIcon resolves an icon name to a file: hicolor sizes from 256 down,
// then scalable, then pixmaps.
func themeIcon(icon string) string {
	if filepath.IsAbs(icon) {
		return icon
	}
	for _, dir := range iconDirs() {
		for _, size := range []string{"256x256", "512x512", "192x192", "128x128", "96x96", "64x64", "48x48", "scalable"} {
			for _, ext := range []string{".png", ".svg"} {
				p := filepath.Join(dir, "hicolor", size, "apps", icon+ext)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}
	for _, ext := range []string{".png", ".svg"} {
		p := filepath.Join("/usr/share/pixmaps", icon+ext)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func appIcon(exe, name string) string {
	icon := desktopIcon(exe, name)
	if icon == "" {
		return ""
	}
	path := themeIcon(icon)
	mime := iconTypes[strings.ToLower(filepath.Ext(path))]
	if path == "" || mime == "" {
		return ""
	}
	if b, ok := readIcon(path); ok {
		return dataURL(mime, b)
	}
	return ""
}
