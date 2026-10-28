package mail

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"godeb/backend/core"
)

func TestValidation(t *testing.T) {
	for _, bad := range []string{"", "-x", "x-", "a..b", "a b", "ä", "a@b", "a/b", "../x", strings.Repeat("a", 65)} {
		if _, err := NormalizeLocal(bad); err == nil {
			t.Errorf("local %q accepted", bad)
		}
	}
	for _, good := range []string{"info", "Anna.Meier", "a+b", "x_y-z", "1"} {
		if _, err := NormalizeLocal(good); err != nil {
			t.Errorf("local %q rejected: %v", good, err)
		}
	}
	for _, bad := range []string{"localhost", "-a.com", "a..com", "a.com/x", "a_b.com"} {
		if _, err := NormalizeDomain(bad); err == nil {
			t.Errorf("domain %q accepted", bad)
		}
	}
	for _, bad := range []string{"Name <a@b.de>", "a@b", "a@@b.de", "a,b@c.de", "a@b.de\nBcc: x@y.de"} {
		if _, err := NormalizeAddress(bad); err == nil {
			t.Errorf("address %q accepted", bad)
		}
	}
	if _, err := cleanAccount(Account{Domain: "example.com", Local: "shop", ForwardOnly: true}); err == nil {
		t.Error("forward-only without target accepted")
	}
	a, err := cleanAccount(Account{Domain: "Example.COM", Local: "Info", Aliases: []string{"info", "Kontakt", "kontakt", ""}, Forward: []string{"info@example.com", "X@Gmail.com"}})
	if err != nil || strings.Join(a.Aliases, ",") != "kontakt" || strings.Join(a.Forward, ",") != "x@gmail.com" || a.Domain != "example.com" {
		t.Fatalf("clean = %+v %v", a, err)
	}
	if err := checkPassword("short"); err == nil {
		t.Error("short password accepted")
	}
	if err := checkPassword("long-enough\x00pw"); err == nil {
		t.Error("control character accepted")
	}
}

func TestMapsAndUsers(t *testing.T) {
	_, domains, accounts := exampleSetup(t)
	dom, mbox, ali, snd := postfixMaps(domains, accounts)
	if !strings.Contains(dom, "example.com OK") || strings.Contains(dom, "old.example") {
		t.Fatalf("suspended domain must not receive mail:\n%s", dom)
	}
	if strings.Contains(mbox, "shop@") || !strings.Contains(mbox, "info@example.com example.com/info/") {
		t.Fatalf("mailboxes:\n%s", mbox)
	}
	for _, want := range []string{
		"info@example.com info@example.com\n",                     // identity before the catch-all
		"anna@example.com anna@example.com, backup@example.org\n", // keep a copy
		"shop@example.com info@example.com, x@gmail.com\n",        // forward only
		"kontakt@example.com info@example.com\n",
		"@example.com info@example.com\n",
	} {
		if !strings.Contains(ali, want) {
			t.Errorf("aliases lack %q:\n%s", want, ali)
		}
	}
	if !strings.Contains(snd, "kontakt@example.com info@example.com") {
		t.Errorf("owner may not send as alias:\n%s", snd)
	}
	users, deny := dovecotUsersFile(domains, accounts)
	if strings.Contains(users, "shop@") || !strings.Contains(users, "userdb_quota_storage_size=1024M") {
		t.Fatalf("users:\n%s", users)
	}
	// Suspended account and every account of a suspended domain: no login.
	if !strings.Contains(deny, "backup@example.org:") || !strings.Contains(deny, "alt@old.example:") || strings.Contains(deny, "info@") {
		t.Fatalf("deny:\n%s", deny)
	}
	for _, line := range strings.Split(strings.TrimSpace(users), "\n")[1:] {
		if strings.Count(line, ":") < 7 || !strings.Contains(line, ":{ARGON2ID}$argon2id$") {
			t.Errorf("passwd-file line malformed: %s", line)
		}
	}
}

func TestDovecotConfigIsClosed(t *testing.T) {
	c := dovecotConfig(Settings{Hostname: "mail.example.com", CertFile: "/c.pem", KeyFile: "/k.pem"})
	for _, want := range []string{"auth_allow_cleartext = no", "ssl = required", "port = 0", "deny = yes", "ssl_server_cert_file = /c.pem", "vacation = yes", "active_path = ~/.dovecot.sieve"} {
		if !strings.Contains(c, want) {
			t.Errorf("dovecot.conf lacks %q", want)
		}
	}
	// No PAM, no system users, no includes of Debian's conf.d.
	for _, bad := range []string{"passdb pam", "driver = pam", "!include", "userdb passwd {", "pop3"} {
		if strings.Contains(c, bad) {
			t.Errorf("dovecot.conf contains %q", bad)
		}
	}
	_, params := postfixMaster()
	if !contains(params, "submission/inet/smtpd_sender_restrictions=reject_sender_login_mismatch,permit_sasl_authenticated,reject") {
		t.Error("submission must enforce the sender login")
	}
	for _, p := range postfixMain(Settings{Hostname: "h", CertFile: "c", KeyFile: "k"}) {
		if strings.HasPrefix(p, "smtpd_sasl_auth_enable=") && p != "smtpd_sasl_auth_enable=no" {
			t.Error("SASL must be off on port 25")
		}
	}
}

func TestConflicts(t *testing.T) {
	_, _, accounts := exampleSetup(t)
	if err := checkAccount(accounts, Account{Domain: "example.com", Local: "kontakt"}, ""); err == nil {
		t.Error("address taken by an alias accepted")
	}
	if err := checkAccount(accounts, Account{Domain: "example.com", Local: "neu", Aliases: []string{"anna"}}, ""); err == nil {
		t.Error("alias taken by a mailbox accepted")
	}
	info := accounts[0]
	if err := checkAccount(accounts, info, info.Address()); err != nil {
		t.Errorf("editing an account conflicts with itself: %v", err)
	}
}

type fakeResolver struct{}

func (fakeResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return []*net.MX{{Host: "mail.example.com.", Pref: 10}}, nil
}
func (fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	switch {
	case name == "example.com":
		return []string{"google-site-verification=x", "v=spf1 mx ip4:203.0.113.9 -all"}, nil
	case strings.HasPrefix(name, "godeb._domainkey."):
		return []string{"v=DKIM1; k=rsa; p=OTHERKEY"}, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}
func (fakeResolver) LookupHost(context.Context, string) ([]string, error) {
	return []string{"203.0.113.7"}, nil
}
func (fakeResolver) LookupAddr(context.Context, string) ([]string, error) {
	return nil, errors.New("timeout")
}

func TestDNSCheck(t *testing.T) {
	dnsResolver = fakeResolver{}
	t.Cleanup(func() { dnsResolver = net.DefaultResolver })
	s := Settings{Hostname: "mail.example.com"}
	recs := checkRecords(context.Background(), expectedRecords(s, Domain{Name: "example.com", Selector: "godeb", DKIMPublic: "MYKEY"}, "203.0.113.7"))
	got := map[string]string{}
	for _, r := range recs {
		got[r.Kind] = r.Status
	}
	want := map[string]string{"mx": "ok", "spf": "ok", "dkim": "wrong", "dmarc": "missing", "a": "ok", "ptr": "unknown"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %s, want %s", k, got[k], v)
		}
	}
	if !strings.Contains(recs[2].Zone, `"v=DKIM1; k=rsa; p=MYKEY"`) {
		t.Errorf("zone line = %s", recs[2].Zone)
	}
	long := strings.Repeat("A", 400)
	if z := splitTXT(long); strings.Count(z, `"`) != 4 {
		t.Errorf("TXT not split: %s", z)
	}
}

func TestParseQuota(t *testing.T) {
	out := "username\tQuota name\tType\tValue\tLimit\t%\ninfo@example.com\tUser quota\tSTORAGE\t2048\t1048576\t0\ninfo@example.com\tUser quota\tMESSAGE\t12\t-\t0\n"
	q := parseQuota(out)
	if q["info@example.com"] != 2048 || len(q) != 1 {
		t.Fatalf("quota = %v", q)
	}
}

func TestFileSafety(t *testing.T) {
	t.Setenv("GODEB_MAIL_ROOT", t.TempDir())
	if err := writeFile("/etc/passwd", "x"); err == nil {
		t.Fatal("file outside the allowlist written")
	}
	if err := writeFile(mapDomains, "example.com OK\n"); err != nil {
		t.Fatal(err)
	}
	if err := removeMailbox("example.com", "../../etc"); err == nil {
		t.Fatal("path traversal accepted")
	}
	if err := writeDKIM("../x", "godeb", "k"); err == nil {
		t.Fatal("bad DKIM domain accepted")
	}
	dir := root(filepath.Join(vmailDir, "example.com", "info"))
	_ = os.MkdirAll(dir, 0o755)
	if err := removeMailbox("example.com", "info"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("mailbox not removed")
	}
}

// TestHandlers runs the API against a temporary database and root; system
// commands are recorded instead of run.
func TestHandlers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_MAIL_ROOT", dir)
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_DB", filepath.Join(dir, "godeb.db"))
	t.Setenv("GODEB_MASTER_KEY", filepath.Join(dir, "master.key"))
	t.Setenv("GODEB_AGENT_MASTER_KEY", filepath.Join(dir, "agent.key"))
	var calls []string
	admin = func(_ context.Context, name string, args ...string) core.CommandResult {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return core.CommandResult{}
	}
	adminTimeout = func(ctx context.Context, _ time.Duration, name string, args ...string) core.CommandResult {
		return admin(ctx, name, args...)
	}
	t.Cleanup(func() { admin, adminTimeout = core.RunAdmin, core.RunAdminTimeout })
	if err := saveSettings(Settings{Hostname: "mail.example.com", CertFile: snakeoilCert, KeyFile: snakeoilKey}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Mount(mux)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := do("POST", base+"/domains", `{"name":"Example.com"}`); rec.Code != http.StatusCreated {
		t.Fatalf("add domain: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(root(dkimKeyPath("example.com", "godeb"))); err != nil {
		t.Fatalf("DKIM key not written: %v", err)
	}
	if rec := do("POST", base+"/domains/example.com/accounts", `{"local":"info","password":"Correct-Horse-9","quotaMB":500,"aliases":["kontakt"]}`); rec.Code != http.StatusCreated {
		t.Fatalf("add account: %d %s", rec.Code, rec.Body)
	}
	if rec := do("POST", base+"/domains/example.com/accounts", `{"local":"kontakt","password":"Correct-Horse-9"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate via alias: %d", rec.Code)
	}
	if rec := do("POST", base+"/domains/example.com/accounts", `{"local":"x","password":"short"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("short password: %d", rec.Code)
	}
	users, _ := os.ReadFile(root(dovecotUsers))
	if !strings.Contains(string(users), "info@example.com:{ARGON2ID}") || strings.Contains(string(users), "Correct-Horse-9") {
		t.Fatalf("users file:\n%s", users)
	}
	// Edit without a password keeps the old hash.
	if rec := do("POST", base+"/domains/example.com/accounts/info", `{"quotaMB":0,"aliases":[],"suspended":true}`); rec.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	deny, _ := os.ReadFile(root(dovecotDeny))
	users2, _ := os.ReadFile(root(dovecotUsers))
	if !strings.Contains(string(deny), "info@example.com:") || string(users2) == "" || !strings.Contains(string(users2), "info@example.com:{ARGON2ID}") {
		t.Fatalf("suspend: deny=%s users=%s", deny, users2)
	}
	if rec := do("POST", base+"/domains/example.com", `{"catchAll":"nobody@example.com"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("catch-all to a missing address: %d", rec.Code)
	}
	rec := do("GET", base+"/domains/example.com", "")
	var view struct {
		Accounts []struct {
			Address string `json:"address"`
			Hash    string `json:"hash"`
		} `json:"accounts"`
		Records []Record `json:"records"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || len(view.Accounts) != 1 || strings.Contains(rec.Body.String(), "argon2") {
		t.Fatalf("domain view leaks or is wrong: %s", rec.Body)
	}
	if !contains(calls, "systemctl reload-or-restart postfix") || !containsPrefix(calls, "postconf -e myhostname=mail.example.com") {
		t.Fatalf("calls = %v", calls)
	}
	if rec := do("POST", base+"/domains/example.com/accounts/info/delete", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete account: %d", rec.Code)
	}
	if rec := do("POST", base+"/domains/example.com/delete", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete domain: %d", rec.Code)
	}
	if _, err := os.Stat(root(dkimKeyPath("example.com", "godeb"))); !os.IsNotExist(err) {
		t.Fatal("DKIM key left behind")
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func containsPrefix(list []string, p string) bool {
	for _, v := range list {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}
