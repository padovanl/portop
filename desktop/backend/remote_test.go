package backend

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemotePinnedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer srv.Close()
	b := New(filepath.Join(t.TempDir(), "s.json"))
	var rows []remoteRow

	err := b.remoteGet(context.Background(), Host{URL: srv.URL}, "", "/api/ports", &rows)
	var untrusted *UntrustedCertError
	if !errors.As(err, &untrusted) {
		t.Fatalf("a self-signed certificate: %v, want UntrustedCertError", err)
	}
	if err := b.remoteGet(context.Background(), Host{URL: srv.URL, Fingerprint: untrusted.Fingerprint}, "", "/api/ports", &rows); err != nil {
		t.Fatalf("the pinned certificate: %v", err)
	}
	err = b.remoteGet(context.Background(), Host{URL: srv.URL, Fingerprint: strings.Repeat("00", 32)}, "", "/api/ports", &rows)
	if !errors.Is(err, errCertChanged) || err.Error() != errCertChanged.Error() {
		t.Errorf("another certificate: %v, want errCertChanged as is", err)
	}
}
