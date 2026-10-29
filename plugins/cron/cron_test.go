package cron

import (
	"strings"
	"testing"
)

func TestCleanJob(t *testing.T) {
	j, err := cleanJob(JobRequest{
		Name: "Nightly-Log", Calendar: "*-*-* 03:15:00",
		Command: []string{"/usr/sbin/logrotate", "/etc/logrotate.conf"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if j.Name != "nightly-log" {
		t.Fatalf("%q", j.Name)
	}
	if _, err := cleanJob(JobRequest{Name: "x", Calendar: "daily", Command: []string{"/bin/sh", "-c", "rm"}}); err == nil {
		t.Fatal("shell rejected")
	}
	if _, err := cleanJob(JobRequest{Name: "x", Calendar: "daily", Command: []string{"/usr/bin/systemctl", "start", "ssh.service"}}); err == nil {
		t.Fatal("systemctl start rejected")
	}
	if _, err := cleanJob(JobRequest{
		Name: "ok", Calendar: "daily",
		Command: []string{"/usr/bin/systemctl", "reload", "nginx.service"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanJob(JobRequest{
		Name: "script", Calendar: "weekly",
		Command: []string{"/etc/godeb/scripts/backup"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestParseTimers(t *testing.T) {
	list, err := parseTimers(sampleTimers)
	if err != nil || len(list) != 2 {
		t.Fatalf("%d %v", len(list), err)
	}
	if list[0].Next != "2026-10-29T22:00:00Z" || list[0].Last != "2026-10-29T21:00:00Z" || list[1].Last != "" {
		t.Fatalf("times: %+v", list)
	}
	if _, err := parseTimers("NEXT LEFT LAST"); err == nil {
		t.Fatal("table output must be rejected, not misparsed")
	}
	var managed bool
	for _, tmr := range list {
		if tmr.Managed && strings.Contains(tmr.Unit, "godeb-cron-demo") {
			managed = true
		}
	}
	if !managed {
		t.Fatalf("%+v", list)
	}
}

func TestRenderUnits(t *testing.T) {
	req := JobRequest{Name: "demo", Calendar: "daily", Command: []string{"/usr/bin/true"}, Description: "Demo"}
	svc := renderService(req)
	tim := renderTimer(req)
	for _, want := range []string{"ExecStart=/usr/bin/true", "Type=oneshot"} {
		if !strings.Contains(svc, want) {
			t.Errorf("service lacks %q:\n%s", want, svc)
		}
	}
	for _, want := range []string{"OnCalendar=daily", "Unit=godeb-cron-demo.service", "WantedBy=timers.target"} {
		if !strings.Contains(tim, want) {
			t.Errorf("timer lacks %q:\n%s", want, tim)
		}
	}
}

func TestCleanJobHooks(t *testing.T) {
	for _, cmd := range [][]string{
		{"/usr/bin/certbot", "renew", "--deploy-hook=/tmp/x"},
		{"/usr/bin/certbot", "renew", "--post-hook", "/tmp/x"},
		{"/usr/bin/certbot", "certonly"},
		{"/usr/sbin/logrotate", "/tmp/evil.conf"},
		{"/usr/sbin/logrotate", "-f", "/var/lib/godeb/x.conf"},
		{"/usr/sbin/logrotate", "/etc/logrotate.conf", "/tmp/evil.conf"},
		{"/usr/sbin/service", "ssh", "stop"},
	} {
		if _, err := cleanJob(JobRequest{Name: "x", Calendar: "daily", Command: cmd}); err == nil {
			t.Errorf("accepted %v", cmd)
		}
	}
	for _, cmd := range [][]string{
		{"/usr/bin/certbot", "renew", "--quiet"},
		{"/usr/sbin/logrotate", "-f", "/etc/logrotate.d/nginx"},
	} {
		if _, err := cleanJob(JobRequest{Name: "x", Calendar: "daily", Command: cmd}); err != nil {
			t.Errorf("rejected %v: %v", cmd, err)
		}
	}
}

func TestRenderEscapesSpecifiers(t *testing.T) {
	svc := renderService(JobRequest{Name: "d", Calendar: "daily", Command: []string{"/etc/godeb/scripts/run", "50%h"}, Description: "100% %u"})
	if !strings.Contains(svc, "ExecStart=/etc/godeb/scripts/run 50%%h") || !strings.Contains(svc, "Description=100%% %%u") {
		t.Fatalf("%s", svc)
	}
}
