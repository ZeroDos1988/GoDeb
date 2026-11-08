package mail

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"godeb/backend/core"
)

func TestNormalizeHub(t *testing.T) {
	if h, err := NormalizeHub(" 10.8.0.1 "); err != nil || h != "10.8.0.1" {
		t.Fatalf("got %q %v", h, err)
	}
	for _, bad := range []string{"", "8.8.8.8", "fd00::1", "10.8.0.1]:25\nrelayhost=x", "mail.example.com"} {
		if _, err := NormalizeHub(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestBehindHubRender(t *testing.T) {
	s, domains, _ := exampleSetup(t)
	direct := strings.Join(postfixMain(s), "\n")
	if !strings.Contains(direct, "relayhost=\n") {
		t.Fatal("direct server must send itself")
	}
	s.Hub = "10.8.0.1"
	main := strings.Join(postfixMain(s), "\n")
	if !strings.Contains(main, "relayhost=[10.8.0.1]:25\n") {
		t.Fatalf("no relay through the hub:\n%s", main)
	}
	files := rspamdFiles(s, domains)
	for _, f := range []string{"/dkim_signing.conf", "/arc.conf"} {
		if body := files[rspamdLocal+f]; !strings.Contains(body, "enabled = false;") || strings.Contains(body, "selector") {
			t.Fatalf("%s still signs behind the hub:\n%s", f, body)
		}
	}
	if !strings.Contains(files[rspamdLocal+"/external_relay.conf"], `ip_map = ["10.8.0.1"];`) {
		t.Fatalf("external_relay:\n%s", files[rspamdLocal+"/external_relay.conf"])
	}
	for p := range files {
		if _, ok := allowed[p]; !ok {
			t.Errorf("%s not in the allowlist", p)
		}
	}
}

func TestPortRulesBehindHub(t *testing.T) {
	for _, r := range portRules(true) {
		if strings.Join(r[:4], " ") != "allow in on wg0" {
			t.Fatalf("rule open beyond the tunnel: %v", r)
		}
	}
	if got := strings.Join(portRules(false)[0], " "); got != "allow 25/tcp comment GoDeb-Mail" {
		t.Fatalf("direct rule %q", got)
	}
}

func TestInstallHubCertificate(t *testing.T) {
	useTestRoot(t)
	s := Settings{Hostname: "postfach.example.com", CertFile: snakeoilCert, KeyFile: snakeoilKey, Hub: "10.8.0.1"}
	if err := saveSettings(s); err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM := selfSigned(t, "postfach.example.com")
	wrongCert, wrongKey := selfSigned(t, "other.example.com")
	if err := InstallHubCertificate(t.Context(), wrongCert, wrongKey); err == nil {
		t.Fatal("certificate for another name accepted")
	}
	if err := InstallHubCertificate(t.Context(), certPEM, wrongKey); err == nil {
		t.Fatal("mismatched key accepted")
	}
	// apply runs postfix/dovecot commands that do not exist here; the files
	// and settings must be in place before that.
	_ = InstallHubCertificate(t.Context(), certPEM, keyPEM)
	got, _, err := loadSettings()
	if err != nil || got.CertFile != hubCertFile || got.KeyFile != hubKeyFile {
		t.Fatalf("settings %+v %v", got, err)
	}
	if allowed[hubKeyFile].mode != 0o600 {
		t.Fatal("key must be 0600")
	}
}

func selfSigned(t *testing.T, host string) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kd, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd})
}

// useTestRoot points files and the agent database to a temporary directory
// and records system commands instead of running them.
func useTestRoot(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GODEB_MAIL_ROOT", dir)
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_AGENT_MASTER_KEY", filepath.Join(dir, "agent.key"))
	admin = func(context.Context, string, ...string) core.CommandResult { return core.CommandResult{} }
	adminTimeout = func(context.Context, time.Duration, string, ...string) core.CommandResult {
		return core.CommandResult{}
	}
	t.Cleanup(func() { admin, adminTimeout = core.RunAdmin, core.RunAdminTimeout })
}
