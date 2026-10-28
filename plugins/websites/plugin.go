package websites

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"godeb/backend/core"
	"godeb/plugins/domains"
)

func init() {
	core.Register("websites", Mount)
	domains.RegisterNeeds(dnsNeeds)
}

const base = "/api/plugins/websites"

// Mount registers the websites routes.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+base+"/status", handleStatus)
	mux.HandleFunc("POST "+base+"/sites", handleAdd)
	mux.HandleFunc("POST "+base+"/sites/{name}", handleEdit)
	mux.HandleFunc("POST "+base+"/sites/{name}/delete", handleDelete)
	mux.HandleFunc("POST "+base+"/sites/{name}/certificate", handleCert)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	sites, err := loadSites()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nginx := strings.TrimSpace(core.RunQuiet(r.Context(), "systemctl", "is-active", "nginx").Stdout)
	if nginx == "" {
		nginx = "unknown"
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"sites":  sites,
		"nginx":  nginx,
		"wwwRoot": wwwRoot,
		"mocked": nginx == "unknown" || nginx == "inactive" && len(sites) == 0,
	})
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	var req Site
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	req.Created = time.Now().UTC().Format(time.RFC3339)
	req.Enabled = true
	s, err := cleanSite(req)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := findSite(s.Name); err == nil {
		core.WriteError(w, http.StatusConflict, s.Name+" gibt es schon")
		return
	}
	if err := saveSite(s); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := applySite(r.Context(), s); err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "Website angelegt", "name", s.Name, "domain", s.Domain)
	core.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "site": s})
}

func handleEdit(w http.ResponseWriter, r *http.Request) {
	old, err := findSite(r.PathValue("name"))
	if err != nil {
		core.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	var req Site
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	req.Name = old.Name
	req.Created = old.Created
	s, err := cleanSite(req)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := saveSite(s); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := applySite(r.Context(), s); err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "Website geändert", "name", s.Name)
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "site": s})
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	s, err := findSite(r.PathValue("name"))
	if err != nil {
		core.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	_ = removeSite(r.Context(), s)
	if err := deleteSiteRow(s.Name); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "Website gelöscht", "name", s.Name)
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleCert(w http.ResponseWriter, r *http.Request) {
	s, err := findSite(r.PathValue("name"))
	if err != nil {
		core.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	if err := issueCert(r.Context(), s.Domain, strings.TrimSpace(req.Email)); err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.TLS = true
	s.Public = true
	_ = saveSite(s)
	if err := applySite(r.Context(), s); err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "site": s})
}

func dnsNeeds() ([]domains.Need, error) {
	sites, err := loadSites()
	if err != nil {
		return nil, err
	}
	ip := strings.TrimSpace(core.DefaultRoute(context.Background()).Src)
	var out []domains.Need
	seen := map[string]bool{}
	for _, s := range sites {
		if !s.Enabled || s.Domain == "" {
			continue
		}
		key := s.Domain + "|A"
		if seen[key] || ip == "" {
			continue
		}
		seen[key] = true
		out = append(out, domains.Need{
			Zone: s.Domain, Name: s.Domain, Type: "A", Value: ip, Kind: "a", Source: "websites",
		})
	}
	return out, nil
}
