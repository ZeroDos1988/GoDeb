package doctor

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// FileStat is what the installation check needs to know about a path. It
// comes from Lstat, so a symlink is reported as one and never followed.
type FileStat struct {
	Mode         fs.FileMode
	Owner, Group string
}

// statFile is Env.Stat on the real host.
func statFile(path string) (FileStat, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return FileStat{}, err
	}
	st := FileStat{Mode: fi.Mode()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		st.Owner, st.Group = userName(sys.Uid), groupName(sys.Gid)
	}
	return st, nil
}

// walkFiles is Env.Walk on the real host: every entry below root, without
// following symlinks.
func walkFiles(root string, fn func(path string, st FileStat)) error {
	return filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are reported by the caller's own checks
		}
		if st, err := statFile(p); err == nil {
			fn(p, st)
		}
		return nil
	})
}

var (
	idMu    sync.Mutex
	idNames = map[string]string{}
)

func userName(uid uint32) string {
	return cachedName("u"+strconv.Itoa(int(uid)), func() (string, error) {
		u, err := user.LookupId(strconv.Itoa(int(uid)))
		if err != nil {
			return "", err
		}
		return u.Username, nil
	}, uid)
}

func groupName(gid uint32) string {
	return cachedName("g"+strconv.Itoa(int(gid)), func() (string, error) {
		g, err := user.LookupGroupId(strconv.Itoa(int(gid)))
		if err != nil {
			return "", err
		}
		return g.Name, nil
	}, gid)
}

func cachedName(key string, lookup func() (string, error), id uint32) string {
	idMu.Lock()
	defer idMu.Unlock()
	if n, ok := idNames[key]; ok {
		return n
	}
	n, err := lookup()
	if err != nil {
		n = strconv.Itoa(int(id))
	}
	idNames[key] = n
	return n
}

// Paths the package creates (postinst, systemd units) with their expected
// mode and owner.
type wantPath struct {
	Path         string
	Mode         fs.FileMode
	Owner, Group string
	Optional     bool
}

var installPaths = []wantPath{
	{Path: "/var/lib/godeb", Mode: 0o700, Owner: "godeb", Group: "godeb"},
	{Path: "/etc/godeb", Mode: 0o700, Owner: "root", Group: "root"},
	{Path: "/etc/wireguard", Mode: 0o700, Owner: "root", Group: "root"},
	{Path: "/var/log/godeb", Mode: 0o750, Owner: "godeb", Group: "adm"},
	{Path: "/var/log/godeb/auth.log", Mode: 0o640, Owner: "godeb", Group: "adm"},
	{Path: "/usr/bin/godeb", Mode: 0o755, Owner: "root", Group: "root"},
	{Path: "/run/godeb-agent/agent.sock", Mode: 0o660, Owner: "root", Group: "godeb"},
}

// Secret files: only their owner may read them. Missing files are fine
// (not every host has mail settings or a tunnel yet).
var secretPaths = []struct{ Path, Owner string }{
	{"/var/lib/godeb/master.key", "godeb"},
	{"/var/lib/godeb/godeb.db", "godeb"},
	{"/etc/godeb/master.key", "root"},
	{"/etc/godeb/godeb.db", "root"},
	{"/etc/godeb/smtp.json", "root"},
	{"/etc/wireguard/wg0.conf", "root"},
}

// checkInstall verifies what the package sets up: the system user, modes
// and owners, the two processes and their privileges, fail2ban and changed
// package files. It needs root to see /etc/godeb; the web process never
// runs it (Env.Stat is nil there).
func checkInstall(ctx context.Context, env Env) []Finding {
	const area = "Installation"
	if env.Stat == nil || env.LookupUser == nil {
		return nil
	}
	shell, ok := env.LookupUser("godeb")
	if !ok {
		return []Finding{{Area: area, Level: Info, Text: "kein Paketbetrieb (Benutzer godeb fehlt), Prüfung übersprungen"}}
	}
	var out []Finding
	if strings.HasSuffix(shell, "/nologin") || strings.HasSuffix(shell, "/false") {
		out = append(out, Finding{Area: area, Level: OK, Text: "Benutzer godeb ohne Login-Shell"})
	} else {
		out = append(out, Finding{Area: area, Level: Fail, Text: "Benutzer godeb hat die Login-Shell " + shell})
	}

	var bad []string
	for _, w := range installPaths {
		if msg := checkPath(env, w); msg != "" {
			bad = append(bad, msg)
		}
	}
	for _, s := range secretPaths {
		st, err := env.Stat(s.Path)
		if err != nil {
			continue
		}
		if st.Mode&fs.ModeSymlink != 0 || st.Mode.Perm()&0o077 != 0 || st.Owner != s.Owner {
			bad = append(bad, fmt.Sprintf("%s ist %s, erwartet 0600 %s", s.Path, describe(st), s.Owner))
		}
	}
	if env.Walk != nil {
		_ = env.Walk("/var/lib/godeb", func(p string, st FileStat) {
			switch {
			case st.Mode&fs.ModeSymlink != 0:
				bad = append(bad, p+" ist ein Symlink")
			case st.Owner != "godeb":
				bad = append(bad, p+" gehört "+st.Owner+" statt godeb")
			}
		})
		for _, root := range []string{"/etc/godeb", "/etc/wireguard"} {
			_ = env.Walk(root, func(p string, st FileStat) {
				if st.Mode.IsRegular() && st.Mode.Perm()&0o077 != 0 {
					bad = append(bad, fmt.Sprintf("%s ist für Gruppe oder andere lesbar (%s)", p, describe(st)))
				}
			})
		}
	}
	if len(bad) == 0 {
		out = append(out, Finding{Area: area, Level: OK, Text: "Rechte und Besitzer der GoDeb-Dateien stimmen"})
	} else {
		out = append(out, Finding{Area: area, Level: Fail, Text: fmt.Sprintf("%d Rechte oder Besitzer falsch", len(bad)), Details: bad})
	}

	out = append(out, processFindings(ctx, env)...)
	out = append(out, fail2banFindings(ctx, env)...)
	out = append(out, packageFindings(ctx, env)...)
	return out
}

func checkPath(env Env, w wantPath) string {
	st, err := env.Stat(w.Path)
	if err != nil {
		if w.Optional {
			return ""
		}
		return w.Path + " fehlt"
	}
	if st.Mode&fs.ModeSymlink != 0 {
		return w.Path + " ist ein Symlink"
	}
	if st.Mode.Perm() != w.Mode || st.Owner != w.Owner || st.Group != w.Group {
		return fmt.Sprintf("%s ist %s, erwartet %04o %s:%s", w.Path, describe(st), w.Mode, w.Owner, w.Group)
	}
	return ""
}

func describe(st FileStat) string {
	return fmt.Sprintf("%04o %s:%s", st.Mode.Perm(), st.Owner, st.Group)
}

// processFindings checks that the web interface runs as godeb without
// capabilities and the agent as root.
func processFindings(ctx context.Context, env Env) []Finding {
	const area = "Installation"
	var out []Finding
	for _, p := range []struct {
		unit, want string
		noCaps     bool
	}{
		{"godeb.service", "godeb", true},
		{"godeb-agent.service", "root", false},
	} {
		res := env.Run(ctx, "systemctl", "show", "-p", "MainPID", "--value", p.unit)
		pid := strings.TrimSpace(res.Stdout)
		if res.Err != nil || pid == "" || pid == "0" {
			continue // not running: the services check already reports it
		}
		uid, caps, ok := parseProcStatus(env, pid)
		if !ok {
			out = append(out, Finding{Area: area, Level: Info, Text: p.unit + ": Prozess nicht lesbar"})
			continue
		}
		name := userName(uint32(uid))
		if env.UserOf != nil {
			name = env.UserOf(uid)
		}
		switch {
		case name != p.want:
			out = append(out, Finding{Area: area, Level: Fail, Text: fmt.Sprintf("%s läuft als %s statt %s", p.unit, name, p.want)})
		case p.noCaps && caps != 0:
			out = append(out, Finding{Area: area, Level: Fail, Text: fmt.Sprintf("%s hat Capabilities (%016x)", p.unit, caps)})
		case p.noCaps:
			out = append(out, Finding{Area: area, Level: OK, Text: p.unit + " läuft als " + name + " ohne Capabilities"})
		default:
			out = append(out, Finding{Area: area, Level: OK, Text: p.unit + " läuft als " + name})
		}
	}
	return out
}

// parseProcStatus reads the real uid and the effective capabilities.
func parseProcStatus(env Env, pid string) (uid int, caps uint64, ok bool) {
	if _, err := strconv.Atoi(pid); err != nil {
		return 0, 0, false
	}
	data, err := env.ReadFile("/proc/" + pid + "/status")
	if err != nil {
		return 0, 0, false
	}
	seenUID := false
	for _, l := range lines(string(data)) {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "Uid:":
			if v, err := strconv.Atoi(f[1]); err == nil {
				uid, seenUID = v, true
			}
		case "CapEff:":
			caps, _ = strconv.ParseUint(f[1], 16, 64)
		}
	}
	return uid, caps, seenUID
}

// fail2banFindings checks the two jails the package sets up.
func fail2banFindings(ctx context.Context, env Env) []Finding {
	const area = "Installation"
	var missingJails []string
	for _, jail := range []string{"godeb", "sshd"} {
		res := env.Run(ctx, "fail2ban-client", "status", jail)
		if missing(res) {
			return []Finding{{Area: area, Level: Warn, Text: "fail2ban nicht installiert"}}
		}
		if res.Err != nil {
			missingJails = append(missingJails, jail)
		}
	}
	if len(missingJails) > 0 {
		return []Finding{{Area: area, Level: Warn, Text: "fail2ban-Jail nicht aktiv: " + strings.Join(missingJails, ", "),
			Details: []string{"Neu einrichten: sudo /usr/share/godeb/scripts/setup-fail2ban.sh"}}}
	}
	return []Finding{{Area: area, Level: OK, Text: "fail2ban-Jails godeb und sshd aktiv"}}
}

// packageFindings lists package files that differ from the shipped ones.
// Changed conffiles (APT source, logrotate) are only information.
func packageFindings(ctx context.Context, env Env) []Finding {
	const area = "Installation"
	res := env.Run(ctx, "dpkg", "--verify", "godeb")
	if missing(res) {
		return nil
	}
	var changed, conf []string
	for _, l := range lines(res.Stdout) {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		path := f[len(f)-1]
		if len(f) >= 3 && f[1] == "c" {
			conf = append(conf, path)
		} else {
			changed = append(changed, path)
		}
	}
	var out []Finding
	if len(changed) > 0 {
		out = append(out, Finding{Area: area, Level: Warn, Text: fmt.Sprintf("%d Paketdateien verändert", len(changed)), Details: changed})
	} else if res.Err == nil {
		out = append(out, Finding{Area: area, Level: OK, Text: "Paketdateien unverändert"})
	}
	if len(conf) > 0 {
		out = append(out, Finding{Area: area, Level: Info, Text: "angepasste Konfigurationsdateien", Details: conf})
	}
	return out
}

// lookupUser is Env.LookupUser on the real host: the login shell of name.
func lookupUser(name string) (shell string, ok bool) {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return "", false
	}
	for _, l := range strings.Split(string(data), "\n") {
		f := strings.Split(l, ":")
		if len(f) == 7 && f[0] == name {
			return f[6], true
		}
	}
	return "", false
}
