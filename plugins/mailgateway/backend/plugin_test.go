package mailgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"godeb/backend/core"
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
		"relayhost=\n",
		"relay_transport=smtp:[10.8.0.2]:25",
		"relay_domains=a.example, b.example",
		// Only the mail server's own tunnel address may send out.
		"mynetworks=127.0.0.0/8 [::ffff:127.0.0.0]/104 [::1]/128 10.8.0.2/32\n",
		"reject_unauth_destination",
		"smtp_tls_security_level=may",
	} {
		if !strings.Contains(settings, want) {
			t.Fatalf("missing %s in %s", want, settings)
		}
	}
	if strings.Contains(settings, "0.0.0.0/0") || strings.Contains(settings, "10.8.0.0/24 ") {
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

func TestSetRelayKeepsDKIM(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_DB", filepath.Join(dir, "godeb.db"))
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_MAIL_ROOT", dir)
	var calls []string
	old := runCommand
	runCommand = func(_ context.Context, name string, args ...string) core.CommandResult {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return core.CommandResult{}
	}
	t.Cleanup(func() { runCommand = old })
	if err := SetRelay(t.Context(), "10.8.0.3"); err == nil {
		t.Fatal("relay without domains accepted")
	}
	conn, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := saveDomain(conn, storedDomain{Domain: "example.com", Selector: "s2024", DKIMPublic: "PUB"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"8.8.8.8", "10.9.0.2", "mail.example.com"} {
		if err := SetRelay(t.Context(), bad); err == nil {
			t.Errorf("relay %q accepted", bad)
		}
	}
	if err := SetRelay(t.Context(), "10.8.0.3"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "relay_transport=smtp:[10.8.0.3]:25") || !strings.Contains(joined, "10.8.0.3/32") {
		t.Fatalf("postconf calls:\n%s", joined)
	}
	d, _, err := findDomain(conn, "example.com")
	if err != nil || d.Selector != "s2024" || d.DKIMPublic != "PUB" {
		t.Fatalf("DKIM changed: %+v %v", d, err)
	}
	if relay, _ := loadRelay(conn); relay != "10.8.0.3" {
		t.Fatalf("relay %q", relay)
	}
}
