package mail

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
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
		days, sieveQuote(a.VacationSubject), list.String(), sieveText(a.VacationBody))
	return b.String()
}

// sieveQuote builds a quoted Sieve string (RFC 5228 2.4.2). Sieve knows only
// the escapes \\ and \", so line breaks become spaces; a subject is one line.
func sieveQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return `"` + s + `"`
}

// sieveText builds a multi-line Sieve string: "text:", the lines with a
// leading dot doubled, and a lone dot as terminator.
func sieveText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var b strings.Builder
	b.WriteString("text:\n")
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, ".") {
			b.WriteString(".")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(".\n")
	return b.String()
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
			_ = removeInVmail(sieveRel(a.Domain, a.Local))
			continue
		}
		if err := writeVacation(a); err != nil {
			return err
		}
	}
	return nil
}

// /var/vmail and everything below belong to the mailbox user vmail. The
// agent (root) only works there through an os.Root, so a symlink planted by
// a compromised mail process cannot lead it to other files.
func openVmail() (*os.Root, error) {
	return os.OpenRoot(root(vmailDir))
}

func sieveRel(domain, local string) string {
	return filepath.Join(domain, local, ".dovecot.sieve")
}

func removeInVmail(rel string) error {
	r, err := openVmail()
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.Remove(rel); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
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
	script := vacationSieve(a)
	if script == "" {
		return removeInVmail(sieveRel(a.Domain, a.Local))
	}
	r, err := openVmail()
	if errors.Is(err, fs.ErrNotExist) {
		// Mail server not installed yet; the install job creates /var/vmail
		// and the next apply writes the script.
		return nil
	}
	if err != nil {
		return err
	}
	defer r.Close()
	if err := ensureMailboxHome(r, a.Domain, a.Local); err != nil {
		return err
	}
	return writeOwned(r, sieveRel(a.Domain, a.Local), script, 0o600, "vmail", "vmail")
}

// ensureMailboxHome creates <domain>/<local> below the vmail root, owned by
// vmail. Lchown never follows a link.
func ensureMailboxHome(r *os.Root, domain, local string) error {
	uid, gid, err := lookupOwner("vmail", "vmail")
	if err != nil {
		return err
	}
	for _, rel := range []string{domain, filepath.Join(domain, local)} {
		if err := r.Mkdir(rel, 0o750); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		fi, err := r.Lstat(rel)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("%s ist kein Verzeichnis", rel)
		}
		if uid >= 0 {
			if err := r.Lchown(rel, uid, gid); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeOwned replaces rel below r atomically with a file owned by the
// mailbox user. Mode and owner are set through the open file.
func writeOwned(r *os.Root, rel, body string, mode os.FileMode, owner, group string) error {
	uid, gid, err := lookupOwner(owner, group)
	if err != nil {
		return err
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(rel), ".godeb-"+hex.EncodeToString(suffix))
	f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer r.Remove(tmp)
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if uid >= 0 {
		if err := f.Chown(uid, gid); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return r.Rename(tmp, rel)
}

// lookupOwner resolves owner and group; -1 when not running as root (tests),
// where chown is skipped.
func lookupOwner(owner, group string) (int, int, error) {
	if os.Geteuid() != 0 {
		return -1, -1, nil
	}
	u, err := user.Lookup(owner)
	if err != nil {
		return 0, 0, fmt.Errorf("Benutzer %s fehlt: %w", owner, err)
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return 0, 0, fmt.Errorf("Gruppe %s fehlt: %w", group, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(g.Gid)
	return uid, gid, nil
}
