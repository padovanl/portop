// Package terminaltext makes untrusted text safe to render in a terminal.
package terminaltext

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Sanitize removes terminal escape sequences and replaces control characters
// with a visible replacement rune so values cannot alter terminal state or
// inject additional lines. Bidirectional controls are replaced too: they
// reorder the text around them, so a process named "\u202Edhss" would be
// drawn as "sshd" (the "Trojan Source" trick).
func Sanitize(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			return '\uFFFD'
		}
		return r
	}, s)
}
