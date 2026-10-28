package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMTASTSPolicyAndRecords(t *testing.T) {
	s := Settings{Hostname: "mail.example.com"}
	p := mtaSTSPolicy(s)
	for _, want := range []string{"version: STSv1", "mode: testing", "mx: mail.example.com", "max_age: 86400"} {
		if !strings.Contains(p, want) {
			t.Fatalf("policy lacks %q:\n%s", want, p)
		}
	}
	id := mtaSTSID(s)
	if len(id) != 16 {
		t.Fatalf("id length %d", len(id))
	}
	rec := mtaSTSRecord("example.com", s)
	if !strings.HasPrefix(rec, "v=STSv1; id=") || !strings.Contains(rec, id) {
		t.Fatalf("%q", rec)
	}
	if tlsRPTRecord("example.com") != "v=TLSRPTv1; rua=mailto:tls-reports@example.com" {
		t.Fatal(tlsRPTRecord("example.com"))
	}
	recs := expectedRecords(s, Domain{Name: "example.com", Selector: "godeb", DKIMPublic: "K"}, "203.0.113.7")
	kinds := map[string]bool{}
	for _, r := range recs {
		kinds[r.Kind] = true
	}
	for _, k := range []string{"mtasts", "tlsrpt", "mtasts-host", "dmarc", "mx"} {
		if !kinds[k] {
			t.Errorf("missing kind %s", k)
		}
	}
}

func TestSyncMTASTS(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_MAIL_ROOT", dir)
	s := Settings{Hostname: "mail.example.com"}
	domains := []Domain{{Name: "example.com"}, {Name: "old.example", Suspended: true}}
	if err := syncMTASTS(s, domains); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "etc/godeb/mta-sts/example.com.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "mx: mail.example.com") {
		t.Fatalf("%s", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "etc/godeb/mta-sts/old.example.txt")); !os.IsNotExist(err) {
		t.Fatalf("suspended domain should have no policy: %v", err)
	}
	// Stale file is removed on next sync.
	stale := filepath.Join(dir, "etc/godeb/mta-sts/gone.example.txt")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncMTASTS(s, domains); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale policy kept")
	}
}
