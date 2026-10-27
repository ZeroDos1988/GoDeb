package mail

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
)

// dkimBits: 2048-bit RSA fits into one DNS TXT record split in strings and
// is what every receiver checks.
const dkimBits = 2048

// newDKIMKey returns a PEM private key (PKCS#8) for Rspamd and the base64
// public key for the DNS record.
func newDKIMKey() (privatePEM, publicB64 string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, dkimBits)
	if err != nil {
		return "", "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), base64.StdEncoding.EncodeToString(pub), nil
}

// dkimRecordValue is the TXT value of <selector>._domainkey.<domain>.
func dkimRecordValue(publicB64 string) string {
	return "v=DKIM1; k=rsa; p=" + publicB64
}

// splitTXT cuts a long TXT value into 255-byte strings as zone files need
// them ("…" "…"); most DNS web interfaces take the joined value too.
func splitTXT(v string) string {
	var parts []string
	for len(v) > 255 {
		parts = append(parts, fmt.Sprintf("%q", v[:255]))
		v = v[255:]
	}
	parts = append(parts, fmt.Sprintf("%q", v))
	return strings.Join(parts, " ")
}
