package notify

import "testing"

func TestEscapeAppleScript(t *testing.T) {
	cases := map[string]string{
		`hello`:        `hello`,
		`say "hi"`:     `say \"hi\"`,
		`back\slash`:   `back\\slash`,
		`both " and \`: `both \" and \\`,
	}
	for in, want := range cases {
		if got := escapeAppleScript(in); got != want {
			t.Errorf("escapeAppleScript(%q) = %q, want %q", in, got, want)
		}
	}
}
