package domains

import (
	"net/http"
	"strings"

	"godeb/backend/core"
)

func init() {
	core.Register("domains", Mount)
}

const base = "/api/plugins/domains"

// Mount registers the domains routes.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+base+"/status", handleStatus)
	mux.HandleFunc("POST "+base+"/token", handleToken)
	mux.HandleFunc("POST "+base+"/domains", handleAddDomain)
	mux.HandleFunc("GET "+base+"/domains/{domain}", handleDomain)
	mux.HandleFunc("POST "+base+"/domains/{domain}", handleEditDomain)
	mux.HandleFunc("POST "+base+"/domains/{domain}/delete", handleDeleteDomain)
	mux.HandleFunc("GET "+base+"/domains/{domain}/records", handleRecords)
	mux.HandleFunc("POST "+base+"/domains/{domain}/apply", handleApply)
}

type statusResponse struct {
	Settings Settings `json:"settings"`
	Domains  []Domain `json:"domains"`
	PublicIP string   `json:"publicIp"`
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	s, _, err := loadSettings()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	domains, err := listDomains()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if domains == nil {
		domains = []Domain{}
	}
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, statusResponse{
		Settings: s,
		Domains:  domains,
		PublicIP: core.DefaultRoute(r.Context()).Src,
	})
}

type tokenRequest struct {
	Token    string `json:"token"`
	Clear    bool   `json:"clear"`
	ServerID int64  `json:"serverId"`
}

func handleToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	if req.Clear {
		empty := ""
		if err := upsertSettings(&empty, nil); err != nil {
			core.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	token := strings.TrimSpace(req.Token)
	if token != "" {
		if !tokenOK(token) {
			core.WriteError(w, http.StatusBadRequest, "Token ist ungültig")
			return
		}
		if err := validateToken(r.Context(), token); err != nil {
			core.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := upsertSettings(&token, nil); err != nil {
			core.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.ServerID > 0 {
		id := req.ServerID
		if err := upsertSettings(nil, &id); err != nil {
			core.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if token == "" && req.ServerID == 0 {
		core.WriteError(w, http.StatusBadRequest, "Token oder Server-ID erforderlich")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type domainRequest struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

func handleAddDomain(w http.ResponseWriter, r *http.Request) {
	var req domainRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	name, err := NormalizeDomain(req.Name)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	mode, err := normalizeMode(req.Mode)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok, err := getDomain(name); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	} else if ok {
		core.WriteError(w, http.StatusConflict, "Domain ist schon eingetragen")
		return
	}
	if err := addDomain(name, mode); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "mode": mode})
}

func handleEditDomain(w http.ResponseWriter, r *http.Request) {
	name, err := NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req domainRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	mode, err := normalizeMode(req.Mode)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok, err := getDomain(name); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		core.WriteError(w, http.StatusNotFound, "Domain nicht gefunden")
		return
	}
	if err := updateDomain(name, mode); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "mode": mode})
}

func handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	name, err := NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := deleteDomain(name); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type domainView struct {
	Domain  Domain   `json:"domain"`
	Records []Record `json:"records"`
}

func handleDomain(w http.ResponseWriter, r *http.Request) {
	name, err := NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	d, ok, err := getDomain(name)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		core.WriteError(w, http.StatusNotFound, "Domain nicht gefunden")
		return
	}
	recs, err := liveRecords(r, name)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, domainView{Domain: d, Records: recs})
}

func handleRecords(w http.ResponseWriter, r *http.Request) {
	name, err := NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok, err := getDomain(name); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		core.WriteError(w, http.StatusNotFound, "Domain nicht gefunden")
		return
	}
	recs, err := liveRecords(r, name)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, map[string]any{"records": recs})
}

func handleApply(w http.ResponseWriter, r *http.Request) {
	name, err := NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	d, ok, err := getDomain(name)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		core.WriteError(w, http.StatusNotFound, "Domain nicht gefunden")
		return
	}
	if d.Mode != ModeHetzner {
		core.WriteError(w, http.StatusBadRequest, "Domain ist nicht auf Hetzner gestellt")
		return
	}
	s, token, err := loadSettings()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if token == "" {
		core.WriteError(w, http.StatusBadRequest, "Kein Hetzner-API-Token hinterlegt")
		return
	}
	needs, err := collectNeeds(name)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(needs) == 0 {
		core.WriteError(w, http.StatusBadRequest, "Keine Einträge von Plugins für diese Domain")
		return
	}
	recs, err := applyZone(r.Context(), token, name, needs, s.ServerID)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"records": recs})
}

func liveRecords(r *http.Request, zone string) ([]Record, error) {
	needs, err := collectNeeds(zone)
	if err != nil {
		return nil, err
	}
	recs := recordsFor(r.Context(), needs)
	_, token, err := loadSettings()
	if err != nil || token == "" {
		return recs, nil
	}
	d, ok, err := getDomain(zone)
	if err != nil || !ok || d.Mode != ModeHetzner {
		return recs, nil
	}
	return markManaged(r.Context(), token, zone, recs), nil
}
