package domains

import (
	"errors"
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
	Name     string `json:"name"`
	Mode     string `json:"mode"`
	Defaults *bool  `json:"defaults"` // nil keeps the stored value (edit) or off (add)
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
	defaults := req.Defaults != nil && *req.Defaults
	if err := addDomain(name, mode, defaults); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "mode": mode, "defaults": defaults})
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
	cur, ok, err := getDomain(name)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		core.WriteError(w, http.StatusNotFound, "Domain nicht gefunden")
		return
	}
	defaults := cur.Defaults
	if req.Defaults != nil {
		defaults = *req.Defaults
	}
	if err := updateDomain(name, mode, defaults); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "mode": mode, "defaults": defaults})
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
	Domain Domain `json:"domain"`
	recordsView
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
	view, err := liveRecords(r, name)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, domainView{Domain: d, recordsView: view})
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
	view, err := liveRecords(r, name)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, view)
}

func handleApply(w http.ResponseWriter, r *http.Request) {
	name, err := NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	recs, err := Apply(r.Context(), name)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errNotFound) {
			status = http.StatusNotFound
		}
		core.WriteError(w, status, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"records": recs})
}

// recordsView is the live state of a zone plus why Hetzner could not be
// read (zone missing in the project, token without DNS rights, …).
type recordsView struct {
	Records      []Record `json:"records"`
	HetznerError string   `json:"hetznerError,omitempty"`
	ZoneMissing  bool     `json:"zoneMissing,omitempty"`
}

func liveRecords(r *http.Request, zone string) (recordsView, error) {
	needs, err := collectNeeds(zone)
	if err != nil {
		return recordsView{}, err
	}
	view := recordsView{Records: recordsFor(r.Context(), needs)}
	_, token, err := loadSettings()
	if err != nil || token == "" {
		return view, nil
	}
	d, ok, err := getDomain(zone)
	if err != nil || !ok || d.Mode != ModeHetzner {
		return view, nil
	}
	if view.Records, err = markManaged(r.Context(), token, zone, view.Records); err != nil {
		view.HetznerError = err.Error()
		view.ZoneMissing = errors.Is(err, errZoneMissing)
	}
	return view, nil
}
