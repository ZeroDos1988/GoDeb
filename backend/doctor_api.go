package main

import (
	"context"
	"net"
	"net/http"
	"time"

	"godeb/backend/agent"
	"godeb/backend/core"
	"godeb/backend/db"
	"godeb/backend/doctor"
	"godeb/backend/firstrun"
	"godeb/backend/metrics"
	"godeb/plugins/domains"
	"godeb/plugins/wireguard"
)

// doctorEnv is the host Env used by the CLI and the agent internal route.
func doctorEnv() doctor.Env {
	env := doctor.Default(version)
	env.DNS = func(ctx context.Context) ([]doctor.DNSRecord, error) {
		recs, err := domains.CheckAll(ctx)
		if err != nil {
			return nil, err
		}
		// Records of zones in Hetzner mode can be published with one click
		// (the same apply as on the Domains page; a conflict with a foreign
		// record stays a conflict there).
		hetzner := map[string]bool{}
		if zones, tokenSet, err := domains.HetznerZones(); err == nil && tokenSet {
			for _, z := range zones {
				hetzner[z] = true
			}
		}
		out := make([]doctor.DNSRecord, 0, len(recs))
		for _, r := range recs {
			rec := doctor.DNSRecord{Name: r.Name, Type: r.Type, Status: r.Status, Expected: r.ZoneLine, Found: r.Found}
			if hetzner[r.Zone] && r.Kind != "ptr" && r.Status != "conflict" {
				rec.Fix = &core.Fix{Action: "dnsApply", Path: "/api/plugins/domains/domains/" + r.Zone + "/apply"}
			}
			out = append(out, rec)
		}
		return out, nil
	}
	// The agent and the CLI (root) only check root's own files; the web
	// database belongs to the user godeb and is checked by the web process.
	env.Databases = []doctor.DBFile{
		{Name: "Agent-Datenbank", Path: db.AgentPath()},
		{Name: "Messwerte", Path: metrics.Path()},
	}
	env.CheckDB = db.QuickCheckFile
	return env
}

// webDatabaseFindings checks the web process's own database.
func webDatabaseFindings(ctx context.Context) []doctor.Finding {
	if db.Path() == db.AgentPath() {
		return nil // development: one file, already checked
	}
	env := doctor.Env{
		Databases: []doctor.DBFile{{Name: "Web-Datenbank", Path: db.Path()}},
		CheckDB:   db.QuickCheckFile,
	}
	return doctor.CheckOnly(ctx, env, doctor.CheckDatabases)
}

// runDoctorReport runs the read-only checks and returns the JSON report.
func runDoctorReport(ctx context.Context) doctor.Report {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return doctor.NewReport(doctor.Check(ctx, doctorEnv()), time.Now())
}

// runFullDoctorReport adds the report head and the names a redacted report
// hides. Runs in the agent (root) or the CLI; same code for both.
func runFullDoctorReport(ctx context.Context) doctor.FullReport {
	env := doctorEnv()
	full := doctor.FullReport{Report: runDoctorReport(ctx), Meta: doctor.CollectMeta(ctx, env)}
	full.Meta.Setup = "abgeschlossen"
	if firstrun.Required() {
		full.Meta.Setup = "offen"
	}
	full.Sensitive = append(full.Sensitive, full.Meta.Hostname, full.Meta.FQDN)
	if role, err := wireguard.CurrentRole(); err == nil {
		full.Meta.Role = string(role.Role)
		if host, _, err := net.SplitHostPort(role.Hub); err == nil {
			full.Sensitive = append(full.Sensitive, host)
		}
	}
	full.Sensitive = append(full.Sensitive, domains.Names()...)
	return full
}

// doctorText is the response of GET /api/doctor/report: the full text
// (redacted on request) and the short summary for a GitHub issue, which is
// always redacted because the repository is public.
type doctorText struct {
	Text     string `json:"text"`
	Summary  string `json:"summary"`
	Version  string `json:"version"`
	Redacted bool   `json:"redacted"`
	Fails    int    `json:"fails"`
	Warns    int    `json:"warns"`
}

func renderDoctorText(full doctor.FullReport, redact bool, at time.Time) doctorText {
	text := doctor.Text(full.Meta, full.Report, at)
	if redact {
		text = doctor.Redact(text, full.Sensitive)
	}
	return doctorText{
		Text:     text,
		Summary:  doctor.Redact(doctor.Summary(full.Meta, full.Report), full.Sensitive),
		Version:  full.Meta.Version,
		Redacted: redact,
		Fails:    full.Report.Fails,
		Warns:    full.Report.Warns,
	}
}

// mountDoctorAPI exposes GET /api/doctor for the overview. With an agent the
// web process only forwards; without one it runs the checks itself (dev).
func mountDoctorAPI(mux *http.ServeMux, client *agent.Client) {
	mux.HandleFunc("GET /api/doctor", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if client != nil {
			var report doctor.Report
			if err := client.Call(r.Context(), http.MethodGet, "/internal/doctor", nil, &report); err != nil {
				core.WriteError(w, http.StatusBadGateway, err.Error())
				return
			}
			report.Add(webDatabaseFindings(r.Context()))
			core.WriteJSON(w, http.StatusOK, report)
			return
		}
		core.WriteJSON(w, http.StatusOK, runDoctorReport(r.Context()))
	})
	// The diagnosis report as text for copying, downloading or a GitHub
	// issue. Read-only; nothing leaves the server unless the admin sends it.
	mux.HandleFunc("GET /api/doctor/report", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		redact := r.URL.Query().Get("redact") == "1"
		var full doctor.FullReport
		if client != nil {
			if err := client.Call(r.Context(), http.MethodGet, "/internal/doctor/full", nil, &full); err != nil {
				core.WriteError(w, http.StatusBadGateway, err.Error())
				return
			}
			full.Report.Add(webDatabaseFindings(r.Context()))
		} else {
			full = runFullDoctorReport(r.Context())
		}
		core.WriteJSON(w, http.StatusOK, renderDoctorText(full, redact, time.Now()))
	})
}
