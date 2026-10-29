package cron

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"godeb/backend/core"
)

// sampleTimers is what systemd 257 (Debian 13) prints for
// "systemctl list-timers --all --output=json": times in microseconds since
// the epoch, 0 or null when a timer never ran or has no next run.
const sampleTimers = `[{"next":1793311200000000,"left":1793311200000000,"last":1793307600000000,"passed":60000000,"unit":"apt-daily.timer","activates":"apt-daily.service"},` +
	`{"next":1793332800000000,"left":1793332800000000,"last":null,"passed":null,"unit":"godeb-cron-demo.timer","activates":"godeb-cron-demo.service"}]`

// listTimers runs systemctl list-timers --all. The JSON form has fixed
// fields; the table form packs next run, time left and last run into
// columns of varying width.
func listTimers(ctx context.Context) ([]Timer, core.CommandEnvelope) {
	out, env := core.ReadCommand(ctx, sampleTimers, "systemctl", "list-timers", "--all", "--no-pager", "--output=json")
	list, err := parseTimers(out)
	if err != nil && !env.Mocked {
		env.Mocked, env.Detail = true, "systemctl list-timers: "+err.Error()
		list, _ = parseTimers(sampleTimers)
	}
	return list, env
}

type timerJSON struct {
	Next      *int64 `json:"next"`
	Last      *int64 `json:"last"`
	Unit      string `json:"unit"`
	Activates string `json:"activates"`
}

func parseTimers(out string) ([]Timer, error) {
	var raw []timerJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw); err != nil {
		return []Timer{}, err
	}
	list := make([]Timer, 0, len(raw))
	for _, r := range raw {
		if !strings.HasSuffix(r.Unit, ".timer") {
			continue
		}
		list = append(list, Timer{
			Unit: r.Unit, Activates: r.Activates, Managed: isManagedTimer(r.Unit),
			Next: microTime(r.Next), Last: microTime(r.Last),
		})
	}
	return list, nil
}

// microTime turns systemd's microseconds since the epoch into RFC 3339;
// "" for never.
func microTime(us *int64) string {
	if us == nil || *us <= 0 {
		return ""
	}
	return time.UnixMicro(*us).UTC().Format(time.RFC3339)
}

func listCronD(_ context.Context) ([]CronFile, bool) {
	dir := "/etc/cron.d"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []CronFile{{Name: "example", Preview: "# mocked\n0 5 * * * root /usr/bin/true\n"}}, true
	}
	var out []CronFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		preview := string(body)
		if len(preview) > 400 {
			preview = preview[:400] + "…"
		}
		out = append(out, CronFile{Name: name, Preview: preview})
	}
	if out == nil {
		out = []CronFile{}
	}
	return out, false
}
