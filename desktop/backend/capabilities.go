package backend

import (
	"os"
	"runtime"
)

// Capabilities says which features this system and this build offer, so
// the window hides what cannot work, or says why.
type Capabilities struct {
	OS        string `json:"os"`
	Docker    bool   `json:"docker"`    // a Docker or Podman engine answers
	Editor    bool   `json:"editor"`    // an editor is set or was found
	Terminal  bool   `json:"terminal"`  // Restart and Start can open a terminal window
	Dialogs   bool   `json:"dialogs"`   // native save and folder dialogs
	Autostart bool   `json:"autostart"` // start at login
	Shortcut  bool   `json:"shortcut"`  // a global shortcut can be registered
}

// Capabilities reports what is available here.
func (b *Backend) Capabilities() Capabilities {
	native := b.OnSettings != nil
	return Capabilities{
		OS:        runtime.GOOS,
		Docker:    b.docker.Available(),
		Editor:    b.settings.get().Editor != "" || len(b.Editors()) > 0,
		Terminal:  terminalAvailable(),
		Dialogs:   b.SaveDialog != nil && b.FolderDialog != nil,
		Autostart: native,
		// Wayland gives applications no global shortcuts; X11 does.
		Shortcut: native && !(runtime.GOOS == "linux" && os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("DISPLAY") == ""),
	}
}
