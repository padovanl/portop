package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInterpolate(t *testing.T) {
	env := map[string]string{"PORT": "9000", "EMPTY": "", "HOST": "127.0.0.1"}
	cases := map[string]string{
		"8080:80":                 "8080:80",
		"${PORT}:80":              "9000:80",
		"$PORT:80":                "9000:80",
		"${MISSING:-8080}:80":     "8080:80",
		"${EMPTY:-8080}:80":       "8080:80",
		"${EMPTY-8080}:80":        ":80",
		"${HOST}:${PORT:-1}:5432": "127.0.0.1:9000:5432",
		"${MISSING:-${PORT}}:80":  "9000:80",
		"${PORT:+5000}:80":        "5000:80",
		"${MISSING:+5000}80":      "80",
		"price $$5":               "price $5",
		"cost $":                  "cost $",
	}
	for in, want := range cases {
		got, err := interpolate(in, env)
		if err != nil || got != want {
			t.Errorf("interpolate(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"${MISSING:?set MISSING}", "${EMPTY:?}", "${PORT", "${PORT!x}"} {
		if _, err := interpolate(in, env); err == nil {
			t.Errorf("interpolate(%q) succeeded, want an error", in)
		}
	}
}

// Ports written with variables are common in Compose files; they used to
// abort the whole audit with a parse error.
func TestLoadDirSubstitutesVariables(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("compose.yaml", `
services:
  web:
    ports:
      - "${PORTOP_TEST_WEB_PORT:-8080}:80"
  db:
    ports:
      - "127.0.0.1:${PORTOP_TEST_DB_PORT}:5432"
      - target: 6379
        published: ${PORTOP_TEST_CACHE_PORT}
`)
	write(".env", "# comment\nPORTOP_TEST_DB_PORT=5433\nPORTOP_TEST_CACHE_PORT=\"6380\"\n")
	t.Setenv("PORTOP_TEST_CACHE_PORT", "6381") // the environment wins over .env

	summary, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	got := map[string]uint16{}
	for _, r := range summary.Results {
		got[r.Service+"/"+r.HostIP] = r.Published
		if r.Target == 6379 {
			got["cache"] = r.Published
		}
	}
	if got["web/"] != 8080 || got["db/127.0.0.1"] != 5433 || got["cache"] != 6381 {
		t.Fatalf("published ports = %v, want web 8080, db 5433, cache 6381", got)
	}
}

func TestLoadDirEmptyComposeFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDir(dir); err != nil {
		t.Fatalf("LoadDir on an empty file: %v", err)
	}
}
