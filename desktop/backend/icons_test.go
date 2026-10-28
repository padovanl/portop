package backend

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectIcon(t *testing.T) {
	root := t.TempDir()
	if projectIcon(root) != "" {
		t.Fatal("an icon was found in an empty project")
	}
	os.MkdirAll(filepath.Join(root, "public"), 0o755)
	os.WriteFile(filepath.Join(root, "public", "favicon.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o644)
	if url := projectIcon(root); !strings.HasPrefix(url, "data:image/svg+xml;base64,") {
		t.Fatalf("projectIcon = %q", url)
	}
	// Too big to be an icon.
	big := filepath.Join(t.TempDir(), "favicon.png")
	os.WriteFile(big, make([]byte, maxIcon+1), 0o644)
	if _, ok := readIcon(big); ok {
		t.Error("an oversized file was read as an icon")
	}
}

func TestIcnsPNG(t *testing.T) {
	entry := func(typ string, data []byte) []byte {
		h := make([]byte, 8)
		copy(h, typ)
		binary.BigEndian.PutUint32(h[4:], uint32(8+len(data)))
		return append(h, data...)
	}
	png128 := append(append([]byte{}, pngMagic...), []byte("small")...)
	png256 := append(append([]byte{}, pngMagic...), []byte("large")...)
	body := append(append(entry("ic07", png128), entry("ic08", png256)...), entry("icp4", []byte("not png"))...)
	file := append([]byte("icns\x00\x00\x00\x00"), body...)
	binary.BigEndian.PutUint32(file[4:], uint32(len(file)))
	if got := icnsPNG(file); string(got) != string(png256) {
		t.Errorf("icnsPNG picked %q", got)
	}
	if icnsPNG([]byte("not an icns")) != nil {
		t.Error("garbage parsed as icns")
	}
}
