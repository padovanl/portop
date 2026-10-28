//go:build !windows

package hostcmd

import "os/exec"

// Hide keeps a command's console window from appearing on Windows; it does
// nothing elsewhere.
func Hide(cmd *exec.Cmd) *exec.Cmd { return cmd }
