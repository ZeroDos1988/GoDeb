package mailgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeRelayAndDomain(t *testing.T) {
	host, err := normalizeRelay("10.8.0.2")
	if err != nil || host != "10.8.0.2" {
		t.Fatalf("tunnel relay: %v %q", err, host)
	}
	if _, err := normalizeRelay(""); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"8.8.8.8", "10.9.0.2", "10.8.1.4", "example.org"} {
		if _, err := normalizeRelay(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	domain, err := normalizeDomain("Mail.Example.ORG.")
	if err != nil || domain != "mail.example.org" {
		t.Fatalf("domain: %v %q", err, domain)
	}
	if _, err := normalizeDomain("localhost"); err == nil {
		t.Fatal("single label accepted")
	}
}

func TestPostfixIsNotAnOpenRelay(t *testing.T) {
	settings := strings.Join(postfixSettings("10.8.0.2", []string{"b.example", "a.example"}), "\n")
	for _, want := range []string{
		"relayhost=[10.8.0.2]:25",
		"relay_domains=a.example, b.example",
		"reject_unauth_destination",
		"smtp_tls_security_level=may",
	} {
		if !strings.Contains(settings, want) {
			t.Fatalf("missing %s in %s", want, settings)
		}
	}
	if strings.Contains(settings, "0.0.0.0/0") {
		t.Fatal("relay accepts the whole internet")
	}
}

func TestParseQueueFormats(t *testing.T) {
	jsonQueue := `{"queue_name":"deferred","queue_id":"ABC123","arrival_time":1760380000,"message_size":1200,"sender":"from@example.org","recipients":[{"address":"to@example.org","delay_reason":"connect refused"}]}`
	messages, err := parsePostqueueJSON(jsonQueue + "\n")
	if err != nil || len(messages) != 1 || messages[0].ID != "ABC123" || messages[0].Recipients[0] != "to@example.org" || messages[0].Reason != "connect refused" {
		t.Fatalf("json: %+v %v", messages, err)
	}
	text := "" +
		"-Queue ID-  --Size-- ----Arrival Time---- -Sender/Recipient-------\n" +
		"DEF456*     80 Tue Oct 14 18:04:11  from@example.org\n" +
		"                                         to@example.org\n" +
		"\n" +
		"-- 80 bytes in 1 Request.\n"
	parsed := parseMailq(text)
	if len(parsed) != 1 || parsed[0].ID != "DEF456" || parsed[0].Status != "active" || parsed[0].Recipients[0] != "to@example.org" {
		t.Fatalf("mailq: %+v", parsed)
	}
	if len(parseMailq("Mail queue is empty\n")) != 0 {
		t.Fatal("empty queue")
	}
}

func TestEvaluateDNS(t *testing.T) {
	const public = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A"
	look := fakeResolver{
		txt: map[string][]string{
			"example.org":                  {"v=spf1 mx a ip4:203.0.113.10 -all"},
			"godeb._domainkey.example.org": {"v=DKIM1; h=sha256; k=rsa; p=OTHER"},
			"_dmarc.example.org":           {"v=DMARC1; p=quarantine; rua=mailto:dmarc@example.org"},
		},
		ptr: []string{"mail.example.org."},
	}
	checks := evaluateDNS(context.Background(), "example.org", "godeb", "203.0.113.10", public, look)
	got := map[string]string{}
	for _, check := range checks {
		got[check.ID] = check.Status
		if check.Record == "" {
			t.Fatalf("missing copy record for %s", check.ID)
		}
	}
	if got["ptr"] != "ok" || got["spf"] != "ok" || got["dkim"] != "warn" || got["dmarc"] != "ok" {
		t.Fatalf("lights: %+v", got)
	}
}

func TestOpenDKIMFilesStayPrivate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GODEB_MAIL_ROOT", root)
	privatePEM, publicB64, err := generateDKIM()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(privatePEM, "BEGIN RSA PRIVATE KEY") || len(publicB64) < 200 {
		t.Fatal("unexpected key material")
	}
	err = writeOpenDKIM([]dkimFiles{{
		domain: "example.org", selector: "godeb", privatePEM: privatePEM, publicB64: publicB64,
	}})
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(root, "etc", "opendkim", "keys", "example.org", "godeb.private")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	table, err := os.ReadFile(filepath.Join(root, "etc", "opendkim", "key.table"))
	if err != nil || !strings.Contains(string(table), keyPath) {
		t.Fatalf("key table: %v %s", err, table)
	}
}

func TestQueueHistoryRemembersOnce(t *testing.T) {
	t.Setenv("GODEB_DB", filepath.Join(t.TempDir(), "godeb.db"))
	conn, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	message := []queueMessage{{ID: "ABC", Sender: "a@b", Recipients: []string{"c@d"}, Reason: "deferred"}}
	if err := rememberQueue(conn, message); err != nil {
		t.Fatal(err)
	}
	if err := rememberQueue(conn, message); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM mail_queue_events WHERE queue_id = 'ABC'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count %d", count)
	}
}

func TestSetupRejectsPublicRelay(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/plugins/mailgateway/setup", strings.NewReader(`{"domain":"example.org","relayHost":"8.8.8.8"}`))
	rec := httptest.NewRecorder()
	handleSetup(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

type fakeResolver struct {
	txt map[string][]string
	ptr []string
}

func (f fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	return f.txt[name], nil
}

func (f fakeResolver) LookupAddr(context.Context, string) ([]string, error) {
	return f.ptr, nil
}
