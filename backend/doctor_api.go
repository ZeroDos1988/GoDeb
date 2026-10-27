package main

import (
	"context"
	"net/http"
	"time"

	"godeb/backend/agent"
	"godeb/backend/core"
	"godeb/backend/doctor"
	"godeb/plugins/domains"
)

// doctorEnv is the host Env used by the CLI and the agent internal route.
func doctorEnv() doctor.Env {
	env := doctor.Default(version)
	env.DNS = func(ctx context.Context) ([]doctor.DNSRecord, error) {
		recs, err := domains.CheckAll(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]doctor.DNSRecord, 0, len(recs))
		for _, r := range recs {
			out = append(out, doctor.DNSRecord{Name: r.Name, Type: r.Type, Status: r.Status, Expected: r.ZoneLine, Found: r.Found})
		}
		return out, nil
	}
	return env
}

// runDoctorReport runs the read-only checks and returns the JSON report.
func runDoctorReport(ctx context.Context) doctor.Report {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return doctor.NewReport(doctor.Check(ctx, doctorEnv()), time.Now())
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
			core.WriteJSON(w, http.StatusOK, report)
			return
		}
		core.WriteJSON(w, http.StatusOK, runDoctorReport(r.Context()))
	})
}
