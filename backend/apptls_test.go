package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"godeb/plugins/apphttps"
)

func writePair(t *testing.T, cn string, mtime time.Time) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	if err := os.MkdirAll(filepath.Dir(apphttps.CertPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(apphttps.KeyPath(), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(apphttps.CertPath(), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(apphttps.CertPath(), mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestCertFilesReloadOnChange(t *testing.T) {
	t.Setenv("GODEB_STATE_DIR", t.TempDir())
	var c certFiles
	if _, err := c.get(nil); err == nil {
		t.Fatal("no files must be an error")
	}
	now := time.Now()
	writePair(t, "first", now.Add(-time.Minute))
	cert, err := c.get(nil)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	if leaf.Subject.CommonName != "first" {
		t.Fatalf("cn = %s", leaf.Subject.CommonName)
	}
	writePair(t, "second", now)
	cert, _ = c.get(nil)
	leaf, _ = x509.ParseCertificate(cert.Certificate[0])
	if leaf.Subject.CommonName != "second" {
		t.Fatalf("not reloaded: %s", leaf.Subject.CommonName)
	}
	// A broken file keeps the last good certificate.
	if err := os.WriteFile(apphttps.CertPath(), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cert, err := c.get(nil); err != nil || cert == nil {
		t.Fatalf("lost certificate: %v", err)
	}
}
