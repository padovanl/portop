package cli

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

//go:embed web/login.html
var loginPage string

//go:embed web/login.js
var loginJS string

var loginTemplate = template.Must(template.New("login").Parse(loginPage))

const (
	sessionCookie    = "portop_session"
	sessionLifetime  = 12 * time.Hour
	maxLoginFailures = 5
	loginLockout     = time.Minute
)

type webSession struct {
	csrf    string
	expires time.Time
}

type loginFailures struct {
	count int
	last  time.Time
}

// webAuth holds the credentials and the in-memory sessions of a dashboard
// started with --web-auth. Sessions do not survive a restart, which is the
// point: stopping portop signs everyone out.
type webAuth struct {
	user         string
	userHash     [32]byte
	passwordHash [32]byte
	host         string
	now          func() time.Time

	mu       sync.Mutex
	sessions map[string]webSession
	failures map[string]loginFailures
}

func newWebAuth(user, password, host string) *webAuth {
	return &webAuth{
		user:         user,
		userHash:     sha256.Sum256([]byte(user)),
		passwordHash: sha256.Sum256([]byte(password)),
		host:         host,
		now:          time.Now,
		sessions:     map[string]webSession{},
		failures:     map[string]loginFailures{},
	}
}

// credentialsMatch hashes both values first so the comparison takes the
// same time regardless of how long the submitted strings are.
func (a *webAuth) credentialsMatch(user, password string) bool {
	u := sha256.Sum256([]byte(user))
	p := sha256.Sum256([]byte(password))
	userOK := subtle.ConstantTimeCompare(u[:], a.userHash[:])
	passwordOK := subtle.ConstantTimeCompare(p[:], a.passwordHash[:])
	return userOK&passwordOK == 1
}

func (a *webAuth) session(r *http.Request) (webSession, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return webSession{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[cookie.Value]
	if !ok {
		return webSession{}, false
	}
	if !a.now().Before(s.expires) {
		delete(a.sessions, cookie.Value)
		return webSession{}, false
	}
	return s, true
}

// lockedFor reports how long a client must wait after too many failed
// sign-ins. It is keyed by clientKey, so one noisy client does not lock
// out the others.
func (a *webAuth) lockedFor(ip string) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	f, ok := a.failures[ip]
	if !ok || f.count < maxLoginFailures {
		return 0
	}
	if wait := f.last.Add(loginLockout).Sub(a.now()); wait > 0 {
		return wait
	}
	delete(a.failures, ip)
	return 0
}

func (a *webAuth) recordFailure(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for key, f := range a.failures {
		if now.Sub(f.last) > loginLockout {
			delete(a.failures, key)
		}
	}
	f := a.failures[ip]
	f.count++
	f.last = now
	a.failures[ip] = f
}

func (a *webAuth) signIn(w http.ResponseWriter, r *http.Request, ip string) error {
	id, err := randomHex(32)
	if err != nil {
		return err
	}
	csrf, err := randomHex(24)
	if err != nil {
		return err
	}
	a.mu.Lock()
	now := a.now()
	for key, s := range a.sessions {
		if !now.Before(s.expires) {
			delete(a.sessions, key)
		}
	}
	a.sessions[id] = webSession{csrf: csrf, expires: now.Add(sessionLifetime)}
	delete(a.failures, ip)
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   int(sessionLifetime / time.Second),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (a *webAuth) signOut(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
}

type loginView struct {
	Host      string
	User      string
	Error     string
	Transport string
	Warning   bool
}

func (a *webAuth) renderLogin(w http.ResponseWriter, r *http.Request, status int, user, message string) {
	view := loginView{Host: a.host, User: user, Error: message}
	switch {
	case r.TLS != nil:
		view.Transport = "Encrypted connection (HTTPS)"
	case isLoopbackHost(requestHost(r)):
		view.Transport = "Local connection on this machine"
	default:
		view.Transport = "Unencrypted HTTP: your password is sent in clear text"
		view.Warning = true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = loginTemplate.Execute(w, view)
}

func (a *webAuth) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientKey(r)
	if wait := a.lockedFor(ip); wait > 0 {
		seconds := int(wait.Round(time.Second) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		a.renderLogin(w, r, http.StatusTooManyRequests, "", "Too many failed attempts. Try again in "+strconv.Itoa(seconds)+" s.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		a.renderLogin(w, r, http.StatusBadRequest, "", "The sign-in form could not be read.")
		return
	}
	user := r.PostForm.Get("username")
	if !a.credentialsMatch(user, r.PostForm.Get("password")) {
		a.recordFailure(ip)
		a.renderLogin(w, r, http.StatusUnauthorized, user, "Incorrect username or password.")
		return
	}
	if err := a.signIn(w, r, ip); err != nil {
		a.renderLogin(w, r, http.StatusInternalServerError, user, "Could not start a session.")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// basicAuth checks HTTP Basic credentials and answers the request itself
// when they are missing, wrong or locked out.
func (a *webAuth) basicAuth(w http.ResponseWriter, r *http.Request) bool {
	ip := clientKey(r)
	if wait := a.lockedFor(ip); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Round(time.Second)/time.Second)+1))
		http.Error(w, "too many failed attempts", http.StatusTooManyRequests)
		return false
	}
	user, password, ok := r.BasicAuth()
	if ok && a.credentialsMatch(user, password) {
		return true
	}
	if ok {
		a.recordFailure(ip)
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="portop", charset="UTF-8"`)
	http.Error(w, "sign in required", http.StatusUnauthorized)
	return false
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// clientKey identifies a client for the sign-in lockout: its IPv4
// address, or the /64 prefix of its IPv6 address. Anyone holding an IPv6
// address holds at least a /64, so keying by the full address would let
// a client pick a new one for every guess and never be locked out.
func clientKey(r *http.Request) string {
	ip := net.ParseIP(remoteIP(r))
	if ip == nil || ip.To4() != nil {
		return remoteIP(r)
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func requestHost(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.Host); err == nil {
		return host
	}
	return r.Host
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}
