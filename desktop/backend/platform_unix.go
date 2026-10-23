//go:build !windows

package backend

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// processArgv reads a process's arguments without the quoting lost by
// joining them: from /proc on Linux. Elsewhere it returns nil and the
// command line string is used as it is.
func processArgv(pid int) []string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil || len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
}

// isAdmin is true when the window itself runs as root.
func isAdmin() bool { return os.Geteuid() == 0 }

// elevateAvailable is true where runElevated can ask for rights: macOS
// always, Linux when pkexec is installed.
func elevateAvailable() bool {
	if runtime.GOOS == "darwin" {
		return true
	}
	_, err := exec.LookPath("pkexec")
	return err == nil
}

// runElevated runs this executable with args as root: pkexec on Linux,
// the administrator password dialog on macOS.
func runElevated(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		script := "do shell script " + appleScriptString(shellJoin(append([]string{exe}, args...))) + " with administrator privileges"
		cmd = exec.Command("/usr/bin/osascript", "-e", script)
	} else {
		pkexec, err := exec.LookPath("pkexec")
		if err != nil {
			return errors.New("this process belongs to another user, and pkexec is not installed to ask for administrator rights")
		}
		cmd = exec.Command(pkexec, append([]string{exe}, args...)...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(stderr.String())
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		switch {
		case runtime.GOOS != "darwin" && (exit.ExitCode() == 126 || exit.ExitCode() == 127):
			return errors.New("administrator authorization was cancelled")
		case runtime.GOOS == "darwin" && strings.Contains(msg, "-128"):
			return errors.New("administrator authorization was cancelled")
		}
	}
	if msg != "" {
		msg = strings.TrimPrefix(msg, "portop: ")
		return errors.New(msg)
	}
	return err
}

// openTerminal runs command from dir in a new terminal window.
func openTerminal(title, dir, command string) error {
	line := keepOpen(dir, command)
	if runtime.GOOS == "darwin" {
		script := "tell application \"Terminal\"\n  do script " + appleScriptString(line) + "\n  activate\nend tell"
		return exec.Command("/usr/bin/osascript", "-e", script).Start()
	}
	for _, t := range linuxTerminals(line, dir) {
		if path, err := exec.LookPath(t[0]); err == nil {
			cmd := exec.Command(path, t[1:]...)
			cmd.Dir = dir
			return cmd.Start()
		}
	}
	return errors.New("no terminal emulator found; set $TERMINAL")
}

// linuxTerminals lists terminal commands to try, the user's first.
func linuxTerminals(line, dir string) [][]string {
	sh := []string{"sh", "-c", line}
	var out [][]string
	if t := os.Getenv("TERMINAL"); t != "" {
		out = append(out, append([]string{t, "-e"}, sh...))
	}
	return append(out,
		append([]string{"x-terminal-emulator", "-e"}, sh...),
		append([]string{"gnome-terminal", "--working-directory=" + dir, "--"}, sh...),
		append([]string{"ptyxis", "--working-directory=" + dir, "--"}, sh...),
		append([]string{"konsole", "--workdir", dir, "-e"}, sh...),
		append([]string{"xfce4-terminal", "--working-directory=" + dir, "-x"}, sh...),
		append([]string{"kitty", "--directory", dir}, sh...),
		append([]string{"alacritty", "--working-directory", dir, "-e"}, sh...),
		append([]string{"wezterm", "start", "--cwd", dir, "--"}, sh...),
		append([]string{"foot", "-D", dir}, sh...),
		append([]string{"xterm", "-e"}, sh...),
	)
}

// terminalAvailable is true when openTerminal has a terminal to open:
// always on macOS (Terminal.app), and on Linux when one is installed.
func terminalAvailable() bool {
	if runtime.GOOS == "darwin" {
		return true
	}
	for _, t := range linuxTerminals("", "") {
		if _, err := exec.LookPath(t[0]); err == nil {
			return true
		}
	}
	return false
}
