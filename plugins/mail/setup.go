package mail

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
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

// DomainOptions adjust AddDomain. The zero value gives the default: a
// postmaster mailbox of its own that collects the role addresses.
type DomainOptions struct {
	// Mailbox is the local part of a first personal mailbox in this domain
	// (e.g. "anna" for anna@example.com), created with Password.
	Mailbox  string
	Password string
	// RoleTarget sends postmaster, abuse, dmarc and tls-reports to this
	// existing (or just created) address instead of a postmaster mailbox:
	// as aliases when it lies in the same domain, otherwise through a
	// forward-only postmaster address.
	RoleTarget string
}

// DomainSetup is what AddDomain created. Password is only set when a
// postmaster mailbox was created now; GoDeb keeps only its hash.
type DomainSetup struct {
	Domain     Domain   `json:"domain"`
	Mailbox    string   `json:"mailbox,omitempty"` // first personal mailbox created now
	Postmaster string   `json:"postmaster,omitempty"`
	Password   string   `json:"password,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	RoleTarget string   `json:"roleTarget,omitempty"`
	Notes      []string `json:"notes"`
}

// ErrDomainExists is returned by AddDomain for a domain that is set up and
// needed no change.
var ErrDomainExists = errors.New("Domain ist bereits eingerichtet")

// CheckMailboxPassword validates a password before anything is created.
func CheckMailboxPassword(password string) error { return checkPassword(password) }

// AddDomain creates a mail domain with its DKIM key, optionally a first
// mailbox, and the role addresses (postmaster, abuse, dmarc, tls-reports)
// so the reports the DNS records ask for have a place to go. Then it
// writes the configuration. An existing domain only gets what is missing.
func AddDomain(ctx context.Context, name string, opt DomainOptions) (DomainSetup, error) {
	name, err := NormalizeDomain(name)
	if err != nil {
		return DomainSetup{}, err
	}
	if !Installed() {
		return DomainSetup{}, errors.New("Der Mailserver ist noch nicht eingerichtet")
	}
	target := ""
	if strings.TrimSpace(opt.RoleTarget) != "" {
		if target, err = NormalizeAddress(opt.RoleTarget); err != nil {
			return DomainSetup{}, err
		}
	}
	var first Account
	if strings.TrimSpace(opt.Mailbox) != "" {
		if first, err = cleanAccount(Account{Domain: name, Local: opt.Mailbox, Created: time.Now().UTC().Format(time.RFC3339)}); err != nil {
			return DomainSetup{}, err
		}
		if first.Hash, err = HashPassword(opt.Password); err != nil {
			return DomainSetup{}, err
		}
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
	res := DomainSetup{Domain: d, RoleTarget: target}
	if _, err := findAccount(accounts, first.Address()); first.Local != "" && err == nil {
		// Already there (wizard run twice): keep it and its password.
		first.Local = ""
	}
	if first.Local != "" {
		if err := checkAccount(accounts, first, ""); err != nil {
			return res, err
		}
		if err := saveAccount(first); err != nil {
			return res, err
		}
		accounts = append(accounts, first)
		res.Mailbox = first.Address()
		slog.InfoContext(ctx, "Postfach angelegt", "adresse", first.Address())
	}
	if target != "" {
		res.Aliases, err = routeRoles(name, accounts, target)
	} else {
		res.Postmaster, res.Password, res.Aliases, err = ensurePostmaster(name, accounts)
	}
	if err != nil {
		return res, err
	}
	res.Notes, err = apply(ctx)
	if err == nil && exists && res.Mailbox == "" && res.Postmaster == "" && len(res.Aliases) == 0 {
		err = ErrDomainExists
	}
	return res, err
}

// routeRoles sends the free role addresses of domain to target: as aliases
// of the target mailbox when it is in this domain, otherwise through a
// forward-only postmaster address. Taken addresses stay as they are.
func routeRoles(domain string, accounts []Account, target string) ([]string, error) {
	locals := append([]string{"postmaster"}, roleAliases...)
	var free []string
	for _, local := range locals {
		if !conflict(accounts, local+"@"+domain, "") {
			free = append(free, local)
		}
	}
	if len(free) == 0 {
		return nil, nil
	}
	added := make([]string, 0, len(free))
	for _, local := range free {
		added = append(added, local+"@"+domain)
	}
	if strings.HasSuffix(target, "@"+domain) {
		for i, a := range accounts {
			if a.Address() == target {
				accounts[i].Aliases = append(accounts[i].Aliases, free...)
				return added, saveAccount(accounts[i])
			}
		}
		return nil, errors.New(target + " gibt es nicht")
	}
	// postmaster@ carries the other role addresses as aliases; if it is
	// taken, the first free one does.
	fwd := Account{Domain: domain, Local: free[0], Aliases: free[1:], Forward: []string{target}, ForwardOnly: true,
		Created: time.Now().UTC().Format(time.RFC3339)}
	fwd, err := cleanAccount(fwd)
	if err != nil {
		return nil, err
	}
	return added, saveAccount(fwd)
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

// RandomPassword returns a password for a new mailbox (command line).
func RandomPassword() (string, error) { return randomPassword(20) }

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
