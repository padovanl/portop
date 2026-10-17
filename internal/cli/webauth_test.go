package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/padovanl/portop/internal/app"
)

func authHandler(t *testing.T, anyHost bool) (http.Handler, *webAuth) {
	t.Helper()
	auth := newWebAuth("admin", "correct horse", "testhost")
	h := webHandler(func(context.Context, app.Options) ([]app.Row, error) { return nil, nil },
		webConfig{Auth: auth, AnyHost: anyHost})
	return h, auth
}

func serve(h http.Handler, method, target string, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func login(h http.Handler, user, password string) *httptest.ResponseRecorder {
	form := url.Values{"username": {user}, "password": {password}}
	return serve(h, http.MethodPost, "http://127.0.0.1/login", form.Encode(), nil)
}

func sessionCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatalf("no session cookie in %v", rec.Header())
	return nil
}

func TestWebAuthRequiresSignIn(t *testing.T) {
	h, _ := authHandler(t, false)

	if rec := serve(h, http.MethodGet, "http://127.0.0.1/", "", nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("dashboard without session: %d %v", rec.Code, rec.Header())
	}
	for _, path := range []string{"/api/ports", "/api/session", "/api/process/1"} {
		if rec := serve(h, http.MethodGet, "http://127.0.0.1"+path, "", nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without session: %d", path, rec.Code)
		}
	}
	for _, path := range []string{"/login", "/login.js", "/app.css", "/logo.png"} {
		if rec := serve(h, http.MethodGet, "http://127.0.0.1"+path, "", nil); rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Errorf("%s should be public: %d", path, rec.Code)
		}
	}
	page := serve(h, http.MethodGet, "http://127.0.0.1/login", "", nil).Body.String()
	if !strings.Contains(page, `action="/login"`) || !strings.Contains(page, "testhost") || !strings.Contains(page, "Local connection") {
		t.Fatalf("login page content:\n%s", page)
	}
}

func TestWebAuthSession(t *testing.T) {
	h, auth := authHandler(t, false)

	rec := login(h, "admin", "wrong")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Incorrect username or password") {
		t.Fatalf("wrong password: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `value="admin"`) {
		t.Error("username should be kept after a failed sign-in")
	}

	rec = login(h, "admin", "correct horse")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("sign in: %d %v", rec.Code, rec.Header())
	}
	cookie := sessionCookieFrom(t, rec)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie attributes: %+v", cookie)
	}

	if rec := serve(h, http.MethodGet, "http://127.0.0.1/", "", cookie); rec.Code != http.StatusOK {
		t.Fatalf("dashboard with session: %d", rec.Code)
	}
	if rec := serve(h, http.MethodGet, "http://127.0.0.1/login", "", cookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("login page while signed in: %d", rec.Code)
	}
	rec = serve(h, http.MethodGet, "http://127.0.0.1/api/session", "", cookie)
	var session map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil || session["user"] != "admin" || session["host"] != "testhost" || len(session["token"]) < 32 {
		t.Fatalf("session: %s, %v", rec.Body.String(), err)
	}

	// The session cookie alone must not be enough to act on a process.
	if rec := serve(h, http.MethodGet, "http://127.0.0.1/api/process/1", "", cookie); rec.Code != http.StatusForbidden {
		t.Fatalf("process request without token: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/process/1", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-Portop-Token", session["token"])
	withToken := httptest.NewRecorder()
	h.ServeHTTP(withToken, req)
	if withToken.Code != http.StatusNotFound {
		t.Fatalf("process request with token reaches the handler: %d", withToken.Code)
	}

	if rec := serve(h, http.MethodPost, "http://127.0.0.1/logout", "", cookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec := serve(h, http.MethodGet, "http://127.0.0.1/api/ports", "", cookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("session should end at logout: %d", rec.Code)
	}

	rec = login(h, "admin", "correct horse")
	cookie = sessionCookieFrom(t, rec)
	auth.now = func() time.Time { return time.Now().Add(sessionLifetime + time.Minute) }
	if rec := serve(h, http.MethodGet, "http://127.0.0.1/api/ports", "", cookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session: %d", rec.Code)
	}
}

func TestWebAuthLockout(t *testing.T) {
	h, auth := authHandler(t, false)
	start := time.Now()
	auth.now = func() time.Time { return start }
	for i := 0; i < maxLoginFailures; i++ {
		if rec := login(h, "admin", "nope"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i+1, rec.Code)
		}
	}
	rec := login(h, "admin", "correct horse")
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "Too many failed attempts") {
		t.Fatalf("locked out client: %d", rec.Code)
	}
	auth.now = func() time.Time { return start.Add(loginLockout + time.Second) }
	if rec := login(h, "admin", "correct horse"); rec.Code != http.StatusSeeOther {
		t.Fatalf("after the lockout: %d", rec.Code)
	}
}

func TestWebAuthHostAndOrigin(t *testing.T) {
	local, _ := authHandler(t, false)
	if rec := serve(local, http.MethodGet, "http://attacker.example/login", "", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign host on a loopback server: %d", rec.Code)
	}

	remote, _ := authHandler(t, true)
	rec := serve(remote, http.MethodGet, "http://server.lan:8088/login", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Unencrypted HTTP") {
		t.Fatalf("remote login page: %d", rec.Code)
	}

	form := url.Values{"username": {"admin"}, "password": {"correct horse"}}
	req := httptest.NewRequest(http.MethodPost, "http://server.lan:8088/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://attacker.example")
	cross := httptest.NewRecorder()
	remote.ServeHTTP(cross, req)
	if cross.Code != http.StatusForbidden {
		t.Fatalf("cross-origin sign-in: %d", cross.Code)
	}
	req.Header.Set("Origin", "http://server.lan:8088")
	req.Body = io.NopCloser(strings.NewReader(form.Encode()))
	same := httptest.NewRecorder()
	remote.ServeHTTP(same, req)
	if same.Code != http.StatusSeeOther || len(same.Result().Cookies()) == 0 {
		t.Fatalf("same-origin sign-in did not set a session: %d", same.Code)
	}
}

func TestWebNoAuthHasNoSession(t *testing.T) {
	h := webHandler(func(context.Context, app.Options) ([]app.Row, error) { return nil, nil }, webConfig{Token: "t"})
	if rec := serve(h, http.MethodGet, "http://127.0.0.1/api/session", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("session endpoint without auth: %d", rec.Code)
	}
	if rec := serve(h, http.MethodGet, "http://127.0.0.1/login", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("login without auth: %d", rec.Code)
	}
}

// Every IPv6 client controls at least a /64, so a new source address per
// guess must not reset the lockout.
func TestWebAuthLockoutCoversIPv6Prefix(t *testing.T) {
	h, auth := authHandler(t, false)
	start := time.Now()
	auth.now = func() time.Time { return start }
	attempt := func(remote, password string) int {
		form := url.Values{"username": {"admin"}, "password": {password}}
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < maxLoginFailures; i++ {
		remote := fmt.Sprintf("[2001:db8:1:2::%x]:40000", i+1)
		if code := attempt(remote, "nope"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i+1, code)
		}
	}
	if code := attempt("[2001:db8:1:2::ffff]:40000", "correct horse"); code != http.StatusTooManyRequests {
		t.Fatalf("new address in the same /64: %d, want the lockout", code)
	}
	if code := attempt("[2001:db8:1:3::1]:40000", "correct horse"); code != http.StatusSeeOther {
		t.Fatalf("another /64: %d, want a normal sign-in", code)
	}
	if code := attempt("192.0.2.7:40000", "correct horse"); code != http.StatusSeeOther {
		t.Fatalf("IPv4 client: %d, want a normal sign-in", code)
	}
}
