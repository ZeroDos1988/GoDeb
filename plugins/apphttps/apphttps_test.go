package apphttps

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"godeb/backend/core"
	"godeb/backend/metrics"
	"godeb/plugins/domains"
)

func setup(t *testing.T) *[][]string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GODEB_DB", filepath.Join(dir, "godeb.db"))
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_MASTER_KEY", filepath.Join(dir, "master.key"))
	t.Setenv("GODEB_STATE_DIR", filepath.Join(dir, "state"))
	oldIP, oldUFW, oldPub, oldObtain := tunnelIPv4, runUFW, publishA, obtainCert
	t.Cleanup(func() { tunnelIPv4, runUFW, publishA, obtainCert = oldIP, oldUFW, oldPub, oldObtain })
	tunnelIPv4 = func() (string, error) { return "10.8.0.1", nil }
	var ufw [][]string
	runUFW = func(_ context.Context, args ...string) core.CommandResult {
		ufw = append(ufw, args)
		return core.CommandResult{}
	}
	return &ufw
}

func selfSigned(t *testing.T, host string, notAfter time.Time) (string, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter,
	}, &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}}, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func TestHostFor(t *testing.T) {
	zones := []string{"example.com"}
	if h, err := hostFor("GoDeb", "example.com.", zones); err != nil || h != "godeb.example.com" {
		t.Fatalf("host = %q %v", h, err)
	}
	for _, c := range [][2]string{{"a.b", "example.com"}, {"-x", "example.com"}, {"x", "other.com"}, {"", "example.com"}, {"x\n", "example.com\nfoo"}} {
		if _, err := hostFor(c[0], c[1], zones); err == nil {
			t.Fatalf("%q/%q accepted", c[0], c[1])
		}
	}
}

func FuzzHostFor(f *testing.F) {
	for _, s := range []string{"godeb", "a-b", "x.y", "äö", "a\nb", `a"b`, "*", ""} {
		f.Add(s, "example.com")
	}
	f.Fuzz(func(t *testing.T, label, zone string) {
		host, err := hostFor(label, zone, []string{"example.com"})
		if err != nil {
			return
		}
		rel, ok := strings.CutSuffix(host, ".example.com")
		if !ok || !labelPattern.MatchString(rel) {
			t.Fatalf("unsafe host %q", host)
		}
	})
}

func TestIssueWritesFilesAndOpensPort(t *testing.T) {
	ufw := setup(t)
	var published []domains.Need
	publishA = func(_ context.Context, n domains.Need) error {
		published = append(published, n)
		return nil
	}
	certPEM, keyPEM := selfSigned(t, "godeb.example.com", time.Now().Add(90*24*time.Hour))
	obtainCert = func(_ context.Context, host, account string) (issued, error) {
		if host != "godeb.example.com" || !strings.Contains(account, "PRIVATE KEY") {
			t.Fatalf("obtain(%q, %q)", host, account)
		}
		return issued{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: time.Now().Add(90 * 24 * time.Hour)}, nil
	}
	if _, err := update(func(r *settingsRow) {
		r.Label, r.Zone, r.Host, r.Enabled = "godeb", "example.com", "godeb.example.com", true
	}); err != nil {
		t.Fatal(err)
	}
	if err := issue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 || published[0].Name != "godeb.example.com" || published[0].Value != "10.8.0.1" || published[0].Type != "A" {
		t.Fatalf("published = %+v", published)
	}
	l := LoadListen()
	if !l.Enabled || l.Host != "godeb.example.com" || l.Port != Port {
		t.Fatalf("listen = %+v", l)
	}
	if _, err := tls.LoadX509KeyPair(CertPath(), KeyPath()); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(KeyPath()); info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %v", info.Mode())
	}
	last := (*ufw)[len(*ufw)-1]
	if strings.Join(last, " ") != "allow in on wg0 to any port 8443 proto tcp comment GoDeb HTTPS im Tunnel" {
		t.Fatalf("ufw = %v", last)
	}
	row, _ := load()
	if needsRenewal(row, time.Now()) || !needsRenewal(row, time.Now().Add(70*24*time.Hour)) {
		t.Fatal("renewal window wrong")
	}

	// Disable: listen file off, port closed, certificate kept.
	mux := http.NewServeMux()
	Mount(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, base+"/disable", nil))
	if rec.Code != 200 {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body)
	}
	if LoadListen().Enabled {
		t.Fatal("still enabled")
	}
	if got := strings.Join((*ufw)[len(*ufw)-1], " "); got != "delete allow in on wg0 to any port 8443 proto tcp" {
		t.Fatalf("ufw = %s", got)
	}
}

func TestEnableValidates(t *testing.T) {
	setup(t)
	mux := http.NewServeMux()
	Mount(mux)
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, base+"/enable", strings.NewReader(body)))
		return rec
	}
	// No Hetzner token yet.
	if rec := post(`{"label":"godeb","zone":"example.com"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("without token: %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+"/status", nil))
	var s statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil || s.Enabled || s.Port != Port || s.Address != "10.8.0.1" {
		t.Fatalf("status = %+v %v", s, err)
	}
	if rec := httptest.NewRecorder(); true {
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, base+"/renew", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("renew while off: %d", rec.Code)
		}
	}
}

func TestExpiryFinding(t *testing.T) {
	now := time.Now()
	row := settingsRow{Enabled: true, CertPEM: "x", NotAfter: now.Add(40 * 24 * time.Hour).Format(time.RFC3339)}
	if f := expiryFinding(row, now); f != nil {
		t.Fatalf("40 days: %+v", f)
	}
	row.NotAfter = now.Add(10 * 24 * time.Hour).Format(time.RFC3339)
	if f := expiryFinding(row, now); len(f) != 1 || f[0].Status != metrics.StatusWarning {
		t.Fatalf("10 days: %+v", f)
	}
	row.NotAfter = now.Add(time.Hour).Format(time.RFC3339)
	if f := expiryFinding(row, now); len(f) != 1 || f[0].Status != metrics.StatusCritical {
		t.Fatalf("1 hour: %+v", f)
	}
	if f := expiryFinding(settingsRow{Enabled: true, LastError: "boom"}, now); len(f) != 1 {
		t.Fatalf("no cert: %+v", f)
	}
}

func TestLoadListenRejectsBadPort(t *testing.T) {
	t.Setenv("GODEB_STATE_DIR", t.TempDir())
	if err := writeFiles(Listen{Enabled: true, Host: "x.example.com", Port: 80}, []byte("c"), []byte("k")); err != nil {
		t.Fatal(err)
	}
	if LoadListen().Enabled {
		t.Fatal("port 80 accepted")
	}
}
