// Package notify sends best-effort desktop notifications (used by
// portop's "alert on new listening port" feature). It shells out to
// notify-send (Linux/freedesktop) or osascript (macOS) when available
// and silently does nothing otherwise, since a missing notifier must
// never be fatal for a terminal tool.
package notify

import (
	"os/exec"
	"runtime"
)

// Send fires a best-effort desktop notification with the given title and
// body. Errors are swallowed: notifications are a nice-to-have, and a
// headless server (the most common place to run portop) simply won't
// have a notifier installed.
func Send(title, body string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		script := `display notification "` + escapeAppleScript(body) + `" with title "` + escapeAppleScript(title) + `"`
		cmd = exec.Command("osascript", "-e", script)
	default:
		if _, err := exec.LookPath("notify-send"); err != nil {
			return
		}
		cmd = exec.Command("notify-send", title, body)
	}
	_ = cmd.Run()
}

func escapeAppleScript(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			out = append(out, '\\')
		}
		out = append(out, s[i])
	}
	return string(out)
}
