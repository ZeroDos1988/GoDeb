package mailgateway

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// postfixSettings are the main.cf keys for a closed store-and-forward relay.
// relay_domains lists only the configured domains, and the next hop stays
// inside the WireGuard transfer network.
func postfixSettings(relayHost string, domains []string) []string {
	sorted := append([]string(nil), domains...)
	sort.Strings(sorted)
	return []string{
		"relayhost=[" + relayHost + "]:25",
		"relay_domains=" + strings.Join(sorted, ", "),
		"mydestination=localhost.localdomain, localhost",
		"inet_interfaces=all",
		"mynetworks=127.0.0.0/8 [::ffff:127.0.0.0]/104 [::1]/128",
		"smtpd_relay_restrictions=permit_mynetworks, reject_unauth_destination",
		"smtpd_recipient_restrictions=permit_mynetworks, reject_unauth_destination",
		"maximal_queue_lifetime=5d",
		"bounce_queue_lifetime=5d",
		"delay_warning_time=4h",
		"smtp_tls_security_level=may",
		"smtpd_tls_security_level=may",
		"smtp_tls_CApath=/etc/ssl/certs",
		"milter_protocol=6",
		"milter_default_action=accept",
		"smtpd_milters=inet:127.0.0.1:8891",
		"non_smtpd_milters=inet:127.0.0.1:8891",
	}
}

func generateDKIM() (privatePEM, publicB64 string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", err
	}
	return string(block), base64.StdEncoding.EncodeToString(der), nil
}

func dkimTXT(selector, domain, publicB64 string) string {
	return fmt.Sprintf("%s._domainkey.%s TXT \"v=DKIM1; h=sha256; k=rsa; p=%s\"", selector, domain, publicB64)
}

func etcPath(elems ...string) string {
	full := filepath.Join(append([]string{"/etc"}, elems...)...)
	if root := strings.TrimSpace(os.Getenv("GODEB_MAIL_ROOT")); root != "" {
		return filepath.Join(root, strings.TrimPrefix(full, string(os.PathSeparator)))
	}
	return full
}

type dkimFiles struct {
	domain     string
	selector   string
	privatePEM string
	publicB64  string
}

func writeOpenDKIM(domains []dkimFiles) error {
	confPath := etcPath("opendkim.conf")
	keyTable := etcPath("opendkim", "key.table")
	signing := etcPath("opendkim", "signing.table")
	trusted := etcPath("opendkim", "trusted.hosts")
	if err := os.MkdirAll(filepath.Dir(keyTable), 0o750); err != nil {
		return err
	}
	var keys, signs strings.Builder
	for _, domain := range domains {
		dir := etcPath("opendkim", "keys", domain.domain)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
		keyPath := filepath.Join(dir, domain.selector+".private")
		if err := os.WriteFile(keyPath, []byte(domain.privatePEM), 0o600); err != nil {
			return err
		}
		txt := dkimTXT(domain.selector, domain.domain, domain.publicB64) + "\n"
		if err := os.WriteFile(filepath.Join(dir, domain.selector+".txt"), []byte(txt), 0o644); err != nil {
			return err
		}
		identity := domain.selector + "._domainkey." + domain.domain
		fmt.Fprintf(&keys, "%s %s:%s:%s\n", identity, domain.domain, domain.selector, keyPath)
		fmt.Fprintf(&signs, "*@%s %s\n", domain.domain, identity)
	}
	conf := strings.Join([]string{
		"Syslog yes",
		"UMask 007",
		"Mode sv",
		"Canonicalization relaxed/simple",
		"OversignHeaders From",
		"KeyTable refile:" + keyTable,
		"SigningTable refile:" + signing,
		"ExternalIgnoreList refile:" + trusted,
		"InternalHosts refile:" + trusted,
		"Socket inet:8891@127.0.0.1",
		"UserID opendkim:opendkim",
		"",
	}, "\n")
	hosts := "127.0.0.1\nlocalhost\n10.8.0.0/24\n"
	for _, file := range []struct {
		path string
		body string
		mode os.FileMode
	}{
		{confPath, conf, 0o644},
		{keyTable, keys.String(), 0o644},
		{signing, signs.String(), 0o644},
		{trusted, hosts, 0o644},
	} {
		if err := os.WriteFile(file.path, []byte(file.body), file.mode); err != nil {
			return err
		}
	}
	return nil
}
