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
	list := parseTimers(sampleTimers)
	if len(list) < 2 {
		t.Fatalf("%d", len(list))
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
