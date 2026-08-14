// Package openurl opens a URL in the user's default browser, on
// whichever OS portop happens to be running on.
package openurl

import (
	"fmt"
	"os/exec"
	"runtime"
)

// URLForPort builds the URL portop's "o" (open) action points the
// browser at for a local listening port: https for the conventional TLS
// ports, http otherwise.
func URLForPort(port uint16) string {
	scheme := "http"
	if port == 443 || port == 8443 {
		scheme = "https"
	}
	return fmt.Sprintf("%s://localhost:%d", scheme, port)
}

// Open launches the given URL in the default browser. The command is
// started but not waited on, so a slow or misbehaving browser launcher
// never blocks the TUI.
func Open(url string) error {
	cmd := command(url)
	if cmd == nil {
		return fmt.Errorf("openurl: unsupported platform %q", runtime.GOOS)
	}
	return cmd.Start()
}

func command(url string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return exec.Command("xdg-open", url)
	}
}
