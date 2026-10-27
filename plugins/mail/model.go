// Package mail is a full mail server for GoDeb, modelled on the mail part
// of HestiaCP: mail domains, mailboxes with quota, aliases, forwarding,
// catch-all, DKIM/SPF/DMARC and spam filtering.
//
// Stack (Debian 13): Postfix (SMTP, submission), Dovecot 2.4 (IMAP, LMTP,
// Sieve, quota), Rspamd (spam, DKIM signing, SPF/DKIM/DMARC checks) with
// Redis, optional ClamAV. GoDeb keeps the metadata in the agent database
// (/etc/godeb, root) and writes every Postfix map and Dovecot file from it;
// the files are not meant to be edited by hand.
package mail

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
)

// Domain is a mail domain.
type Domain struct {
	Name       string `json:"name"`
	Selector   string `json:"selector"`
	DKIMPublic string `json:"dkimPublic"`
	CatchAll   string `json:"catchAll"` // address that gets mail for unknown local parts, "" = reject
	Suspended  bool   `json:"suspended"`
	Created    string `json:"created"`
}

// Account is a mailbox (or a forward-only address).
type Account struct {
	Domain      string   `json:"domain"`
	Local       string   `json:"local"`
	Hash        string   `json:"-"`
	QuotaMB     int      `json:"quotaMB"` // 0 = unlimited
	Aliases     []string `json:"aliases"` // further local parts of the same domain
	Forward     []string `json:"forward"` // full addresses
	ForwardOnly bool     `json:"forwardOnly"`
	Suspended   bool     `json:"suspended"`
	Created     string   `json:"created"`
}

// Address is local@domain.
func (a Account) Address() string { return a.Local + "@" + a.Domain }

// Settings is the server-wide part.
type Settings struct {
	Hostname string `json:"hostname"` // FQDN of the MX, e.g. mail.example.com
	ClamAV   bool   `json:"clamav"`
	// CertFile/KeyFile: Let's Encrypt once issued, the snakeoil pair before.
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`
}

const (
	snakeoilCert = "/etc/ssl/certs/ssl-cert-snakeoil.pem"
	snakeoilKey  = "/etc/ssl/private/ssl-cert-snakeoil.key"
	maxQuotaMB   = 1 << 20 // 1 TB
	maxAliases   = 50
	maxForward   = 20
)

var (
	domainPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	localPattern    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._+-]{0,62}[a-z0-9])?$`)
	selectorPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
)

// NormalizeDomain lowercases and checks a domain name.
func NormalizeDomain(v string) (string, error) {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
	if len(d) > 253 || !domainPattern.MatchString(d) {
		return "", errors.New("Domain ist ungültig")
	}
	return d, nil
}

// NormalizeLocal checks the part before @. Only a conservative set: these
// names end up in Postfix maps and Dovecot's passwd-file.
func NormalizeLocal(v string) (string, error) {
	l := strings.ToLower(strings.TrimSpace(v))
	if !localPattern.MatchString(l) || strings.Contains(l, "..") {
		return "", errors.New("Name vor dem @: Buchstaben, Ziffern und . _ + - (nicht am Anfang oder Ende)")
	}
	return l, nil
}

// NormalizeAddress checks a full address (forward targets, catch-all).
func NormalizeAddress(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	parsed, err := mail.ParseAddress(v)
	if err != nil || parsed.Address != v || parsed.Name != "" {
		return "", fmt.Errorf("Adresse %q ist ungültig", v)
	}
	local, domain, ok := strings.Cut(v, "@")
	if !ok {
		return "", fmt.Errorf("Adresse %q ist ungültig", v)
	}
	if _, err := NormalizeLocal(local); err != nil {
		return "", fmt.Errorf("Adresse %q ist ungültig", v)
	}
	if _, err := NormalizeDomain(domain); err != nil {
		return "", fmt.Errorf("Adresse %q ist ungültig", v)
	}
	return v, nil
}

// NormalizeHostname checks the mail server's FQDN (needs at least three
// labels in practice, but two are allowed for small setups).
func NormalizeHostname(v string) (string, error) {
	h, err := NormalizeDomain(v)
	if err != nil {
		return "", errors.New("Hostname des Mailservers ist ungültig (z. B. mail.example.com)")
	}
	return h, nil
}

// cleanAccount validates and normalises an account before storing it.
func cleanAccount(a Account) (Account, error) {
	var err error
	if a.Domain, err = NormalizeDomain(a.Domain); err != nil {
		return a, err
	}
	if a.Local, err = NormalizeLocal(a.Local); err != nil {
		return a, err
	}
	if a.QuotaMB < 0 || a.QuotaMB > maxQuotaMB {
		return a, errors.New("Quota ist ungültig")
	}
	if len(a.Aliases) > maxAliases || len(a.Forward) > maxForward {
		return a, errors.New("Zu viele Aliase oder Weiterleitungen")
	}
	seen := map[string]bool{a.Local: true}
	aliases := []string{}
	for _, al := range a.Aliases {
		if strings.TrimSpace(al) == "" {
			continue
		}
		l, err := NormalizeLocal(al)
		if err != nil {
			return a, fmt.Errorf("Alias %q: %w", al, err)
		}
		if !seen[l] {
			seen[l] = true
			aliases = append(aliases, l)
		}
	}
	a.Aliases = aliases
	fwd := []string{}
	seenFwd := map[string]bool{}
	for _, f := range a.Forward {
		if strings.TrimSpace(f) == "" {
			continue
		}
		addr, err := NormalizeAddress(f)
		if err != nil {
			return a, err
		}
		if addr == a.Address() {
			continue
		}
		if !seenFwd[addr] {
			seenFwd[addr] = true
			fwd = append(fwd, addr)
		}
	}
	a.Forward = fwd
	if a.ForwardOnly && len(a.Forward) == 0 {
		return a, errors.New("„Nur weiterleiten“ braucht mindestens eine Zieladresse")
	}
	return a, nil
}
