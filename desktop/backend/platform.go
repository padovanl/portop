package backend

import (
	"os/user"
	"strings"
)

// ownProcess is true when owner (as procinfo reports it: a name, a
// DOMAIN\name or a raw uid) is the user running the window.
func ownProcess(owner string) bool {
	me, err := user.Current()
	if err != nil || owner == "" {
		return false
	}
	return strings.EqualFold(owner, me.Username) || owner == me.Uid
}

// shellQuote quotes one argument for sh.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./_-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func shellJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = shellQuote(a)
	}
	return strings.Join(q, " ")
}

// appleScriptString quotes s as an AppleScript string literal.
func appleScriptString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// keepOpen is the shell line a restarted command runs in: the terminal
// stays open after the command exits, so its last errors can be read.
func keepOpen(dir, command string) string {
	return "cd " + shellQuote(dir) + " && " + command +
		`; status=$?; printf '\n[portop] the command exited with status %s. Press Enter to close.\n' "$status"; read _`
}
