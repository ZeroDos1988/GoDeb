package mail

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxVacationSubject  = 200
	maxVacationBody     = 4000
	minVacationDays     = 1
	maxVacationDays     = 30
	defaultVacationDays = 7
)

// cleanVacation validates the absence notice fields of an account.
func cleanVacation(a Account) (Account, error) {
	if a.ForwardOnly {
		a.Vacation = false
		a.VacationSubject, a.VacationBody, a.VacationDays = "", "", 0
		return a, nil
	}
	if !a.Vacation {
		a.VacationSubject, a.VacationBody = strings.TrimSpace(a.VacationSubject), strings.TrimSpace(a.VacationBody)
		if a.VacationDays == 0 {
			a.VacationDays = defaultVacationDays
		}
		return a, nil
	}
	a.VacationSubject = strings.TrimSpace(a.VacationSubject)
	a.VacationBody = strings.TrimSpace(a.VacationBody)
	if a.VacationSubject == "" {
		a.VacationSubject = "Abwesend"
	}
	if a.VacationBody == "" {
		return a, fmt.Errorf("Abwesenheitsnotiz braucht einen Text")
	}
	if len(a.VacationSubject) > maxVacationSubject {
		return a, fmt.Errorf("Betreff der Abwesenheit maximal %d Zeichen", maxVacationSubject)
	}
	if len(a.VacationBody) > maxVacationBody {
		return a, fmt.Errorf("Text der Abwesenheit maximal %d Zeichen", maxVacationBody)
	}
	if err := sieveSafeText(a.VacationSubject); err != nil {
		return a, fmt.Errorf("Betreff: %w", err)
	}
	if err := sieveSafeText(a.VacationBody); err != nil {
		return a, fmt.Errorf("Text: %w", err)
	}
	if a.VacationDays < minVacationDays || a.VacationDays > maxVacationDays {
		return a, fmt.Errorf("Wiederholung der Abwesenheit: %d–%d Tage", minVacationDays, maxVacationDays)
	}
	return a, nil
}

func sieveSafeText(s string) error {
	for _, r := range s {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') {
			return fmt.Errorf("enthält Steuerzeichen")
		}
	}
	return nil
}

// vacationSieve builds the personal Sieve script for one mailbox, or "" when
// the absence note is off (caller removes the active script).
func vacationSieve(a Account) string {
	if !a.Vacation || a.ForwardOnly {
		return ""
	}
	addrs := []string{a.Address()}
	for _, al := range a.Aliases {
		addrs = append(addrs, al+"@"+a.Domain)
	}
	var list strings.Builder
	for i, addr := range addrs {
		if i > 0 {
			list.WriteString(", ")
		}
		list.WriteString(sieveQuote(addr))
	}
	days := a.VacationDays
	if days < minVacationDays || days > maxVacationDays {
		days = defaultVacationDays
	}
	var b strings.Builder
	b.WriteString(header)
	b.WriteString(`require ["vacation"];` + "\n")
	fmt.Fprintf(&b, "vacation :days %d :subject %s :addresses [%s] %s;\n",
		days, sieveQuote(a.VacationSubject), list.String(), sieveQuote(a.VacationBody))
	return b.String()
}

func sieveQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\r\n", `\n`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\n`)
	return `"` + s + `"`
}

func mailboxHome(domain, local string) string {
	return filepath.Join(root(vmailDir), domain, local)
}

func personalSievePath(domain, local string) string {
	return filepath.Join(mailboxHome(domain, local), ".dovecot.sieve")
}

// syncVacation writes or removes the personal Sieve script for every mailbox.
func syncVacation(accounts []Account) error {
	for _, a := range accounts {
		if a.ForwardOnly {
			_ = os.Remove(personalSievePath(a.Domain, a.Local))
			continue
		}
		if err := writeVacation(a); err != nil {
			return err
		}
	}
	return nil
}

func writeVacation(a Account) error {
	if _, err := NormalizeDomain(a.Domain); err != nil {
		return err
	}
	if _, err := NormalizeLocal(a.Local); err != nil {
		return err
	}
	home := mailboxHome(a.Domain, a.Local)
	active := personalSievePath(a.Domain, a.Local)
	script := vacationSieve(a)
	if script == "" {
		_ = os.Remove(active)
		return nil
	}
	if err := ensureMailboxHome(home); err != nil {
		return err
	}
	return writeOwned(active, script, 0o600, "vmail", "vmail")
}

func ensureMailboxHome(home string) error {
	base := filepath.Clean(root(vmailDir)) + string(os.PathSeparator)
	clean := filepath.Clean(home)
	if !strings.HasPrefix(clean+string(os.PathSeparator), base) {
		return fmt.Errorf("Pfad außerhalb von /var/vmail")
	}
	if err := os.MkdirAll(clean, 0o750); err != nil {
		return err
	}
	return chownPath(clean, "vmail", "vmail")
}

// writeOwned writes a file under /var/vmail owned by the mailbox user.
func writeOwned(full, body string, mode os.FileMode, owner, group string) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
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
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	if err := chownPath(tmp.Name(), owner, group); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), full)
}

func chownPath(path, owner, group string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	u, err := user.Lookup(owner)
	if err != nil {
		return fmt.Errorf("Benutzer %s fehlt: %w", owner, err)
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return fmt.Errorf("Gruppe %s fehlt: %w", group, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(g.Gid)
	return os.Chown(path, uid, gid)
}
