package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// loginClient signs admin in with password and code and returns the client.
func loginClient(t *testing.T, handler http.Handler) (*client, string) {
	t.Helper()
	if err := SetPassword("admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, handler: handler, cookies: map[string]*http.Cookie{}}
	c.do("POST", "/api/auth/login", `{"username":"admin","password":"correct horse battery"}`)
	rec := c.do("POST", "/api/auth/2fa/enroll", "")
	var enr Enrollment
	if err := json.Unmarshal(rec.Body.Bytes(), &enr); err != nil {
		t.Fatal(err)
	}
	rec = c.do("POST", "/api/auth/2fa/verify", `{"code":"`+code(t, enr.Secret)+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", rec.Code, rec.Body.String())
	}
	return c, rec.Body.String()
}

func newRememberMux() *http.ServeMux {
	mux := http.NewServeMux()
	Mount(mux, func(*http.Request, string) {})
	mux.HandleFunc("GET /api/plugins", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return mux
}

func TestRememberDevice(t *testing.T) {
	setup(t)
	c, body := loginClient(t, RequireSession(newRememberMux()))
	if !strings.Contains(body, `"remember_offer":true`) {
		t.Fatalf("login body=%s", body)
	}
	if ck := c.cookies[SessionCookie]; ck == nil || ck.MaxAge != 0 {
		t.Fatalf("plain login must set a browser-session cookie: %+v", ck)
	}

	rec := c.do("POST", "/api/auth/remember", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remember status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ck := c.cookies[SessionCookie]; ck == nil || ck.MaxAge != int(rememberTTL.Seconds()) {
		t.Fatalf("remembered cookie = %+v", ck)
	}
	list, err := ListSessions("admin", "")
	if err != nil || len(list) != 1 || !list[0].Remembered {
		t.Fatalf("sessions = %+v err=%v", list, err)
	}

	// Well past the normal 12 h the remembered session still works ...
	advance(20 * 24 * time.Hour)
	if rec := c.do("GET", "/api/plugins", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("remembered session after 20 days: %d", rec.Code)
	}
	// ... but changes need a new code: the login window is long closed.
	if rec := c.do("POST", "/api/auth/sessions/revoke-others", ""); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"step_up_required":true`) {
		t.Fatalf("change without code: %d %s", rec.Code, rec.Body.String())
	}
	// And it ends after rememberTTL.
	advance(11 * 24 * time.Hour)
	if rec := c.do("GET", "/api/plugins", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("remembered session after 31 days: %d", rec.Code)
	}
}

func TestRememberOnlyRightAfterLogin(t *testing.T) {
	setup(t)
	c, _ := loginClient(t, RequireSession(newRememberMux()))
	advance(rememberWindow + time.Minute)
	rec := c.do("POST", "/api/auth/remember", "")
	if rec.Code == http.StatusOK {
		t.Fatal("session extended long after the login")
	}
	list, _ := ListSessions("admin", "")
	if len(list) != 1 || list[0].Remembered {
		t.Fatalf("sessions = %+v", list)
	}
}

func TestRememberRefusedOnShortLivedAccess(t *testing.T) {
	setup(t)
	c, body := loginClient(t, NoRemember(RequireSession(newRememberMux())))
	if !strings.Contains(body, `"remember_offer":false`) {
		t.Fatalf("login body=%s", body)
	}
	if rec := c.do("POST", "/api/auth/remember", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("remember on remote access: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRememberedSessionEndsWithPasswordChange(t *testing.T) {
	setup(t)
	c, _ := loginClient(t, RequireSession(newRememberMux()))
	c.do("POST", "/api/auth/remember", "")
	if err := SetPassword("admin", "another long passphrase"); err != nil {
		t.Fatal(err)
	}
	if rec := c.do("GET", "/api/plugins", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("remembered session survived password change: %d", rec.Code)
	}
}
