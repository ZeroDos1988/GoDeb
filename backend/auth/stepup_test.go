package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func (c *client) doCode(method, path, body, code string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "192.0.2.50:5555"
	req.Header.Set(StepUpHeader, code)
	for _, cookie := range c.cookies {
		if strings.HasPrefix(path, cookie.Path) {
			req.AddCookie(cookie)
		}
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	for _, cookie := range rec.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(c.cookies, cookie.Name)
			continue
		}
		c.cookies[cookie.Name] = cookie
	}
	return rec
}

// signedIn returns a client with a full session and the user's TOTP secret.
func signedIn(t *testing.T, failures *[]string) (*client, string) {
	t.Helper()
	if err := SetPassword("admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Mount(mux, func(r *http.Request, username string) { *failures = append(*failures, username) })
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
	mux.HandleFunc("GET /api/plugins", ok)
	mux.HandleFunc("POST /api/plugins/fail2ban/unban", ok)
	mux.HandleFunc("POST /api/plugins/ufw/rules", ok)
	mux.HandleFunc("GET /api/plugins/wireguard/peers/{id}/config", ok)
	c := &client{t: t, handler: RequireSession(mux), cookies: map[string]*http.Cookie{}}
	c.do("POST", "/api/auth/login", `{"username":"admin","password":"correct horse battery"}`)
	var enr Enrollment
	if err := json.Unmarshal(c.do("POST", "/api/auth/2fa/enroll", "").Body.Bytes(), &enr); err != nil {
		t.Fatal(err)
	}
	if rec := c.do("POST", "/api/auth/2fa/verify", `{"code":"`+code(t, enr.Secret)+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", rec.Code, rec.Body.String())
	}
	return c, enr.Secret
}

func TestStepUpLevels(t *testing.T) {
	cases := []struct {
		method, path string
		want         stepUpLevel
	}{
		{"GET", "/api/plugins/ufw/status", stepUpNone},
		{"GET", "/api/plugins/wireguard/peers/laptop/config", stepUpWindow},
		{"GET", "/api/plugins/wireguard/peers/laptop/qr", stepUpWindow},
		{"GET", "/api/plugins/wireguard/peers/heim-vm/code", stepUpWindow},
		{"POST", "/api/user/settings", stepUpNone},
		{"POST", "/api/plugin-manager/scan", stepUpNone},
		{"POST", "/api/plugins/fail2ban/unban", stepUpWindow},
		{"POST", "/api/plugins/new-plugin/anything", stepUpWindow},
		{"PUT", "/api/plugins/steam/games/enshrouded/config", stepUpWindow},
		{"POST", "/api/plugins/steam/games/enshrouded/action", stepUpWindow},
		{"POST", "/api/plugins/steam/games/enshrouded/install", stepUpFresh},
		{"POST", "/api/plugins/steam/games/valheim/update", stepUpFresh},
		{"POST", "/api/plugins/steam/games/x/../enshrouded/install", stepUpFresh},
		{"POST", "/api/auth/sessions/revoke-others", stepUpWindow},
		{"POST", "/api/plugin-manager/apt/install", stepUpFresh},
		{"POST", "/api/plugins/ufw/rules", stepUpFresh},
		{"POST", "/api/plugins/systemd/action", stepUpFresh},
		{"PUT", "/api/auth/oidc/providers/google", stepUpFresh},
		{"POST", "/api/plugins/wireguard/peers", stepUpFresh},
		{"POST", "/api/plugins/wireguard/peers/laptop/send-email", stepUpWindow},
		{"POST", "/api/plugins/x/../ufw/rules", stepUpFresh},
		{"DELETE", "/api/plugins/ufw/rules/3", stepUpFresh},
		{"POST", "/api/plugins/wireguard/join", stepUpFresh},
		{"POST", "/api/plugins/wireguard/trust", stepUpFresh},
		{"POST", "/api/plugins/wireguard/trust/sync", stepUpWindow},
		{"POST", "/api/plugins/portforward/rules", stepUpFresh},
		{"POST", "/api/plugins/portforward/rules/enshrouded/delete", stepUpFresh},
		{"GET", "/api/plugins/portforward/status", stepUpNone},
	}
	for _, tc := range cases {
		if got := stepUpFor(tc.method, tc.path); got != tc.want {
			t.Errorf("%s %s = %d want %d", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestStepUpWindowAndFreshCodes(t *testing.T) {
	setup(t)
	var failures []string
	c, secret := signedIn(t, &failures)

	// The login code opens the window, so ordinary actions work right away.
	if rec := c.do("POST", "/api/plugins/fail2ban/unban", "{}"); rec.Code != http.StatusNoContent {
		t.Fatalf("window after login: %d %s", rec.Code, rec.Body.String())
	}
	// Critical actions never ride on the window.
	rec := c.do("POST", "/api/plugins/ufw/rules", "{}")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"step_up_required":true`) || !strings.Contains(rec.Body.String(), `"critical":true`) {
		t.Fatalf("critical without code: %d %s", rec.Code, rec.Body.String())
	}

	advance(elevationTTL + time.Second)
	rec = c.do("POST", "/api/plugins/fail2ban/unban", "{}")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"critical":false`) {
		t.Fatalf("expired window: %d %s", rec.Code, rec.Body.String())
	}
	if rec := c.do("GET", "/api/plugins/wireguard/peers/laptop/config", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("key download without code: %d", rec.Code)
	}
	if rec := c.do("GET", "/api/plugins", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("plain read blocked: %d", rec.Code)
	}

	current := code(t, secret)
	if rec := c.doCode("POST", "/api/plugins/ufw/rules", "{}", current); rec.Code != http.StatusNoContent {
		t.Fatalf("critical with code: %d %s", rec.Code, rec.Body.String())
	}
	// The same code cannot confirm a second critical action.
	if rec := c.doCode("POST", "/api/plugins/ufw/rules", "{}", current); rec.Code != http.StatusForbidden {
		t.Fatalf("replayed code accepted: %d", rec.Code)
	}
	// But the code opened the window for ordinary actions.
	if rec := c.do("POST", "/api/plugins/fail2ban/unban", "{}"); rec.Code != http.StatusNoContent {
		t.Fatalf("window after step-up: %d", rec.Code)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %v", failures)
	}
}

func TestStepUpFailuresEndSession(t *testing.T) {
	setup(t)
	var failures []string
	c, _ := signedIn(t, &failures)
	for i := 1; i < maxCodeAttempts; i++ {
		rec := c.doCode("POST", "/api/plugins/ufw/rules", "{}", "000000")
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"remaining":`) {
			t.Fatalf("attempt %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if rec := c.doCode("POST", "/api/plugins/ufw/rules", "{}", "000000"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("last attempt: %d", rec.Code)
	}
	if rec := c.do("GET", "/api/plugins", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("session survived brute force: %d", rec.Code)
	}
	if len(failures) != maxCodeAttempts {
		t.Fatalf("failures = %d", len(failures))
	}
}

func TestPendingLoginAttemptLimit(t *testing.T) {
	setup(t)
	var failures []string
	_, _ = signedIn(t, &failures)
	mux := http.NewServeMux()
	Mount(mux, nil)
	c := &client{t: t, handler: RequireSession(mux), cookies: map[string]*http.Cookie{}}
	c.do("POST", "/api/auth/login", `{"username":"admin","password":"correct horse battery"}`)
	for i := 1; i < maxCodeAttempts; i++ {
		if rec := c.do("POST", "/api/auth/2fa/authenticate", `{"code":"000000"}`); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, rec.Code)
		}
	}
	if rec := c.do("POST", "/api/auth/2fa/authenticate", `{"code":"000000"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("last attempt: %d", rec.Code)
	}
	if rec := c.do("POST", "/api/auth/2fa/authenticate", `{"code":"000000"}`); rec.Code != http.StatusUnauthorized ||
		!strings.Contains(rec.Body.String(), "Passwort") {
		t.Fatalf("pending login survived: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionsRevocable(t *testing.T) {
	setup(t)
	var failures []string
	c, secret := signedIn(t, &failures)

	// A copied cookie stops working once the session is logged out.
	stolen := *c.cookies[SessionCookie]
	c.do("POST", "/api/auth/logout", "")
	thief := &client{t: t, handler: c.handler, cookies: map[string]*http.Cookie{SessionCookie: &stolen}}
	if rec := thief.do("GET", "/api/plugins", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("logged-out cookie still valid: %d", rec.Code)
	}

	// Two fresh sessions; "end all others" keeps only the caller.
	advance(time.Minute)
	c.do("POST", "/api/auth/login", `{"username":"admin","password":"correct horse battery"}`)
	c.do("POST", "/api/auth/2fa/authenticate", `{"code":"`+code(t, secret)+`"}`)
	other := &client{t: t, handler: c.handler, cookies: map[string]*http.Cookie{}}
	advance(time.Minute)
	other.do("POST", "/api/auth/login", `{"username":"admin","password":"correct horse battery"}`)
	if rec := other.do("POST", "/api/auth/2fa/authenticate", `{"code":"`+code(t, secret)+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("second login: %d %s", rec.Code, rec.Body.String())
	}

	var list struct {
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(c.do("GET", "/api/auth/sessions", "").Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Sessions) != 2 {
		t.Fatalf("sessions = %+v", list.Sessions)
	}
	if rec := c.do("POST", "/api/auth/sessions/revoke-others", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"revoked":1`) {
		t.Fatalf("revoke-others: %d %s", rec.Code, rec.Body.String())
	}
	if rec := other.do("GET", "/api/plugins", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session still valid: %d", rec.Code)
	}
	if rec := c.do("GET", "/api/plugins", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("own session revoked: %d", rec.Code)
	}

	if n, err := RevokeSessions("admin", ""); err != nil || n != 1 {
		t.Fatalf("cli revoke n=%d err=%v", n, err)
	}
	if rec := c.do("GET", "/api/plugins", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("session survived cli revoke: %d", rec.Code)
	}
}
