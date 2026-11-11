package doctor

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"strings"
	"testing"
	"time"

	"godeb/backend/core"
)

func TestRedact(t *testing.T) {
	in := strings.Join([]string{
		"Endpoint 49.12.34.56:51820 und nochmal 49.12.34.56",
		"Tunnel 10.8.0.2, LAN 192.168.1.10, lo 127.0.0.1, Doku 203.0.113.5",
		"IPv6 2a01:4f8:c17:1234::1 und fe80::1 und ::1",
		"Zeit 10:15:30",
		"Host vps1 / vps1.example.org, mail.example.org, example.org",
		"godeb.service läuft, notexample.org bleibt",
	}, "\n")
	out := Redact(in, []string{"vps1", "vps1.example.org", "example.org", "mail.example.org", "godeb"})
	for _, gone := range []string{"49.12.34.56", "2a01:4f8", "vps1", "example.org"} {
		if strings.Contains(strings.ReplaceAll(out, "notexample.org", ""), gone) {
			t.Errorf("%q not redacted:\n%s", gone, out)
		}
	}
	for _, kept := range []string{"10.8.0.2", "192.168.1.10", "127.0.0.1", "203.0.113.5", "fe80::1", "10:15:30", "godeb.service", "notexample.org"} {
		if !strings.Contains(out, kept) {
			t.Errorf("%q should stay:\n%s", kept, out)
		}
	}
	if strings.Count(out, "<ip-1>") != 2 {
		t.Errorf("same address should share a placeholder:\n%s", out)
	}
}

func TestTextAndSummary(t *testing.T) {
	r := NewReport([]Finding{
		{Area: "Dienste", Level: OK, Text: "godeb läuft"},
		{Area: "Firewall", Level: Fail, Text: "ufw nicht aktiv"},
	}, time.Unix(0, 0))
	meta := Meta{Hostname: "vps1", Version: "0.2.22", OS: "Debian GNU/Linux 13 (trixie)", Arch: "amd64", Role: "hub"}
	text := Text(meta, r, time.Unix(0, 0).UTC())
	for _, want := range []string{"Host:      vps1", "Rolle:     hub", "== Firewall", "FAIL  ufw nicht aktiv", "Ergebnis: 1 Fehler, 0 Warnungen"} {
		if !strings.Contains(text, want) {
			t.Errorf("text misses %q:\n%s", want, text)
		}
	}
	sum := Summary(meta, r)
	if strings.Contains(sum, "godeb läuft") || !strings.Contains(sum, "- FAIL [Firewall] ufw nicht aktiv") {
		t.Errorf("summary should list only problems:\n%s", sum)
	}
}

func TestCheckInstall(t *testing.T) {
	files := map[string]FileStat{
		"/var/lib/godeb":              {Mode: fs.ModeDir | 0o700, Owner: "godeb", Group: "godeb"},
		"/etc/godeb":                  {Mode: fs.ModeDir | 0o700, Owner: "root", Group: "root"},
		"/etc/wireguard":              {Mode: fs.ModeDir | 0o755, Owner: "root", Group: "root"}, // wrong
		"/var/log/godeb":              {Mode: fs.ModeDir | 0o750, Owner: "godeb", Group: "godeb"},
		"/var/log/godeb/auth.log":     {Mode: 0o640, Owner: "godeb", Group: "godeb"},
		"/usr/bin/godeb":              {Mode: 0o755, Owner: "root", Group: "root"},
		"/run/godeb-agent/agent.sock": {Mode: fs.ModeSocket | 0o660, Owner: "root", Group: "godeb"},
		"/etc/godeb/master.key":       {Mode: 0o644, Owner: "root", Group: "root"}, // wrong
	}
	env := Env{
		Stat: func(p string) (FileStat, error) {
			if st, ok := files[p]; ok {
				return st, nil
			}
			return FileStat{}, fs.ErrNotExist
		},
		Walk: func(root string, fn func(string, FileStat)) error {
			if root == "/var/lib/godeb" {
				fn("/var/lib/godeb/link", FileStat{Mode: fs.ModeSymlink | 0o777, Owner: "godeb"})
			}
			return nil
		},
		LookupUser: func(string) (string, bool) { return "/usr/sbin/nologin", true },
		UserOf: func(uid int) string {
			if uid == 0 {
				return "root"
			}
			return "godeb"
		},
		ReadFile: func(p string) ([]byte, error) {
			switch p {
			case "/proc/100/status":
				return []byte("Uid:\t999\t999\t999\t999\nCapEff:\t0000000000000000\n"), nil
			case "/proc/200/status":
				return []byte("Uid:\t0\t0\t0\t0\nCapEff:\t000001ffffffffff\n"), nil
			}
			return nil, errors.New("nope")
		},
		Run: func(_ context.Context, name string, args ...string) core.CommandResult {
			switch name {
			case "systemctl":
				if args[len(args)-1] == "godeb.service" {
					return core.CommandResult{Stdout: "100\n"}
				}
				return core.CommandResult{Stdout: "200\n"}
			case "fail2ban-client":
				if args[1] == "sshd" {
					return core.CommandResult{Err: errors.New("exit 255")}
				}
				return core.CommandResult{}
			case "dpkg":
				return core.CommandResult{Stdout: "??5??????   /usr/bin/godeb\n??5?????? c /etc/logrotate.d/godeb\n", Err: errors.New("exit 1")}
			}
			return core.CommandResult{Err: exec.ErrNotFound}
		},
	}
	got := map[string]Finding{}
	for _, f := range checkInstall(context.Background(), env) {
		got[f.Text] = f
	}
	want := map[string]Level{
		"Benutzer godeb ohne Login-Shell":                 OK,
		"3 Rechte oder Besitzer falsch":                   Fail,
		"godeb.service läuft als godeb ohne Capabilities": OK,
		"godeb-agent.service läuft als root":              OK,
		"fail2ban-Jail nicht aktiv: sshd":                 Warn,
		"1 Paketdateien verändert":                        Warn,
		"angepasste Konfigurationsdateien":                Info,
	}
	for text, lvl := range want {
		f, ok := got[text]
		if !ok || f.Level != lvl {
			t.Errorf("missing %s %q; got %v", lvl, text, got)
		}
	}
	details := strings.Join(got["3 Rechte oder Besitzer falsch"].Details, "\n")
	for _, w := range []string{"/etc/wireguard ist 0755", "/etc/godeb/master.key ist 0644", "/var/lib/godeb/link ist ein Symlink"} {
		if !strings.Contains(details, w) {
			t.Errorf("details miss %q:\n%s", w, details)
		}
	}
}

func TestCheckInstallSkipsWithoutPackage(t *testing.T) {
	env := Env{Stat: statFile, LookupUser: func(string) (string, bool) { return "", false }}
	out := checkInstall(context.Background(), env)
	if len(out) != 1 || out[0].Level != Info {
		t.Fatalf("want one info finding, got %v", out)
	}
	if checkInstall(context.Background(), Env{}) != nil {
		t.Fatal("no Stat: no findings")
	}
}
