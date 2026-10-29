package cron

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Timer is one systemd timer (system-wide).
type Timer struct {
	Unit        string `json:"unit"`
	Next        string `json:"next"`
	Left        string `json:"left"`
	Last        string `json:"last"`
	Passed      string `json:"passed"`
	Activates   string `json:"activates"`
	Managed     bool   `json:"managed"` // written by GoDeb under godeb-cron-*.timer
	Description string `json:"description,omitempty"`
}

// CronFile is one file under /etc/cron.d (read-only in the UI).
type CronFile struct {
	Name    string `json:"name"`
	Preview string `json:"preview"`
}

// JobRequest creates a GoDeb-managed timer.
type JobRequest struct {
	Name        string   `json:"name"`
	Calendar    string   `json:"calendar"` // systemd OnCalendar=
	Command     []string `json:"command"`  // argv, no shell
	Description string   `json:"description"`
}

const (
	unitPrefix = "godeb-cron-"
	unitDir    = "/etc/systemd/system"
)

var (
	namePattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	calendarPattern = regexp.MustCompile(`^[A-Za-z0-9*][A-Za-z0-9 *,/:.-]{0,79}$`)
	argPattern      = regexp.MustCompile(`^[A-Za-z0-9._/=+:@%,-]+$`)
	scriptPattern   = regexp.MustCompile(`^/(?:usr/local/sbin|etc/godeb/scripts)/[A-Za-z0-9._+-]+$`)
)

// Fixed binaries that may be scheduled (no shell, argv only). Each one has
// its own argument check in validateCommand: certbot and logrotate can run
// arbitrary programs through hooks or config files, so only fixed argument
// lists pass.
var allowedBins = map[string]bool{
	"/bin/true":           true,
	"/usr/bin/true":       true,
	"/usr/sbin/logrotate": true,
	"/usr/bin/certbot":    true,
	"/usr/bin/systemctl":  true,
	"/bin/systemctl":      true,
}

var logrotateConfig = regexp.MustCompile(`^/etc/logrotate\.(?:conf|d/[A-Za-z0-9._-]+)$`)

func unitName(name string) string    { return unitPrefix + name + ".timer" }
func serviceName(name string) string { return unitPrefix + name + ".service" }
func timerPath(name string) string   { return filepath.Join(unitDir, unitName(name)) }
func servicePath(name string) string { return filepath.Join(unitDir, serviceName(name)) }

func cleanJob(req JobRequest) (JobRequest, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !namePattern.MatchString(name) {
		return req, errors.New("Name: Kleinbuchstaben, Ziffern und Bindestrich (max. 32)")
	}
	cal := strings.TrimSpace(req.Calendar)
	if !calendarPattern.MatchString(cal) || strings.Contains(cal, "..") {
		return req, errors.New("Zeitplan (OnCalendar) ist ungültig")
	}
	if len(req.Command) == 0 || len(req.Command) > 8 {
		return req, errors.New("Befehl: 1–8 Argumente, kein Shell")
	}
	cmd := make([]string, 0, len(req.Command))
	for _, a := range req.Command {
		a = strings.TrimSpace(a)
		if a == "" || !argPattern.MatchString(a) || strings.Contains(a, "..") {
			return req, fmt.Errorf("Argument %q ist ungültig", a)
		}
		cmd = append(cmd, a)
	}
	if err := validateCommand(cmd); err != nil {
		return req, err
	}
	desc := strings.TrimSpace(req.Description)
	if len(desc) > 120 {
		return req, errors.New("Beschreibung maximal 120 Zeichen")
	}
	for _, r := range desc {
		if r < 32 {
			return req, errors.New("Beschreibung enthält Steuerzeichen")
		}
	}
	return JobRequest{Name: name, Calendar: cal, Command: cmd, Description: desc}, nil
}

func validateCommand(cmd []string) error {
	bin, args := cmd[0], cmd[1:]
	if allowedBins[bin] {
		switch bin {
		case "/usr/bin/systemctl", "/bin/systemctl":
			return validateSystemctl(args)
		case "/usr/bin/certbot":
			return validateCertbot(args)
		case "/usr/sbin/logrotate":
			return validateLogrotate(args)
		}
		return nil
	}
	if scriptPattern.MatchString(bin) {
		return nil
	}
	return errors.New("Befehl nicht erlaubt: Allowlist oder /usr/local/sbin/… bzw. /etc/godeb/scripts/…")
}

// validateCertbot allows "renew" plus harmless flags; --deploy-hook and
// friends would run any file as root.
func validateCertbot(args []string) error {
	if len(args) == 0 || args[0] != "renew" {
		return errors.New("certbot nur mit „renew“")
	}
	for _, a := range args[1:] {
		switch a {
		case "--quiet", "-q", "--no-random-sleep-on-renew":
		default:
			return fmt.Errorf("certbot renew: Option %q nicht erlaubt", a)
		}
	}
	return nil
}

// validateLogrotate allows only root-owned configs below /etc: a config
// file can run postrotate scripts as root.
func validateLogrotate(args []string) error {
	if len(args) > 0 && (args[0] == "-f" || args[0] == "--force") {
		args = args[1:]
	}
	if len(args) != 1 || !logrotateConfig.MatchString(args[0]) {
		return errors.New("logrotate nur mit /etc/logrotate.conf oder /etc/logrotate.d/<datei> (optional -f)")
	}
	return nil
}

func validateSystemctl(args []string) error {
	if len(args) < 2 {
		return errors.New("systemctl braucht Aktion und Unit")
	}
	switch args[0] {
	case "reload", "try-reload-or-restart", "reload-or-restart":
	default:
		return errors.New("systemctl: nur reload / reload-or-restart / try-reload-or-restart")
	}
	unit := args[1]
	if !regexp.MustCompile(`^[a-zA-Z0-9@._-]{1,64}\.(service|timer|socket)$`).MatchString(unit) {
		return errors.New("systemctl-Unit ist ungültig")
	}
	if len(args) > 2 {
		return errors.New("systemctl: keine weiteren Argumente")
	}
	return nil
}

func isManagedTimer(unit string) bool {
	return strings.HasPrefix(unit, unitPrefix) && strings.HasSuffix(unit, ".timer")
}
