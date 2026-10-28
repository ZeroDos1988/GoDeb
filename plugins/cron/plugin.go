package cron

import (
	"log/slog"
	"net/http"
	"strings"

	"godeb/backend/core"
)

func init() {
	core.Register("cron", Mount)
}

const base = "/api/plugins/cron"

// Mount registers the scheduled-tasks routes.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+base+"/status", handleStatus)
	mux.HandleFunc("POST "+base+"/jobs", handleAddJob)
	mux.HandleFunc("POST "+base+"/jobs/{name}/delete", handleDeleteJob)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	timers, env := listTimers(r.Context())
	cronFiles, cronMocked := listCronD(r.Context())
	if cronMocked {
		env.Mocked = true
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"command":   env.Command,
		"mocked":    env.Mocked,
		"detail":    env.Detail,
		"timers":    timers,
		"cronFiles": cronFiles,
		"allowlist": allowlistInfo(),
	})
}

func allowlistInfo() map[string]any {
	bins := make([]string, 0, len(allowedBins))
	for b := range allowedBins {
		bins = append(bins, b)
	}
	return map[string]any{
		"binaries": bins,
		"scriptDirs": []string{"/usr/local/sbin", "/etc/godeb/scripts"},
		"calendarHint": "daily, weekly, *-*-* 03:00:00, Mon *-*-* 06:00:00",
	}
}

func handleAddJob(w http.ResponseWriter, r *http.Request) {
	var req JobRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	job, err := cleanJob(req)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := writeJob(r.Context(), job); err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "Timer angelegt", "name", job.Name, "calendar", job.Calendar)
	core.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "unit": unitName(job.Name)})
}

func handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	if !namePattern.MatchString(name) {
		core.WriteError(w, http.StatusBadRequest, "Name ist ungültig")
		return
	}
	if err := deleteJob(r.Context(), name); err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "Timer gelöscht", "name", name)
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
