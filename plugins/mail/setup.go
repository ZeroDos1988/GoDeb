package mail

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"math/big"
	"strconv"
	"time"
)

// roleAliases are the addresses every mail domain should have: abuse
// (RFC 2142) and the report addresses of the DMARC and TLS-RPT records.
// They land in the postmaster mailbox.
var roleAliases = []string{"abuse", "dmarc", "tls-reports"}

// Installed reports whether the mail server is set up on this host.
func Installed() bool {
	_, ok, err := loadSettings()
	return err == nil && ok
}

// Hostname returns the configured MX host name, "" before the setup.
func Hostname() string {
	s, ok, err := loadSettings()
	if err != nil || !ok {
		return ""
	}
	return s.Hostname
}

// JobState is the running or last background job (installation, ClamAV,
// certificate), nil if there was none since the start.
func JobState() *Job { return jobs.snapshot() }

// StartInstall installs Postfix, Dovecot, Rspamd and Redis in the
// background. The plugin view and the setup wizard use it.
func StartInstall(hostname string, clamav bool) error {
	host, err := NormalizeHostname(hostname)
	if err != nil {
		return err
	}
	if n := gatewayDomains(); n > 0 {
		return errors.New("Das Mail-Gateway ist eingerichtet (" + strconv.Itoa(n) + " Relay-Domains). Beide konfigurieren Postfix; erst die Domains im Mail-Gateway entfernen.")
	}
	s, _, err := loadSettings()
	if err != nil {
		return err
	}
	s.Hostname, s.ClamAV = host, clamav
	if err := jobs.start("install", func(ctx context.Context) error { return install(ctx, s) }); err != nil {
		return err
	}
	slog.Info("Mailserver-Einrichtung gestartet", "hostname", host, "clamav", clamav)
	return nil
}

// StartCertificate requests the Let's Encrypt certificate for the host name
// in the background.
func StartCertificate(email string) error {
	if !Installed() {
		return errors.New("Der Mailserver ist noch nicht eingerichtet")
	}
	return jobs.start("certificate", func(ctx context.Context) error { return certificate(ctx, email) })
}

// DomainSetup is what AddDomain created. Password is only set when the
// postmaster mailbox was created now; GoDeb keeps only its hash.
type DomainSetup struct {
	Domain     Domain   `json:"domain"`
	Postmaster string   `json:"postmaster,omitempty"`
	Password   string   `json:"password,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	Notes      []string `json:"notes"`
}

// ErrDomainExists is returned by AddDomain for a domain that is set up.
var ErrDomainExists = errors.New("Domain ist bereits eingerichtet")

// AddDomain creates a mail domain with its DKIM key and the postmaster
// mailbox (aliases abuse, dmarc and tls-reports, so the reports the DNS
// records ask for have a place to go),
// then writes the configuration. An existing domain only gets the missing
// role addresses.
func AddDomain(ctx context.Context, name string) (DomainSetup, error) {
	name, err := NormalizeDomain(name)
	if err != nil {
		return DomainSetup{}, err
	}
	if !Installed() {
		return DomainSetup{}, errors.New("Der Mailserver ist noch nicht eingerichtet")
	}
	domains, accounts, err := loadAll()
	if err != nil {
		return DomainSetup{}, err
	}
	d, err := findDomain(domains, name)
	exists := err == nil
	if !exists {
		priv, pub, err := newDKIMKey()
		if err != nil {
			return DomainSetup{}, err
		}
		d = Domain{Name: name, Selector: "godeb", DKIMPublic: pub, Created: time.Now().UTC().Format(time.RFC3339)}
		if err := writeDKIM(d.Name, d.Selector, priv); err != nil {
			return DomainSetup{}, err
		}
		if err := saveDomain(d); err != nil {
			return DomainSetup{}, err
		}
		slog.InfoContext(ctx, "Maildomain angelegt", "domain", name)
	}
	res := DomainSetup{Domain: d}
	if res.Postmaster, res.Password, res.Aliases, err = ensurePostmaster(d.Name, accounts); err != nil {
		return res, err
	}
	res.Notes, err = apply(ctx)
	if err == nil && exists && res.Postmaster == "" {
		err = ErrDomainExists
	}
	return res, err
}

// ensurePostmaster creates postmaster@domain with a random password and
// the role aliases that are still free. Addresses that already exist
// (as mailbox or alias) are left alone.
func ensurePostmaster(domain string, accounts []Account) (address, password string, aliases []string, err error) {
	pm := Account{Domain: domain, Local: "postmaster"}
	if conflict(accounts, pm.Address(), "") {
		// The address exists; only add free role aliases to that mailbox.
		for i, a := range accounts {
			if a.Address() != pm.Address() {
				continue
			}
			added := false
			for _, local := range roleAliases {
				if !conflict(accounts, local+"@"+domain, "") {
					accounts[i].Aliases = append(accounts[i].Aliases, local)
					aliases = append(aliases, local+"@"+domain)
					added = true
				}
			}
			if added {
				err = saveAccount(accounts[i])
			}
		}
		return "", "", aliases, err
	}
	for _, local := range roleAliases {
		if !conflict(accounts, local+"@"+domain, "") {
			pm.Aliases = append(pm.Aliases, local)
			aliases = append(aliases, local+"@"+domain)
		}
	}
	if password, err = randomPassword(20); err != nil {
		return "", "", nil, err
	}
	if pm.Hash, err = HashPassword(password); err != nil {
		return "", "", nil, err
	}
	pm.QuotaMB = 1024
	pm.Created = time.Now().UTC().Format(time.RFC3339)
	if pm, err = cleanAccount(pm); err != nil {
		return "", "", nil, err
	}
	if err = saveAccount(pm); err != nil {
		return "", "", nil, err
	}
	return pm.Address(), password, aliases, nil
}

// randomPassword returns n characters without look-alikes (0/O, 1/l/I).
func randomPassword(n int) (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	out := make([]byte, n)
	for i := range out {
		k, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[k.Int64()]
	}
	return string(out), nil
}

// dmarcReports is the address DMARC aggregate reports go to, "" when the
// domain has no such address (then the record asks for none).
func dmarcReports(domain string, accounts []Account) string {
	addr := "dmarc@" + domain
	if conflict(accounts, addr, "") {
		return addr
	}
	return ""
}
