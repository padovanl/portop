package terminaltext

import "testing"

func TestSanitize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "safe unicode", in: "café 日本語", want: "café 日本語"},
		{name: "CSI", in: "safe\x1b[31mred\x1b[0m", want: "safered"},
		{name: "OSC 8", in: "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\", want: "link"},
		{name: "OSC 52", in: "before\x1b]52;c;Y2xpcGJvYXJk\aafter", want: "beforeafter"},
		{name: "line controls", in: "a\nb\rc\td", want: "a�b�c�d"},
		{name: "C1", in: "a\u0085b", want: "a�b"},
		{name: "C1 CSI", in: "a\u009b2Jb", want: "a�2Jb"},
		{name: "raw 8-bit CSI", in: "a\x9b2Jb", want: "ab"},
		{name: "bare ESC", in: "a\x1bcb", want: "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sanitize(tt.in); got != tt.want {
				t.Fatalf("Sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
