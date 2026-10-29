package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"godeb/backend/core"
)

// Job is the one long-running action (installation, ClamAV, certificate).
type Job struct {
	Action   string   `json:"action"`
	Running  bool     `json:"running"`
	OK       bool     `json:"ok"`
	Error    string   `json:"error,omitempty"`
	Started  string   `json:"started"`
	Finished string   `json:"finished,omitempty"`
	Log      []string `json:"log"`
}

type jobRunner struct {
	mu  sync.Mutex
	job *Job
}

var jobs jobRunner

func (r *jobRunner) snapshot() *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job == nil {
		return nil
	}
	cp := *r.job
	cp.Log = append([]string(nil), r.job.Log...)
	return &cp
}

func (r *jobRunner) logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job == nil {
		return
	}
	r.job.Log = append(r.job.Log, fmt.Sprintf(format, args...))
	if len(r.job.Log) > 400 {
		r.job.Log = r.job.Log[len(r.job.Log)-400:]
	}
}

// start runs fn in the background; only one job at a time.
func (r *jobRunner) start(action string, fn func(ctx context.Context) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job != nil && r.job.Running {
		return fmt.Errorf("%s läuft bereits", r.job.Action)
	}
	r.job = &Job{Action: action, Running: true, Started: time.Now().UTC().Format(time.RFC3339), Log: []string{}}
	started := time.Now()
	slog.Info("Mail-Job gestartet", "aktion", action)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		err := fn(ctx)
		r.mu.Lock()
		defer r.mu.Unlock()
		attrs := []any{"aktion", action, "dauer", time.Since(started).Round(time.Second)}
		if err != nil {
			tail := r.job.Log
			if len(tail) > 8 {
				tail = tail[len(tail)-8:]
			}
			slog.Error("Mail-Job fehlgeschlagen", append(attrs, "fehler", err.Error(), "letzte_zeilen", strings.Join(tail, " | "))...)
		} else {
			slog.Info("Mail-Job fertig", attrs...)
		}
		r.job.Running = false
		r.job.Finished = time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			r.job.Error = err.Error()
			r.job.Log = append(r.job.Log, "FEHLER: "+err.Error())
		} else {
			r.job.OK = true
			r.job.Log = append(r.job.Log, "Fertig.")
		}
	}()
	return nil
}

// run executes one step and logs its output.
func run(ctx context.Context, timeout time.Duration, name string, args ...string) error {
	jobs.logf("$ %s %s", name, strings.Join(args, " "))
	res := core.RunAdminTimeout(ctx, timeout, name, args...)
	for _, line := range strings.Split(strings.TrimSpace(res.Stdout+"\n"+res.Stderr), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			jobs.logf("  %s", line)
		}
	}
	if res.Err != nil {
		return fmt.Errorf("%s fehlgeschlagen", name)
	}
	return nil
}

var basePackages = []string{
	"postfix", "dovecot-core", "dovecot-imapd", "dovecot-lmtpd", "dovecot-sieve",
	"rspamd", "redis-server", "ssl-cert",
}

var clamavPackages = []string{"clamav-daemon", "clamav-freshclam"}

func aptInstall(ctx context.Context, pkgs []string) error {
	args := append([]string{"-q", "-y", "--no-install-recommends",
		"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold",
		"-o", "DPkg::Lock::Timeout=300", "install"}, pkgs...)
	return run(ctx, 30*time.Minute, "apt-get", args...)
}

// install sets up the whole server. Every step can run again (repair).
func install(ctx context.Context, s Settings) error {
	jobs.logf("==> Pakete installieren")
	// Postfix asks during installation; answer as "Internet Site".
	sel := fmt.Sprintf("postfix postfix/main_mailer_type select Internet Site\npostfix postfix/mailname string %s\n", s.Hostname)
	if res := core.RunAdminInput(ctx, sel, "debconf-set-selections"); res.Err != nil {
		return errors.New("debconf-set-selections: " + res.Detail())
	}
	if err := run(ctx, 5*time.Minute, "apt-get", "-q", "-o", "DPkg::Lock::Timeout=300", "update"); err != nil {
		return err
	}
	pkgs := append([]string(nil), basePackages...)
	if s.ClamAV {
		pkgs = append(pkgs, clamavPackages...)
	}
	if err := aptInstall(ctx, pkgs); err != nil {
		return err
	}

	jobs.logf("==> Konto vmail und /var/vmail")
	if _, err := userExists("vmail"); err != nil {
		if err := run(ctx, time.Minute, "adduser", "--system", "--group", "--home", vmailDir, "--no-create-home", "--disabled-login", "vmail"); err != nil {
			return err
		}
	}
	if err := run(ctx, time.Minute, "install", "-d", "-m", "0750", "-o", "vmail", "-g", "vmail", vmailDir); err != nil {
		return err
	}
	if err := run(ctx, time.Minute, "install", "-d", "-m", "0750", "-o", "_rspamd", "-g", "_rspamd", dkimDir); err != nil {
		return err
	}
	// Debian's dovecot.conf authenticates system users via PAM. Keep it as
	// a backup once; GoDeb writes its own.
	if _, err := os.Stat(root(dovecotConf) + ".godeb-orig"); errors.Is(err, os.ErrNotExist) {
		if data, err := os.ReadFile(root(dovecotConf)); err == nil && !strings.Contains(string(data), "Written by GoDeb") {
			_ = os.WriteFile(root(dovecotConf)+".godeb-orig", data, 0o644)
		}
	}

	jobs.logf("==> Konfiguration schreiben")
	if err := saveSettings(s); err != nil {
		return err
	}
	log, err := apply(ctx)
	for _, l := range log {
		jobs.logf("  %s", l)
	}
	if err != nil {
		return err
	}

	jobs.logf("==> Dienste starten")
	units := []string{"redis-server", "rspamd", "dovecot", "postfix"}
	if s.ClamAV {
		units = append([]string{"clamav-freshclam", "clamav-daemon"}, units...)
	}
	for _, u := range units {
		if err := run(ctx, 2*time.Minute, "systemctl", "enable", "--now", u); err != nil {
			return err
		}
	}

	jobs.logf("==> Firewall: Mail-Ports öffnen")
	if err := openPorts(ctx); err != nil {
		jobs.logf("  %s", err)
	}
	return nil
}

// openPorts allows 25, 465, 587 and 993 in ufw when ufw is active.
func openPorts(ctx context.Context) error {
	st := core.RunAdmin(ctx, "ufw", "status")
	if st.Err != nil || !strings.Contains(st.Stdout, "Status: active") {
		return errors.New("ufw ist nicht aktiv; die Ports 25, 465, 587 und 993/tcp bitte selbst freigeben")
	}
	for _, p := range mailPorts {
		if err := run(ctx, time.Minute, "ufw", "allow", p+"/tcp", "comment", "GoDeb-Mail"); err != nil {
			return err
		}
	}
	return nil
}

// enableClamAV installs and starts ClamAV, then switches Rspamd's
// antivirus module on (or the other way round).
func setClamAV(ctx context.Context, on bool) error {
	s, _, err := loadSettings()
	if err != nil {
		return err
	}
	if on {
		jobs.logf("==> ClamAV installieren (braucht etwa 1,5 GB RAM)")
		if err := aptInstall(ctx, clamavPackages); err != nil {
			return err
		}
		for _, u := range []string{"clamav-freshclam", "clamav-daemon"} {
			if err := run(ctx, 5*time.Minute, "systemctl", "enable", "--now", u); err != nil {
				return err
			}
		}
	}
	s.ClamAV = on
	if err := saveSettings(s); err != nil {
		return err
	}
	log, err := apply(ctx)
	for _, l := range log {
		jobs.logf("  %s", l)
	}
	if err != nil {
		return err
	}
	if !on {
		jobs.logf("==> ClamAV stoppen")
		for _, u := range []string{"clamav-daemon", "clamav-freshclam"} {
			_ = run(ctx, 2*time.Minute, "systemctl", "disable", "--now", u)
		}
	}
	return nil
}

// certificate requests a Let's Encrypt certificate for the hostname with
// certbot's own web server on port 80, which is open only while certbot
// runs (also on renewal, through the stored hooks).
func certificate(ctx context.Context, email string) error {
	s, _, err := loadSettings()
	if err != nil {
		return err
	}
	jobs.logf("==> Zertifikat für %s", s.Hostname)
	args := []string{"certonly", "--standalone", "--non-interactive", "--agree-tos",
		"--cert-name", s.Hostname, "-d", s.Hostname,
		"--pre-hook", "ufw allow 80/tcp comment GoDeb-ACME",
		"--post-hook", "ufw delete allow 80/tcp",
		"--deploy-hook", "systemctl reload postfix dovecot",
	}
	if email != "" {
		args = append(args, "-m", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}
	if err := run(ctx, 5*time.Minute, "certbot", args...); err != nil {
		return err
	}
	s.CertFile = "/etc/letsencrypt/live/" + s.Hostname + "/fullchain.pem"
	s.KeyFile = "/etc/letsencrypt/live/" + s.Hostname + "/privkey.pem"
	if err := saveSettings(s); err != nil {
		return err
	}
	log, err := apply(ctx)
	for _, l := range log {
		jobs.logf("  %s", l)
	}
	return err
}

func userExists(name string) (bool, error) {
	res := core.RunQuiet(context.Background(), "getent", "passwd", name)
	if res.Err != nil {
		return false, errors.New("fehlt")
	}
	return true, nil
}

// resolvesTo reports whether host has addr among its A/AAAA records.
func resolvesTo(ctx context.Context, host, addr string) bool {
	if addr == "" {
		return false
	}
	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if ip == addr {
			return true
		}
	}
	return false
}
