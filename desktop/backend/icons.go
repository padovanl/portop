package backend

import (
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/padovanl/portop/internal/procinfo"
)

// maxIcon bounds an icon file; anything bigger is not an icon.
const maxIcon = 256 << 10

// projectIconNames and projectIconDirs are where web and app projects
// keep their favicon or logo, most specific first.
var (
	projectIconNames = []string{"favicon.svg", "favicon.png", "favicon.ico", "icon.svg", "icon.png", "logo.svg", "logo.png", "apple-touch-icon.png"}
	projectIconDirs  = []string{"", "public", "static", "app", "src/app", "assets", "src/assets", "public/img", "public/images", "web", "frontend/public", "client/public", "resources"}
)

var iconTypes = map[string]string{".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/x-icon"}

type iconCache struct {
	mu   sync.Mutex
	urls map[string]string // key -> data URL, "" when there is none
}

func (c *iconCache) get(key string, find func() string) string {
	c.mu.Lock()
	if url, ok := c.urls[key]; ok {
		c.mu.Unlock()
		return url
	}
	c.mu.Unlock()
	url := find()
	c.mu.Lock()
	if c.urls == nil {
		c.urls = map[string]string{}
	}
	c.urls[key] = url
	c.mu.Unlock()
	return url
}

var icons iconCache

func dataURL(mime string, b []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
}

// readIcon reads an icon file if it is small enough to be one.
func readIcon(path string) ([]byte, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() || st.Size() == 0 || st.Size() > maxIcon {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(f, maxIcon))
	return b, err == nil
}

func projectIcon(root string) string {
	for _, dir := range projectIconDirs {
		for _, name := range projectIconNames {
			path := filepath.Join(root, filepath.FromSlash(dir), name)
			if b, ok := readIcon(path); ok {
				return dataURL(iconTypes[strings.ToLower(filepath.Ext(name))], b)
			}
		}
	}
	return ""
}

// Icon returns a data URL for a process's icon: its project's favicon or
// logo when root is set and has one, otherwise its application's icon
// (a .desktop entry's on Linux, the app bundle's on macOS, the
// executable's on Windows). "" means none, and the window draws a
// monogram.
func (b *Backend) Icon(pid int, root string) string {
	if root != "" {
		if url := icons.get("project:"+root, func() string { return projectIcon(root) }); url != "" {
			return url
		}
	}
	if pid <= 0 {
		return ""
	}
	info, err := procinfo.Load(pid)
	if err != nil || info.Exe == "" {
		return ""
	}
	return icons.get("app:"+info.Exe, func() string { return appIcon(info.Exe, info.Name) })
}
