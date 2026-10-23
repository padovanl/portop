package backend

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/png"
	"os"
	"strings"
	"testing"
)

// realIconTest checks appIcon against an application that ships with the
// system: the icon must decode and have visible pixels. Set
// PORTOP_ICON_OUT to a directory to keep the PNGs.
func realIconTest(t *testing.T, exe, name string) {
	t.Helper()
	if _, err := os.Stat(exe); err != nil {
		t.Skipf("%s is not installed here", exe)
	}
	url := appIcon(exe, name)
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(url, prefix) {
		t.Fatalf("appIcon(%s) = %.60q, want a PNG data URL", exe, url)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, prefix))
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the icon of %s does not decode: %v", exe, err)
	}
	visible := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x4000 {
				visible++
			}
		}
	}
	if b.Dx() < 32 || visible < b.Dx()*b.Dy()/10 {
		t.Fatalf("the icon of %s is %dx%d with %d visible pixels", exe, b.Dx(), b.Dy(), visible)
	}
	if dir := os.Getenv("PORTOP_ICON_OUT"); dir != "" {
		os.WriteFile(dir+"/"+name+".png", raw, 0o644)
	}
	t.Logf("%s: %dx%d icon, %d visible pixels", exe, b.Dx(), b.Dy(), visible)
}
