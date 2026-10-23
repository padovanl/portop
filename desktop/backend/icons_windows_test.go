package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRealWindowsIcons(t *testing.T) {
	win := os.Getenv("WINDIR")
	realIconTest(t, filepath.Join(win, "System32", "notepad.exe"), "notepad")
	realIconTest(t, filepath.Join(win, "explorer.exe"), "explorer")
}
