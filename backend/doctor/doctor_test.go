package doctor

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"godeb/backend/core"
)

const ssOut = `udp   UNCONN 0      0            0.0.0.0:51820      0.0.0.0:*
udp   UNCONN 0      0      127.0.0.53%lo:53         0.0.0.0:*    users:(("systemd-resolve",pid=300,fd=14))
tcp   LISTEN 0      4096       127.0.0.1:8080       0.0.0.0:*    users:(("godeb",pid=123,fd=7))
tcp   LISTEN 0      4096        10.8.0.1:8080       0.0.0.0:*    users:(("godeb",pid=123,fd=8))
tcp   LISTEN 0      128          0.0.0.0:22         0.0.0.0:*    users:(("sshd",pid=1,fd=3))
tcp   LISTEN 0      128             [::]:22            [::]:*    users:(("sshd",pid=1,fd=4))
tcp   LISTEN 0      128            [::1]:25            [::]:*    users:(("master",pid=9,fd=13))
tcp   LISTEN 0      100     203.0.113.5:11334       0.0.0.0:*    users:(("rspamd",pid=7,fd=9))
`

func iface(ip net.IP) string {
	if ip.Equal(net.ParseIP("10.8.0.1")) {
		return "wg0"
	}
	return "eth0"
}

func TestParseSS(t *testing.T) {
	got := parseSS(ssOut, iface)
	var desc []string
	for _, l := range got {
		desc = append(desc, l.Proto+" "+l.Scope+" "+l.Process)
	}
	want := []string{
		"tcp alle Adressen sshd",
		"tcp Tunnel 10.8.0.1 godeb",
		"tcp 203.0.113.5 rspamd",
		"udp alle Adressen ?",
	}
	if strings.Join(desc, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", desc)
	}
}

func TestParseHandshakesAndPolicy(t *testing.T) {
	peers := parseHandshakes("wg0\tAAAAbbbbCCCCdddd=\t1700000000\nwg0\tZZZZyyyy=\t0\n")
	if len(peers) != 2 || peers[0].Key != "AAAAbbbb" || peers[0].Last.Unix() != 1700000000 || !peers[1].Last.IsZero() {
		t.Fatalf("peers = %+v", peers)
	}
	inst, cand := parsePolicy("godeb:\n  Installed: 0.2.7\n  Candidate: 0.2.8\n  Version table:\n")
	if inst != "0.2.7" || cand != "0.2.8" {
		t.Fatalf("policy = %q %q", inst, cand)
	}
	all, sec := ParseUpgradable("Listing...\nopenssl/stable-security 3.5.1-1+deb13u1 amd64 [upgradable from: 3.5.1-1]\ncurl/stable 8.14 amd64 [upgradable from: 8.13]\n")
	if len(all) != 2 || len(sec) != 1 || sec[0] != "openssl" {
		t.Fatalf("upgradable = %v %v", all, sec)
	}
}

func selfSigned(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "mail.example.org"}, NotBefore: notAfter.Add(-90 * 24 * time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestCheckReport(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	certs := map[string][]byte{
		"/etc/letsencrypt/live/mail.example.org/cert.pem": selfSigned(t, now.Add(60*24*time.Hour)),
		"/etc/letsencrypt/live/old.example.org/cert.pem":  selfSigned(t, now.Add(-24*time.Hour)),
	}
	outputs := map[string]core.CommandResult{
		"apt-cache":  {Stdout: "  Installed: 0.2.7\n  Candidate: 0.2.7\n"},
		"systemctl":  {Stdout: "active\nactive\n"},
		"ss":         {Stdout: ssOut},
		"ufw":        {Stdout: "Status: active\n"},
		"wg":         {Stdout: "wg0\tAAAAbbbbCCCC=\t" + itoa(now.Add(-time.Minute).Unix()) + "\n"},
		"journalctl": {},
		"apt":        {Stdout: "Listing...\n"},
	}
	env := Env{
		Run: func(_ context.Context, name string, args ...string) core.CommandResult {
			if name == "systemctl" && args[0] == "--failed" {
				return core.CommandResult{}
			}
			res, ok := outputs[name]
			if !ok {
				return core.CommandResult{Err: exec.ErrNotFound}
			}
			return res
		},
		ReadFile: func(p string) ([]byte, error) {
			if b, ok := certs[p]; ok {
				return b, nil
			}
			return nil, errors.New("nope")
		},
		Glob: func(string) ([]string, error) {
			return []string{"/etc/letsencrypt/live/mail.example.org/cert.pem", "/etc/letsencrypt/live/old.example.org/cert.pem"}, nil
		},
		Exists:  func(string) bool { return true },
		IfaceOf: iface,
		Disk:    func(string) (uint64, uint64, error) { return 85, 100, nil },
		DNS: func(context.Context) ([]DNSRecord, error) {
			return []DNSRecord{
				{Name: "example.org", Type: "MX", Status: "ok"},
				{Name: "203.0.113.5", Type: "PTR", Status: "missing", Expected: "5.113.0.203.in-addr.arpa. IN PTR mail.example.org."},
			}, nil
		},
		Now:     func() time.Time { return now },
		Version: "0.2.7",
	}
	var buf bytes.Buffer
	fails, warns := Write(&buf, Check(context.Background(), env))
	out := buf.String()
	for _, want := range []string{
		"OK    godeb läuft",
		"OK    ufw aktiv",
		"OK    1 von 1 Peers aktiv",
		"WARN  Rspamd-Weboberfläche von außen erreichbar?",
		"FAIL  old.example.org abgelaufen",
		"OK    mail.example.org gültig bis 2026-12-26",
		"WARN  PTR 203.0.113.5: fehlt",
		"WARN  Neustart nötig",
		"WARN  / zu 85 % belegt",
		"OK    keine Fehler",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if fails != 1 || warns != 4 {
		t.Errorf("fails=%d warns=%d\n%s", fails, warns, out)
	}
	if strings.Contains(out, "AAAAbbbbCCCC=") {
		t.Error("full public key in report")
	}
}

func itoa(n int64) string { return big.NewInt(n).String() }

func TestParseSSHDAndHardening(t *testing.T) {
	pass, root := parseSSHD("passwordauthentication no\npermitrootlogin prohibit-password\n")
	if pass != "no" || root != "prohibit-password" {
		t.Fatalf("%q %q", pass, root)
	}
	got := sshFindings(core.CommandResult{Stdout: "passwordauthentication yes\npermitrootlogin yes\n"})
	if len(got) != 2 || got[0].Level != Warn || got[1].Level != Warn {
		t.Fatalf("%+v", got)
	}
	got = unattendedFindings(core.CommandResult{Stdout: "disabled\n"})
	if len(got) != 1 || got[0].Level != Info || !strings.Contains(got[0].Text, "disabled") {
		t.Fatalf("%+v", got)
	}
}

func TestNewReport(t *testing.T) {
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	r := NewReport([]Finding{
		{Area: "A", Level: OK, Text: "gut"},
		{Area: "B", Level: Warn, Text: "vorsicht", Details: []string{"x"}},
		{Area: "C", Level: Fail, Text: "kaputt"},
	}, at)
	if r.Fails != 1 || r.Warns != 1 || r.Checked != at.Unix() || len(r.Findings) != 3 {
		t.Fatalf("%+v", r)
	}
	if r.Findings[1].Level != "WARN" || r.Findings[1].Details[0] != "x" {
		t.Fatalf("finding = %+v", r.Findings[1])
	}
}

func TestCheckDatabases(t *testing.T) {
	env := Env{
		Databases: []DBFile{{"Agent", "/etc/godeb/godeb.db"}, {"Messwerte", "/x/metrics.db"}, {"Fehlt", "/none"}},
		CheckDB: func(_ context.Context, path string) (bool, error) {
			switch path {
			case "/x/metrics.db":
				return true, errors.New("beschädigt: page 3")
			case "/none":
				return false, nil
			}
			return true, nil
		},
	}
	got := CheckDatabases(context.Background(), env)
	if len(got) != 2 || got[0].Level != OK || got[1].Level != Fail || !strings.Contains(got[1].Text, "page 3") {
		t.Fatalf("got %+v", got)
	}
	if CheckDatabases(context.Background(), Env{}) != nil {
		t.Fatal("without CheckDB there must be no finding")
	}
}
