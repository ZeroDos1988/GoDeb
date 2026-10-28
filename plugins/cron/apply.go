package cron

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"godeb/backend/core"
)

func renderService(req JobRequest) string {
	desc := req.Description
	if desc == "" {
		desc = "GoDeb scheduled job " + req.Name
	}
	exec := strings.Join(shellQuote(req.Command), " ")
	return fmt.Sprintf(`[Unit]
Description=%s
# Written by GoDeb (plugin cron).

[Service]
Type=oneshot
ExecStart=%s
Nice=10
`, escapeUnit(desc), exec)
}

func renderTimer(req JobRequest) string {
	desc := req.Description
	if desc == "" {
		desc = "GoDeb timer " + req.Name
	}
	return fmt.Sprintf(`[Unit]
Description=%s
# Written by GoDeb (plugin cron).

[Timer]
OnCalendar=%s
Persistent=true
Unit=%s

[Install]
WantedBy=timers.target
`, escapeUnit(desc), req.Calendar, serviceName(req.Name))
}

func escapeUnit(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// shellQuote is for systemd ExecStart lines (no shell): quote if needed.
func shellQuote(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"'\\") {
			out[i] = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		} else {
			out[i] = a
		}
	}
	return out
}

func writeJob(ctx context.Context, req JobRequest) error {
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return err
	}
	svc := renderService(req)
	tim := renderTimer(req)
	if err := atomicWrite(servicePath(req.Name), svc, 0o644); err != nil {
		return err
	}
	if err := atomicWrite(timerPath(req.Name), tim, 0o644); err != nil {
		return err
	}
	if res := core.RunAdmin(ctx, "systemctl", "daemon-reload"); res.Err != nil {
		return fmt.Errorf("daemon-reload: %s", res.Detail())
	}
	if res := core.RunAdminTimeout(ctx, time.Minute, "systemctl", "enable", "--now", unitName(req.Name)); res.Err != nil {
		return fmt.Errorf("enable: %s", res.Detail())
	}
	return nil
}

func deleteJob(ctx context.Context, name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if !namePattern.MatchString(name) {
		return fmt.Errorf("Name ist ungültig")
	}
	_ = core.RunAdmin(ctx, "systemctl", "disable", "--now", unitName(name))
	_ = os.Remove(timerPath(name))
	_ = os.Remove(servicePath(name))
	if res := core.RunAdmin(ctx, "systemctl", "daemon-reload"); res.Err != nil {
		return fmt.Errorf("daemon-reload: %s", res.Detail())
	}
	return nil
}

func atomicWrite(path, body string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".godeb-cron-*")
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
	return os.Rename(tmp.Name(), path)
}
