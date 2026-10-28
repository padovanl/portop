package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/padovanl/portop/internal/cli"
)

func TestHelperSurvivesFailedScans(t *testing.T) {
	var quits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Portop-Token") != "tok" {
			http.Error(w, "token required", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/quit" {
			quits.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "port scan failed", http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := New(filepath.Join(t.TempDir(), "s.json"))
	c := &helperConn{HelperInfo: cli.HelperInfo{URL: srv.URL, Token: "tok"}, dir: t.TempDir(), client: srv.Client()}
	b.elev.conn = c
	for i := 1; i < helperFailLimit; i++ {
		if _, ok := b.helperRows(context.Background(), c); ok {
			t.Fatal("a failed scan was reported as a success")
		}
		if b.helper() == nil {
			t.Fatalf("the helper was dropped after %d failed scan(s)", i)
		}
	}
	b.helperRows(context.Background(), c)
	if b.helper() != nil {
		t.Fatalf("the helper was kept after %d failed scans", helperFailLimit)
	}
	if quits.Load() != 1 {
		t.Errorf("the dropped helper was asked to quit %d times, want 1", quits.Load())
	}
}
