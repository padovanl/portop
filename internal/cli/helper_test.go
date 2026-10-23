package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServeHelper(t *testing.T) {
	dir := t.TempDir()
	done := make(chan int, 1)
	go func() { done <- ServeHelper(dir, os.Getpid(), io.Discard) }()

	var info HelperInfo
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, err := os.ReadFile(filepath.Join(dir, HelperFile))
		if err == nil && json.Unmarshal(b, &info) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the helper wrote no address")
		}
		time.Sleep(50 * time.Millisecond)
	}
	get := func(token string) int {
		req, _ := http.NewRequest(http.MethodGet, info.URL+"/api/ports", nil)
		if token != "" {
			req.Header.Set("X-Portop-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := get(""); code != http.StatusForbidden {
		t.Errorf("without the token: %d", code)
	}
	if code := get("wrong"); code != http.StatusForbidden {
		t.Errorf("with a wrong token: %d", code)
	}
	if code := get(info.Token); code != http.StatusOK {
		t.Errorf("with the token: %d", code)
	}

	req, _ := http.NewRequest(http.MethodPost, info.URL+"/quit", nil)
	req.Header.Set("X-Portop-Token", info.Token)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("quit: %v %v", resp, err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code %d", code)
		}
		if _, err := os.Stat(dir); err == nil {
			t.Error("the helper left its directory behind")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the helper did not exit")
	}
}

func TestServeHelperRefusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	os.Symlink(filepath.Join(dir, "elsewhere"), filepath.Join(dir, HelperFile))
	if code := ServeHelper(dir, os.Getpid(), io.Discard); code == 0 {
		t.Error("the helper wrote through a file already in place")
	}
	if _, err := os.Lstat(filepath.Join(dir, HelperFile)); err != nil {
		t.Error("the existing file was removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "elsewhere")); err == nil {
		t.Error("the helper followed a symlink")
	}
}
