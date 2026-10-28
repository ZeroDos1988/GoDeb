package cron

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"godeb/backend/core"
)

const sampleTimers = `NEXT                        LEFT          LAST                        PASSED       UNIT                         ACTIVATES
Wed 2026-10-28 12:00:00 CEST 1h 0min left Wed 2026-10-28 11:00:00 CEST 1min ago     apt-daily.timer              apt-daily.service
Wed 2026-10-28 18:00:00 CEST 7h left      n/a                         n/a          godeb-cron-demo.timer        godeb-cron-demo.service

2 timers listed.
`

// listTimers runs systemctl list-timers --all.
func listTimers(ctx context.Context) ([]Timer, core.CommandEnvelope) {
	out, env := core.ReadCommand(ctx, sampleTimers, "systemctl", "list-timers", "--all", "--no-pager", "--no-legend")
	return parseTimers(out), env
}

func parseTimers(out string) []Timer {
	var list []Timer
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NEXT") || strings.Contains(line, "timers listed") {
			continue
		}
		fields := strings.Fields(line)
		// Columns: NEXT (often multi-token date), LEFT, LAST, PASSED, UNIT, ACTIVATES
		// systemctl without --legend still uses variable-width columns; find *.timer.
		unitIdx := -1
		for i, f := range fields {
			if strings.HasSuffix(f, ".timer") {
				unitIdx = i
				break
			}
		}
		if unitIdx < 0 || unitIdx+1 >= len(fields) {
			continue
		}
		unit := fields[unitIdx]
		activates := fields[unitIdx+1]
		t := Timer{Unit: unit, Activates: activates, Managed: isManagedTimer(unit)}
		// Best-effort: everything before unit is NEXT/LEFT/LAST/PASSED joined.
		if unitIdx >= 1 {
			t.Next = strings.Join(fields[:unitIdx], " ")
		}
		list = append(list, t)
	}
	if list == nil {
		list = []Timer{}
	}
	return list
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
