package sites

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"godeb/backend/core"
	"godeb/plugins/domains"
	"godeb/plugins/mail"
	"godeb/plugins/wireguard"
)

func init() {
	core.Register("sites", Mount)
	core.RegisterService("sites", run)
	domains.RegisterNeeds(needs)
}

// Mount registers the routes. They run in the agent; creating a site needs
// password and a fresh code (auth.stepUpFor), the certificate route needs
// no session (see handleCert).
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plugins/sites", handleList)
	mux.HandleFunc("POST /api/plugins/sites", handleAdd)
	mux.HandleFunc("POST /api/plugins/sites/{name}/certificate", handleRenew)
	mux.HandleFunc("GET "+certPath, handleCert)
}

type listResponse struct {
	core.CommandEnvelope
	Hub          bool     `json:"hub"`
	Sites        []Site   `json:"sites"`
	MailDomains  []string `json:"mailDomains"`
	HetznerToken bool     `json:"hetznerToken"`
	Endpoint     string   `json:"endpoint,omitempty"`
}

func handleList(w http.ResponseWriter, r *http.Request) {
	resp := listResponse{Sites: []Site{}, MailDomains: []string{}}
	role, err := currentRole()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Hub = role.Role == wireguard.RoleHub
	if !resp.Hub {
		core.WriteJSON(w, http.StatusOK, resp)
		return
	}
	if list, err := List(); err == nil {
		resp.Sites = list
	} else {
		resp.Detail = err.Error()
	}
	if list, err := relayDomains(); err == nil && list != nil {
		resp.MailDomains = list
	}
	resp.HetznerToken = tokenSet()
	resp.Endpoint = wireguard.DefaultEndpoint()
	core.WriteJSON(w, http.StatusOK, resp)
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	var req AddRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "Ungültige Anfrage")
		return
	}
	res, err := Add(r.Context(), req)
	var reqErr *wireguard.RequestError
	switch {
	case errors.As(err, &reqErr):
		core.WriteError(w, reqErr.Status, reqErr.Msg)
	case err != nil:
		core.WriteError(w, http.StatusBadGateway, err.Error())
	default:
		w.Header().Set("Cache-Control", "no-store")
		core.WriteJSON(w, http.StatusCreated, res)
	}
}

// handleRenew orders the certificate of a site's mail host again.
func handleRenew(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	list, err := List()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, s := range list {
		if s.Name != name || s.Profile.MailHost == "" {
			continue
		}
		if !tokenSet() {
			core.WriteError(w, http.StatusConflict, "Ohne Hetzner-Token kann der Hub kein Zertifikat holen")
			return
		}
		if err := publish(r.Context(), mailHostNeed(s)); err != nil {
			core.WriteError(w, http.StatusBadGateway, "A-Eintrag: "+err.Error())
			return
		}
		startIssue(s.Profile.MailHost, s.Name)
		core.WriteJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
		return
	}
	core.WriteError(w, http.StatusNotFound, "Heim-Server mit Mail nicht gefunden")
}

// needs shows the A records of the mail hosts on the Domains page.
func needs() ([]domains.Need, error) {
	role, err := currentRole()
	if err != nil || role.Role != wireguard.RoleHub {
		return nil, err
	}
	list, err := List()
	if err != nil {
		return nil, nil
	}
	var out []domains.Need
	for _, s := range list {
		if s.Profile.MailHost != "" {
			out = append(out, mailHostNeed(s))
		}
	}
	return out, nil
}

// run renews the certificates on the hub and fetches the own one on a home
// server: a few minutes after the start, then every six hours. A home
// server without a certificate from the hub yet retries every 15 minutes.
func run(ctx context.Context) {
	wait := 2 * time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = 6 * time.Hour
		role, err := currentRole()
		switch {
		case err != nil:
		case role.Role == wireguard.RoleHub:
			renewAll(time.Now())
		case mailBehindHub() != "":
			c, cancel := context.WithTimeout(ctx, time.Minute)
			if err := FetchCertificate(c); err != nil {
				slog.Warn("Heim-Server: Zertifikat vom Hub nicht übernommen", "fehler", err)
				if !mail.HubCertificateInstalled() {
					wait = 15 * time.Minute
				}
			}
			cancel()
		}
	}
}
