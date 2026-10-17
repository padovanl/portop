package cli

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/procinfo"
)

//go:embed web/index.html
var webPage string

//go:embed web/app.css
var webCSS string

//go:embed web/app.js
var webJS string

//go:embed web/logo.png
var webLogo []byte

// webConfig is what the dashboard server needs besides the port collector.
// Auth is nil when --web-auth is off; the per-run Token then guards process
// details and signals instead of a signed-in session.
type webConfig struct {
	Filter     string
	ListenOnly bool
	Options    app.Options
	Token      string
	Auth       *webAuth
	// AnyHost disables the loopback Host check. It is only set when the
	// server is reachable from other machines, which requires Auth.
	AnyHost bool
}

func webHandler(collect func(context.Context, app.Options) ([]app.Row, error), cfg webConfig) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, webPage)
	})
	mux.HandleFunc("GET /app.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = io.WriteString(w, webCSS)
	})
	mux.HandleFunc("GET /app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = io.WriteString(w, webJS)
	})
	mux.HandleFunc("GET /logo.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(webLogo)
	})
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth == nil {
			http.NotFound(w, r)
			return
		}
		session, _ := cfg.Auth.session(r)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"user": cfg.Auth.user, "host": cfg.Auth.host, "token": session.csrf})
	})
	mux.HandleFunc("GET /api/ports", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		rows, err := collect(ctx, cfg.Options)
		if err != nil {
			http.Error(w, "port scan failed", http.StatusInternalServerError)
			return
		}
		out := make([]jsonRow, 0, len(rows))
		for _, row := range rows {
			if cfg.ListenOnly && !row.Listening() {
				continue
			}
			if !row.Matches(cfg.Filter) {
				continue
			}
			out = append(out, toJSONRow(row))
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(out)
	})
	currentProcess := func(r *http.Request, pid int) (bool, error) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		rows, err := collect(ctx, cfg.Options)
		if err != nil {
			return false, err
		}
		for _, row := range rows {
			if row.PID == pid {
				return true, nil
			}
		}
		return false, nil
	}
	processRequest := func(w http.ResponseWriter, r *http.Request) (int, bool) {
		token := cfg.Token
		if cfg.Auth != nil {
			session, _ := cfg.Auth.session(r)
			token = session.csrf
		}
		if token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Portop-Token")), []byte(token)) != 1 {
			http.Error(w, "action token required; open the URL printed by portop", http.StatusForbidden)
			return 0, false
		}
		pid, err := strconv.Atoi(r.PathValue("pid"))
		if err != nil || pid <= 0 {
			http.Error(w, "invalid PID", http.StatusBadRequest)
			return 0, false
		}
		visible, err := currentProcess(r, pid)
		if err != nil {
			http.Error(w, "port scan failed", http.StatusInternalServerError)
			return 0, false
		}
		if !visible {
			http.Error(w, "process no longer owns a visible socket", http.StatusNotFound)
			return 0, false
		}
		return pid, true
	}
	mux.HandleFunc("GET /api/process/{pid}", func(w http.ResponseWriter, r *http.Request) {
		pid, ok := processRequest(w, r)
		if !ok {
			return
		}
		info, err := procinfo.Load(pid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(info)
	})
	mux.HandleFunc("POST /api/process/{pid}/signal", func(w http.ResponseWriter, r *http.Request) {
		pid, ok := processRequest(w, r)
		if !ok {
			return
		}
		var action struct {
			StartTime string `json:"start_time"`
			Force     bool   `json:"force"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&action); err != nil {
			http.Error(w, "invalid action", http.StatusBadRequest)
			return
		}
		if err := signalProcess(pid, action.StartTime, action.Force); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	var handler http.Handler = mux
	if cfg.Auth != nil {
		handler = requireSession(cfg.Auth, mux)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.AnyHost && !isLoopbackHost(requestHost(r)) {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		// Browsers send Origin on cross-site POSTs; refusing a mismatch
		// stops another site from submitting forms to the dashboard.
		if r.Method == http.MethodPost {
			if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		// Not no-referrer: with it, browsers send "Origin: null" on form
		// posts and the sign-in form would fail the check above.
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'")
		handler.ServeHTTP(w, r)
	})
}

// requireSession serves the sign-in page and its assets to everyone and
// everything else only to signed-in browsers.
func requireSession(auth *webAuth, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/login" && r.Method == http.MethodGet:
			if _, ok := auth.session(r); ok {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			auth.renderLogin(w, r, http.StatusOK, "", "")
			return
		case r.URL.Path == "/login" && r.Method == http.MethodPost:
			auth.handleLogin(w, r)
			return
		case r.URL.Path == "/logout" && r.Method == http.MethodPost:
			auth.signOut(w, r)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		case r.URL.Path == "/login.js" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = io.WriteString(w, loginJS)
			return
		case (r.URL.Path == "/app.css" || r.URL.Path == "/logo.png") && r.Method == http.MethodGet:
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := auth.session(r); !ok {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "sign in required", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// webAddress resolves --web-addr/--web-port into a listen address. Only a
// dashboard that requires sign-in may listen beyond loopback; the returned
// bool reports whether the address is loopback.
func webAddress(address string, port int, addrSet, portSet, allowRemote bool) (string, bool, error) {
	if addrSet && portSet {
		return "", false, fmt.Errorf("use --web-port or --web-addr, not both")
	}
	if portSet {
		if port < 1 || port > 65535 {
			return "", false, fmt.Errorf("--web-port must be between 1 and 65535")
		}
		address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		return "", false, fmt.Errorf("--web-addr must be host:port")
	}
	loopback := isLoopbackHost(host)
	if !loopback {
		if host != "" && net.ParseIP(host) == nil {
			return "", false, fmt.Errorf("--web-addr host must be an IP address or localhost")
		}
		if !allowRemote {
			return "", false, fmt.Errorf("--web-addr must be a loopback address unless --web-auth is set")
		}
	}
	n, err := strconv.Atoi(rawPort)
	if err != nil || n < 1 || n > 65535 {
		return "", false, fmt.Errorf("--web-addr port must be between 1 and 65535")
	}
	return address, loopback, nil
}

type webServe struct {
	Address  string
	Loopback bool
	TLSCert  string
	TLSKey   string
	// Auth, User and Password are set when --web-auth is on. Generated
	// means portop chose the password and must print it.
	Auth      bool
	User      string
	Password  string
	Generated bool
}

func runWeb(stdout, stderr io.Writer, serve webServe, cfg webConfig) int {
	var certificate tls.Certificate
	useTLS := serve.TLSCert != ""
	if useTLS {
		var err error
		certificate, err = tls.LoadX509KeyPair(serve.TLSCert, serve.TLSKey)
		if err != nil {
			fmt.Fprintln(stderr, "portop: web: loading TLS certificate: "+err.Error())
			return 1
		}
	}
	listener, err := net.Listen("tcp", serve.Address)
	if err != nil {
		fmt.Fprintln(stderr, "portop: web: "+err.Error())
		return 1
	}
	defer listener.Close()

	scheme := "http"
	if useTLS {
		scheme = "https"
		listener = tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	}
	hostname, _ := os.Hostname()
	addr := listener.Addr().(*net.TCPAddr)
	shown := listener.Addr().String()
	if addr.IP.IsUnspecified() && hostname != "" {
		shown = net.JoinHostPort(hostname, strconv.Itoa(addr.Port))
	}

	if serve.Auth {
		cfg.Auth = newWebAuth(serve.User, serve.Password, hostname)
		cfg.AnyHost = !serve.Loopback
		fmt.Fprintf(stdout, "portop web: %s://%s/\n", scheme, shown)
		fmt.Fprintf(stdout, "portop web: sign in as %q\n", serve.User)
		if serve.Generated {
			fmt.Fprintf(stdout, "portop web: password: %s (generated for this run; set --web-password or PORTOP_WEB_PASSWORD to choose one)\n", serve.Password)
		}
		if !serve.Loopback && !useTLS {
			fmt.Fprintln(stderr, "portop web: warning: serving plain HTTP beyond loopback; the password and port data are not encrypted. Use --web-tls-cert/--web-tls-key or an SSH tunnel.")
		}
	} else {
		token, err := randomHex(24)
		if err != nil {
			fmt.Fprintln(stderr, "portop: web: could not create action token")
			return 1
		}
		cfg.Token = token
		fmt.Fprintf(stdout, "portop web: %s://%s/#token=%s\n", scheme, shown, token)
	}

	collector := app.NewCollector()
	var mu sync.Mutex
	server := &http.Server{
		Handler: webHandler(func(ctx context.Context, opts app.Options) ([]app.Row, error) {
			mu.Lock()
			defer mu.Unlock()
			return collector.Collect(ctx, opts)
		}, cfg),
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(stderr, "portop: web: "+err.Error())
		return 1
	}
	return 0
}

type webFlags struct {
	addr, user, password, passwordEnv, tlsCert, tlsKey    string
	port                                                  int
	addrSet, portSet, auth, authSet, userSet, passwordSet bool
}

// webServeOptions validates the --web-* flags. Passing a username or
// password turns sign-in on, so --web-auth is only needed to get a
// generated password; --web-auth=false with credentials is a mistake.
func webServeOptions(f webFlags) (webServe, error) {
	credentials := f.userSet || f.passwordSet
	if f.authSet && !f.auth && credentials {
		return webServe{}, fmt.Errorf("--web-user and --web-password need --web-auth")
	}
	serve := webServe{Auth: f.auth || credentials, TLSCert: f.tlsCert, TLSKey: f.tlsKey}
	if (f.tlsCert == "") != (f.tlsKey == "") {
		return webServe{}, fmt.Errorf("--web-tls-cert and --web-tls-key must be used together")
	}
	address, loopback, err := webAddress(f.addr, f.port, f.addrSet, f.portSet, serve.Auth)
	if err != nil {
		return webServe{}, err
	}
	serve.Address, serve.Loopback = address, loopback
	if !serve.Auth {
		return serve, nil
	}
	serve.User = strings.TrimSpace(f.user)
	if serve.User == "" {
		return webServe{}, fmt.Errorf("--web-user must not be empty")
	}
	serve.Password = f.password
	if !f.passwordSet {
		serve.Password = f.passwordEnv
	}
	if f.passwordSet && serve.Password == "" {
		return webServe{}, fmt.Errorf("--web-password must not be empty")
	}
	if serve.Password == "" {
		serve.Password = rand.Text()
		serve.Generated = true
	}
	return serve, nil
}
