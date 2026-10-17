package userfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "config.yml")
	if err := Write(path, []byte("theme: nord\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "theme: nord\n" {
		t.Fatalf("read back %q, %v", data, err)
	}
}

// Only what Write creates is handed to the sudo user: the missing parent
// directories and a new file, never directories that already existed.
func TestWriteHandsOverOnlyWhatItCreated(t *testing.T) {
	var got []string
	old := handOver
	handOver = func(paths []string) { got = append([]string(nil), paths...) }
	t.Cleanup(func() { handOver = old })

	root := t.TempDir()
	path := filepath.Join(root, "portop", "nested", "baseline.json")
	if err := Write(path, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "portop", "nested"), filepath.Join(root, "portop"), path}
	if len(got) != len(want) {
		t.Fatalf("handed over %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("handed over %q, want %q", got, want)
		}
	}

	if err := Write(path, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("rewriting an existing file handed over %q", got)
	}
}
