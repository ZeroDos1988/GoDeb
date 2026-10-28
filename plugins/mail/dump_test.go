package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDumpConfig writes the rendered configuration for an example setup
// into $GODEB_MAIL_DUMP, so it can be checked with the real postconf,
// doveconf and rspamadm (see scripts or the commit message). Skipped
// without the variable.
func TestDumpConfig(t *testing.T) {
	dir := os.Getenv("GODEB_MAIL_DUMP")
	if dir == "" {
		t.Skip("GODEB_MAIL_DUMP not set")
	}
	s, domains, accounts := exampleSetup(t)
	write := func(path, body string) {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("postconf-e.txt", strings.Join(postfixMain(s), "\n")+"\n")
	services, params := postfixMaster()
	write("postconf-M.txt", strings.Join(services, "\n")+"\n")
	write("postconf-P.txt", strings.Join(params, "\n")+"\n")
	d, m, a, snd := postfixMaps(domains, accounts)
	write(mapDomains, d)
	write(mapMailboxes, m)
	write(mapAliases, a)
	write(mapSenders, snd)
	users, deny := dovecotUsersFile(domains, accounts)
	write(dovecotUsers, users)
	write(dovecotDeny, deny)
	write(dovecotConf, dovecotConfig(s))
	write(spamSieve, spamSieveScript)
	write("vacation.sieve", vacationSieve(Account{
		Domain: "example.com", Local: "info", Aliases: []string{"kontakt"}, Vacation: true, VacationDays: 7,
		VacationSubject: `Abwesend "Büro"`, VacationBody: "Hallo,\n\n.punkt am Zeilenanfang\nBis bald.",
	}))
	for p, body := range rspamdFiles(s, domains) {
		write(p, body)
	}
	write(fail2banJail, fail2banConfig)
}

func exampleSetup(t *testing.T) (Settings, []Domain, []Account) {
	t.Helper()
	hash, err := HashPassword("Correct-Horse-9")
	if err != nil {
		t.Fatal(err)
	}
	s := Settings{Hostname: "mail.example.com", CertFile: snakeoilCert, KeyFile: snakeoilKey, ClamAV: true}
	domains := []Domain{
		{Name: "example.com", Selector: "godeb", CatchAll: "info@example.com"},
		{Name: "example.org", Selector: "godeb"},
		{Name: "old.example", Selector: "godeb", Suspended: true},
	}
	accounts := []Account{
		{Domain: "example.com", Local: "info", Hash: hash, QuotaMB: 1024, Aliases: []string{"kontakt", "postmaster"}},
		{Domain: "example.com", Local: "anna", Hash: hash, Forward: []string{"backup@example.org"}},
		{Domain: "example.com", Local: "shop", ForwardOnly: true, Forward: []string{"info@example.com", "x@gmail.com"}},
		{Domain: "example.org", Local: "backup", Hash: hash, Suspended: true},
		{Domain: "old.example", Local: "alt", Hash: hash},
	}
	return s, domains, accounts
}
