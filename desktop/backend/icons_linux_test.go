package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDesktop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spotify.desktop")
	os.WriteFile(path, []byte("[Desktop Entry]\nName=Spotify\nExec=env BAMF=1 /usr/share/spotify/spotify %U\nIcon=spotify-client\nStartupWMClass=Spotify\n[Desktop Action New]\nIcon=other\n"), 0o644)
	icon, execName, wm := parseDesktop(path)
	if icon != "spotify-client" || execName != "spotify" || wm != "spotify" {
		t.Errorf("parseDesktop = %q %q %q", icon, execName, wm)
	}
	if got := themeIcon("/abs/icon.png"); got != "/abs/icon.png" {
		t.Errorf("an absolute icon path was changed: %q", got)
	}
}
