package cron

import (
	"regexp"
	"strings"
	"testing"
)

// unitLine is every line GoDeb writes into a timer or service unit.
var unitLine = regexp.MustCompile(`^(?:\[(?:Unit|Service|Timer|Install)\]|# Written by GoDeb \(plugin cron\)\.|` +
	`Description=[^\n]*|Type=oneshot|Nice=10|Persistent=true|WantedBy=timers\.target|` +
	`OnCalendar=[A-Za-z0-9 *,/:.-]+|Unit=godeb-cron-[a-z0-9-]+\.service|ExecStart=/[^\n]*)$`)

func FuzzJobUnits(f *testing.F) {
	f.Add("backup", "daily", "/usr/local/sbin/backup.sh", "--all", "Nächtliche Sicherung")
	f.Add("x", "daily\nExecStartPre=/bin/sh", "/bin/true", "", "")
	f.Add("x", "daily", "/usr/bin/systemctl", "reload", "%h\\n; rm")
	f.Add("x", "*-*-* 03:00:00", "/usr/sbin/logrotate", "/etc/logrotate.conf", "a\"b")
	f.Fuzz(func(t *testing.T, name, calendar, bin, arg, desc string) {
		cmd := []string{bin}
		if arg != "" {
			cmd = append(cmd, arg)
		}
		if bin == "/usr/bin/systemctl" {
			cmd = append(cmd, "nginx.service")
		}
		job, err := cleanJob(JobRequest{Name: name, Calendar: calendar, Command: cmd, Description: desc})
		if err != nil {
			return
		}
		for _, unit := range []string{renderService(job), renderTimer(job)} {
			for _, line := range strings.Split(strings.TrimSpace(unit), "\n") {
				if line == "" {
					continue
				}
				if !unitLine.MatchString(line) {
					t.Fatalf("unexpected unit line %q for %+v", line, job)
				}
				// A single % starts a systemd specifier (%h, %u, ...).
				if strings.Contains(strings.ReplaceAll(line, "%%", ""), "%") {
					t.Fatalf("unescaped %% in %q", line)
				}
			}
		}
		exec := renderService(job)
		if strings.Count(exec, "\nExecStart=") != 1 {
			t.Fatalf("not exactly one ExecStart:\n%s", exec)
		}
		start := exec[strings.Index(exec, "\nExecStart=")+len("\nExecStart="):]
		start = start[:strings.Index(start, "\n")]
		if !strings.HasPrefix(start, job.Command[0]+" ") && start != job.Command[0] {
			t.Fatalf("ExecStart %q does not start with %q", start, job.Command[0])
		}
	})
}
