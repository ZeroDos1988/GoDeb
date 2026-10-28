package mail

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"godeb/backend/core"
)

// fileSpec is one file GoDeb may write: mode and group.
type fileSpec struct {
	mode  os.FileMode
	group string // "" = root
}

// allowed lists every file the plugin writes. DKIM keys are checked by
// directory and name pattern in writeDKIM.
var allowed = map[string]fileSpec{
	mapDomains:   {0o644, ""},
	mapMailboxes: {0o640, "postfix"},
	mapAliases:   {0o640, "postfix"},
	mapSenders:   {0o640, "postfix"},
	dovecotConf:  {0o644, ""},
	dovecotUsers: {0o640, "dovecot"},
	dovecotDeny:  {0o640, "dovecot"},
	spamSieve:    {0o644, ""},
	fail2banJail: {0o644, ""},

	rspamdLocal + "/dkim_signing.conf":     {0o644, ""},
	rspamdLocal + "/arc.conf":              {0o644, ""},
	rspamdLocal + "/actions.conf":          {0o644, ""},
	rspamdLocal + "/milter_headers.conf":   {0o644, ""},
	rspamdLocal + "/redis.conf":            {0o644, ""},
	rspamdLocal + "/classifier-bayes.conf": {0o644, ""},
	rspamdLocal + "/antivirus.conf":        {0o644, ""},
}

// root prefixes every path in tests (GODEB_MAIL_ROOT); empty in production.
func root(path string) string {
	if r := strings.TrimSpace(os.Getenv("GODEB_MAIL_ROOT")); r != "" {
		return filepath.Join(r, path)
	}
	return path
}

// writeFile replaces an allowlisted file atomically with the given owner.
func writeFile(path, body string) error {
	spec, ok := allowed[path]
	if !ok {
		return fmt.Errorf("%s steht nicht auf der Liste der Mail-Dateien", path)
	}
	return writeAs(root(path), body, spec)
}

func writeAs(full, body string, spec fileSpec) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".godeb-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), spec.mode); err != nil {
		return err
	}
	if spec.group != "" && os.Geteuid() == 0 {
		g, err := user.LookupGroup(spec.group)
		if err != nil {
			return fmt.Errorf("Gruppe %s fehlt: %w", spec.group, err)
		}
		gid, _ := strconv.Atoi(g.Gid)
		if err := os.Chown(tmp.Name(), 0, gid); err != nil {
			return err
		}
	}
	return os.Rename(tmp.Name(), full)
}

// writeDKIM stores a domain's private key for Rspamd (root:_rspamd 0640).
func writeDKIM(domain, selector, privatePEM string) error {
	if _, err := NormalizeDomain(domain); err != nil {
		return err
	}
	if !selectorPattern.MatchString(selector) {
		return errors.New("DKIM-Selector ist ungültig")
	}
	return writeAs(root(dkimKeyPath(domain, selector)), privatePEM, fileSpec{0o640, "_rspamd"})
}

func removeDKIM(domain, selector string) {
	if _, err := NormalizeDomain(domain); err != nil || !selectorPattern.MatchString(selector) {
		return
	}
	_ = os.Remove(root(dkimKeyPath(domain, selector)))
}

// removeMailbox deletes the maildir of an address under /var/vmail. Both
// parts were validated, so the path cannot leave the directory.
func removeMailbox(domain, local string) error {
	if _, err := NormalizeDomain(domain); err != nil {
		return err
	}
	if local != "" {
		if _, err := NormalizeLocal(local); err != nil {
			return err
		}
	}
	dir := filepath.Join(root(vmailDir), domain, local)
	base := filepath.Clean(root(vmailDir)) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(dir)+string(os.PathSeparator), base) || filepath.Clean(dir) == filepath.Clean(root(vmailDir)) {
		return errors.New("Pfad außerhalb von /var/vmail")
	}
	return os.RemoveAll(dir)
}

// admin and adminTimeout run system commands; tests replace them.
var (
	admin        = core.RunAdmin
	adminTimeout = core.RunAdminTimeout
)

// applyMu serialises rewrites of the configuration.
var applyMu sync.Mutex

// apply writes all files from the database and reloads the services. It
// is the only way GoDeb changes the mail configuration.
func apply(ctx context.Context) ([]string, error) {
	applyMu.Lock()
	defer applyMu.Unlock()
	s, installed, err := loadSettings()
	if err != nil {
		return nil, err
	}
	if !installed {
		return nil, errors.New("Der Mailserver ist noch nicht eingerichtet")
	}
	domains, accounts, err := loadAll()
	if err != nil {
		return nil, err
	}
	log := []string{}
	dom, mbox, ali, snd := postfixMaps(domains, accounts)
	users, deny := dovecotUsersFile(domains, accounts)
	files := map[string]string{
		mapDomains: dom, mapMailboxes: mbox, mapAliases: ali, mapSenders: snd,
		dovecotConf: dovecotConfig(s), dovecotUsers: users, dovecotDeny: deny,
		spamSieve: spamSieveScript, fail2banJail: fail2banConfig,
	}
	for p, body := range rspamdFiles(s, domains) {
		files[p] = body
	}
	for p, body := range files {
		if err := writeFile(p, body); err != nil {
			return log, err
		}
	}
	if res := admin(ctx, "postconf", append([]string{"-e"}, postfixMain(s)...)...); res.Err != nil {
		return log, fmt.Errorf("postconf: %s", res.Detail())
	}
	services, params := postfixMaster()
	for _, svc := range services {
		if res := admin(ctx, "postconf", "-M", svc); res.Err != nil {
			return log, fmt.Errorf("postconf -M: %s", res.Detail())
		}
	}
	if res := admin(ctx, "postconf", append([]string{"-P"}, params...)...); res.Err != nil {
		return log, fmt.Errorf("postconf -P: %s", res.Detail())
	}
	if err := syncVacation(accounts); err != nil {
		return log, err
	}
	if err := syncMTASTS(s, domains); err != nil {
		return log, err
	}
	if res := admin(ctx, "sievec", root(spamSieve)); res.Err != nil {
		log = append(log, "sievec: "+res.Detail())
	}
	for _, a := range accounts {
		if vacationSieve(a) == "" {
			continue
		}
		if res := admin(ctx, "sievec", personalSievePath(a.Domain, a.Local)); res.Err != nil {
			log = append(log, "sievec "+a.Address()+": "+res.Detail())
		}
	}
	for _, unit := range []string{"postfix", "dovecot", "rspamd"} {
		if res := adminTimeout(ctx, time.Minute, "systemctl", "reload-or-restart", unit); res.Err != nil {
			log = append(log, unit+": "+res.Detail())
		}
	}
	if res := admin(ctx, "fail2ban-client", "reload"); res.Err != nil {
		log = append(log, "fail2ban: "+res.Detail())
	}
	return log, nil
}

// conflict reports whether address is already used by another mailbox or
// alias of the same domain (except the account itself, for edits).
func conflict(accounts []Account, address, self string) bool {
	for _, a := range accounts {
		if a.Address() == self {
			continue
		}
		if a.Address() == address {
			return true
		}
		for _, al := range a.Aliases {
			if al+"@"+a.Domain == address {
				return true
			}
		}
	}
	return false
}

// checkAccount refuses addresses or aliases that another account already
// uses.
func checkAccount(accounts []Account, a Account, self string) error {
	if conflict(accounts, a.Address(), self) {
		return fmt.Errorf("%s ist bereits vergeben", a.Address())
	}
	for _, al := range a.Aliases {
		addr := al + "@" + a.Domain
		if conflict(accounts, addr, self) {
			return fmt.Errorf("Alias %s ist bereits vergeben", addr)
		}
	}
	return nil
}
