package backend

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDesktop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spotify.desktop")
	os.WriteFile(path, []byte("[Desktop Entry]\nName=Spotify\nExec=env BAMF=1 /usr/share/spotify/spotify %U\nIcon=spotify-client\nStartupWMClass=Spotify\n[Desktop Action New]\nIcon=other\n"), 0o644)
	icon, execName, wm := parseDesktop(path)
	if icon != "spotify-client" || execName != "spotify" || wm != "spotify" {
		t.Errorf("parseDesktop = %q %q %q", icon, execName, wm)
	}
	if got := themeIcons("/abs/icon.png"); len(got) != 1 || got[0] != "/abs/icon.png" {
		t.Errorf("an absolute icon path was changed: %q", got)
	}
}

// Desktop applications on Linux: their .desktop entry names the icon,
// which may be an SVG; PORTOP_REAL_ICONS lists exe:name pairs to check
// (a container with the apps installed sets it).
func TestRealLinuxIcons(t *testing.T) {
	list := os.Getenv("PORTOP_REAL_ICONS")
	if list == "" {
		t.Skip("PORTOP_REAL_ICONS is not set")
	}
	for _, pair := range strings.Split(list, ",") {
		exe, name, _ := strings.Cut(pair, ":")
		url := appIcon(exe, name)
		if !strings.HasPrefix(url, "data:image/") {
			t.Errorf("appIcon(%s) = %.60q", exe, url)
			continue
		}
		t.Logf("%s: %s, %d bytes", exe, url[:strings.Index(url, ";")], len(url))
		if dir := os.Getenv("PORTOP_ICON_OUT"); dir != "" {
			mime := url[len("data:"):strings.Index(url, ";")]
			ext := map[string]string{"image/png": ".png", "image/svg+xml": ".svg", "image/x-icon": ".ico"}[mime]
			raw, _ := base64.StdEncoding.DecodeString(url[strings.Index(url, ",")+1:])
			os.WriteFile(filepath.Join(dir, name+ext), raw, 0o644)
		}
	}
}
