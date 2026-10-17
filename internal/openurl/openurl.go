// Package openurl opens a URL in the user's default browser, on
// whichever OS portop happens to be running on.
package openurl

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
)

// URLForPort builds the URL portop's "o" (open) action points the
// browser at for a local listening port: https for the conventional TLS
// ports, http otherwise.
func URLForPort(port uint16) string {
	return fmt.Sprintf("%s://localhost:%d", scheme(port), port)
}

// URLForListener is URLForPort for a socket bound to addr. A service bound
// to one interface (192.168.1.5:8080, or a container network address) does
// not answer on localhost, so the URL uses that address; wildcard and
// loopback binds keep localhost.
func URLForListener(addr net.IP, port uint16) string {
	if addr == nil || addr.IsUnspecified() || addr.IsLoopback() {
		return URLForPort(port)
	}
	return scheme(port) + "://" + net.JoinHostPort(addr.String(), strconv.Itoa(int(port)))
}

func scheme(port uint16) string {
	if port == 443 || port == 8443 {
		return "https"
	}
	return "http"
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
