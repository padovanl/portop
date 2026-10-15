package docker

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps docker.exe from flashing a console window when portop
// itself runs without one (e.g. launched from the Start menu alias).
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
