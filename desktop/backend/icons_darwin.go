package backend

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var plistIcon = regexp.MustCompile(`<key>CFBundleIconFile</key>\s*<string>([^<]+)</string>`)

// appIcon finds the .app bundle an executable belongs to and the PNG
// inside its icon file.
func appIcon(exe, name string) string {
	bundle := ""
	for dir := exe; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if strings.HasSuffix(dir, ".app") {
			bundle = dir // the outermost .app wins (helpers live inside apps)
		}
	}
	if bundle == "" {
		return ""
	}
	res := filepath.Join(bundle, "Contents", "Resources")
	candidates := []string{}
	if plist, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist")); err == nil {
		if m := plistIcon.FindSubmatch(plist); m != nil {
			n := string(m[1])
			if !strings.HasSuffix(n, ".icns") {
				n += ".icns"
			}
			candidates = append(candidates, filepath.Join(res, filepath.Base(n)))
		}
	}
	candidates = append(candidates, filepath.Join(res, "AppIcon.icns"))
	if entries, err := os.ReadDir(res); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".icns") {
				candidates = append(candidates, filepath.Join(res, e.Name()))
			}
		}
	}
	for _, c := range candidates {
		b, err := os.ReadFile(c)
		if err != nil || len(b) > 8<<20 {
			continue
		}
		if png := icnsPNG(b); png != nil && len(png) <= maxIcon*4 {
			return dataURL("image/png", png)
		}
	}
	return ""
}
