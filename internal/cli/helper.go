package cli

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/procinfo"
)

// HelperFile is the file in the helper's directory that tells portop
// desktop where the elevated helper listens and its token.
const HelperFile = "helper.json"

// HelperInfo is the content of HelperFile.
type HelperInfo struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// ServeHelper runs the elevated helper of portop desktop: the window runs
// unprivileged and starts this, through pkexec, the macOS password
// dialog or UAC, to see and stop every user's processes. It serves the
// dashboard's read API and signal action on a loopback port to whoever
// holds the token it writes to dir (a private directory the window
// made), and exits when the window's process parent is gone or on
// POST /quit.
func ServeHelper(dir string, parent int, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "portop: helper: "+err.Error())
		return 1
	}
	if parent <= 0 {
		return fail(errors.New("a parent PID is required"))
	}
	parentInfo, err := procinfo.Load(parent)
	if err != nil {
		return fail(fmt.Errorf("parent %d: %w", parent, err))
	}
	if st, err := os.Lstat(dir); err != nil || !st.IsDir() {
		return fail(errors.New("the helper directory is not a directory"))
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fail(err)
	}
	token := hex.EncodeToString(raw)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fail(err)
	}

	collector := app.NewCollector()
	var mu sync.Mutex
	collect := func(ctx context.Context, opts app.Options) ([]app.Row, error) {
		mu.Lock()
		defer mu.Unlock()
		return collector.Collect(ctx, opts)
	}
	inner := webHandler(collect, webConfig{
		Token: token,
		Options: app.Options{ResolveSystemd: true, ResolveDocker: true, ResolveDNS: true,
			ResolveFirewall: true, ResolveTraffic: true, ResolveProjects: true},
	})
	quit := make(chan struct{})
	var once sync.Once
	server := newWebServer(helperHandler(token, inner, func() { once.Do(func() { close(quit) }) }))

	// O_EXCL refuses a file (or a symlink) already in place, so the
	// helper never writes through one as administrator.
	info, _ := json.Marshal(HelperInfo{URL: "http://" + listener.Addr().String(), Token: token, PID: os.Getpid()})
	f, err := os.OpenFile(filepath.Join(dir, HelperFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		listener.Close()
		return fail(err)
	}
	_, werr := f.Write(info)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		listener.Close()
		return fail(werr)
	}
	// The window removes them too, unless it was killed.
	defer os.Remove(dir)
	defer os.Remove(filepath.Join(dir, HelperFile))

	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-quit:
			case <-tick.C:
				if p, err := procinfo.Load(parent); err == nil && p.StartTime.Equal(parentInfo.StartTime) {
					continue
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = server.Shutdown(ctx)
			cancel()
			return
		}
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	return 0
}

// helperHandler lets through only requests that carry the token: other
// users on the machine can reach a loopback port too.
func helperHandler(token string, next http.Handler, quit func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Portop-Token")), []byte(token)) != 1 {
			http.Error(w, "token required", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/quit" {
			w.WriteHeader(http.StatusNoContent)
			quit()
			return
		}
		next.ServeHTTP(w, r)
	})
}
