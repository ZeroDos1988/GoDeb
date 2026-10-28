package postgres

import (
	"strings"
	"testing"
)

func TestCleanIdent(t *testing.T) {
	if _, err := cleanIdent("My_App", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanIdent("postgres", "x"); err == nil {
		t.Fatal("reserved")
	}
	if _, err := cleanIdent("1bad", "x"); err == nil {
		t.Fatal("digit")
	}
	if err := cleanPassword("short"); err == nil {
		t.Fatal("short pw")
	}
	if err := cleanPassword("long-enough-secret"); err != nil {
		t.Fatal(err)
	}
}

func TestCleanPasswordASCII(t *testing.T) {
	for _, bad := range []string{" leading-space-pw", "trailing-space-pw ", "umlaut-pässwort", "tab\tin-password"} {
		if err := cleanPassword(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// Vector computed independently with Python (hashlib.pbkdf2_hmac, hmac,
// sha256) for password "long-enough-secret" and salt 0x00..0x0f.
func TestScramVerifier(t *testing.T) {
	salt := make([]byte, 16)
	for i := range salt {
		salt[i] = byte(i)
	}
	got, err := scramVerifierSalt("long-enough-secret", salt)
	if err != nil {
		t.Fatal(err)
	}
	if got != wantScram {
		t.Fatalf("got  %s\nwant %s", got, wantScram)
	}
	if strings.Contains(got, "long-enough-secret") {
		t.Fatal("clear password in verifier")
	}
}

const wantScram = "SCRAM-SHA-256$4096:AAECAwQFBgcICQoLDA0ODw==$kQMfaj0tnzSF3JLohG1O/geohIfj4Z/Fg2fuV5saC5Q=:0aq+SMrs7bpmOn1gGaDQ1U95luG0+vapEIapJltla/A="
