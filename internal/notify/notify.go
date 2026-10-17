// Package notify sends best-effort desktop notifications (used by
// portop's "alert on new listening port" feature). It shells out to
// notify-send (Linux/freedesktop), osascript (macOS) or a PowerShell
// toast (Windows) when available
// and silently does nothing otherwise, since a missing notifier must
// never be fatal for a terminal tool.
package notify

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/padovanl/portop/internal/terminaltext"
)

// Send fires a best-effort desktop notification with the given title and
// body. Errors are swallowed: notifications are a nice-to-have, and a
// headless server (the most common place to run portop) simply won't
// have a notifier installed. It never waits for the notifier, since it is
// called from the TUI's update loop.
func Send(title, body string) {
	cmd := command(runtime.GOOS, terminaltext.Sanitize(title), terminaltext.Sanitize(body))
	if cmd == nil {
		return
	}
	if cmd.Start() == nil {
		go func() { _ = cmd.Wait() }()
	}
}

// command builds the notifier invocation for goos, or nil when there is
// none. The body carries a process name, which any local user controls.
func command(goos, title, body string) *exec.Cmd {
	switch goos {
	case "darwin":
		script := `display notification "` + escapeAppleScript(body) + `" with title "` + escapeAppleScript(title) + `"`
		return exec.Command("osascript", "-e", script)
	case "windows":
		// Text travels in env vars to avoid quoting.
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", windowsToastScript)
		cmd.Env = append(os.Environ(), "PORTOP_TOAST_TITLE="+title, "PORTOP_TOAST_BODY="+body)
		return cmd
	default:
		if _, err := exec.LookPath("notify-send"); err != nil {
			return nil
		}
		// "--" ends option parsing: a process named "-w" or "--action=x"
		// must not become a notify-send option.
		return exec.Command("notify-send", "--", title, body)
	}
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

// windowsToastScript shows a toast through the WinRT notification API,
// attributed to Windows PowerShell's registered app ID so it needs no
// installation step of its own.
const windowsToastScript = `$ErrorActionPreference = 'SilentlyContinue'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$n = $t.GetElementsByTagName('text')
$n.Item(0).AppendChild($t.CreateTextNode($env:PORTOP_TOAST_TITLE)) > $null
$n.Item(1).AppendChild($t.CreateTextNode($env:PORTOP_TOAST_BODY)) > $null
$id = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($id).Show([Windows.UI.Notifications.ToastNotification]::new($t))`
