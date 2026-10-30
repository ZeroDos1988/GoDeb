package apphttps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"godeb/backend/core"
	"godeb/backend/metrics"
	"godeb/plugins/domains"
)

const base = "/api/plugins/apphttps"

// renewBefore: Let's Encrypt certificates last 90 days; renew in the last 30.
const renewBefore = 30 * 24 * time.Hour

func init() {
	core.Register("apphttps", Mount)
	core.RegisterService("apphttps", run)
	domains.RegisterNeeds(needs)
	metrics.RegisterCheck("apphttps", expiryCheck)
}

// Mount registers the routes. They run in the agent.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+base+"/status", handleStatus)
	mux.HandleFunc("POST "+base+"/enable", handleEnable)
	mux.HandleFunc("POST "+base+"/renew", handleRenew)
	mux.HandleFunc("POST "+base+"/disable", handleDisable)
}

type statusResponse struct {
	Enabled      bool     `json:"enabled"`
	Label        string   `json:"label"`
	Zone         string   `json:"zone"`
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	URL          string   `json:"url,omitempty"`
	Address      string   `json:"address,omitempty"`
	AddressError string   `json:"addressError,omitempty"`
	Zones        []string `json:"zones"`
	TokenSet     bool     `json:"tokenSet"`
	NotAfter     string   `json:"notAfter,omitempty"`
	LastError    string   `json:"lastError,omitempty"`
	LastRun      string   `json:"lastRun,omitempty"`
	Running      bool     `json:"running"`
}

func status() (statusResponse, error) {
	row, err := load()
	if err != nil {
		return statusResponse{}, err
	}
	zones, tokenSet, err := domains.HetznerZones()
	if err != nil {
		return statusResponse{}, err
	}
	s := statusResponse{
		Enabled: row.Enabled, Label: row.Label, Zone: row.Zone, Host: row.Host, Port: Port,
		Zones: zones, TokenSet: tokenSet, LastError: row.LastError, LastRun: row.LastRun,
		Running: running.Load(),
	}
	if row.CertPEM != "" {
		s.NotAfter = row.NotAfter
	}
	if row.Enabled && row.CertPEM != "" {
		s.URL = "https://" + row.Host + ":" + strconv.Itoa(Port)
	}
	if s.Address, err = tunnelIPv4(); err != nil {
		s.AddressError = err.Error()
	}
	return s, nil
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	s, err := status()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, s)
}

type enableRequest struct {
	Label string `json:"label"`
	Zone  string `json:"zone"`
}

func handleEnable(w http.ResponseWriter, r *http.Request) {
	var req enableRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	zones, tokenSet, err := domains.HetznerZones()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !tokenSet {
		core.WriteError(w, http.StatusBadRequest, "Kein Hetzner-API-Token hinterlegt (Domains)")
		return
	}
	host, err := hostFor(req.Label, req.Zone, zones)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := tunnelIPv4(); err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if running.Load() {
		core.WriteError(w, http.StatusConflict, "Zertifikat wird gerade geholt")
		return
	}
	row, err := update(func(row *settingsRow) {
		if row.Host != host {
			row.CertPEM, row.CertKey, row.NotAfter = "", "", ""
		}
		row.Label, row.Zone, row.Host, row.Enabled = req.Label, req.Zone, host, true
		row.LastError = ""
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if needsRenewal(row, time.Now()) {
		startIssue()
	} else if err := activate(r.Context(), row); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handleStatus(w, r)
}

func handleRenew(w http.ResponseWriter, r *http.Request) {
	row, err := load()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !row.Enabled {
		core.WriteError(w, http.StatusBadRequest, "App-Zugang ist aus")
		return
	}
	if !startIssue() {
		core.WriteError(w, http.StatusConflict, "Zertifikat wird gerade geholt")
		return
	}
	handleStatus(w, r)
}

func handleDisable(w http.ResponseWriter, r *http.Request) {
	if running.Load() {
		core.WriteError(w, http.StatusConflict, "Zertifikat wird gerade geholt")
		return
	}
	row, err := update(func(row *settingsRow) { row.Enabled = false })
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeFiles(Listen{Host: row.Host, Port: Port}, nil, nil); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	firewall(r.Context(), false)
	handleStatus(w, r)
}

func needsRenewal(row settingsRow, now time.Time) bool {
	if row.CertPEM == "" {
		return true
	}
	end, err := time.Parse(time.RFC3339, row.NotAfter)
	return err != nil || end.Sub(now) < renewBefore
}

// activate hands the stored certificate to the web process and opens the
// port on wg0.
func activate(ctx context.Context, row settingsRow) error {
	if err := writeFiles(Listen{Enabled: true, Host: row.Host, Port: Port}, []byte(row.CertPEM), []byte(row.CertKey)); err != nil {
		return err
	}
	firewall(ctx, true)
	return nil
}

var running atomic.Bool

// startIssue runs one ACME order in the background; false when one runs.
func startIssue() bool {
	if !running.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer running.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		err := issue(ctx)
		msg := ""
		if err != nil {
			msg = err.Error()
			slog.Error("App-Zugang: Zertifikat nicht erhalten", "fehler", err)
		}
		_, _ = update(func(row *settingsRow) {
			row.LastRun = time.Now().UTC().Format(time.RFC3339)
			row.LastError = msg
		})
	}()
	return true
}

func issue(ctx context.Context) error {
	row, err := load()
	if err != nil {
		return err
	}
	if !row.Enabled || row.Host == "" {
		return errors.New("App-Zugang ist aus")
	}
	ip, err := tunnelIPv4()
	if err != nil {
		return err
	}
	if err := publishA(ctx, need(row, ip)); err != nil {
		return fmt.Errorf("A-Eintrag: %w", err)
	}
	if row.AccountKey == "" {
		_, pemKey, err := accountKey("")
		if err != nil {
			return err
		}
		if row, err = update(func(r *settingsRow) { r.AccountKey = pemKey }); err != nil {
			return err
		}
	}
	res, err := obtainCert(ctx, row.Host, row.AccountKey)
	if err != nil {
		return err
	}
	row, err = update(func(r *settingsRow) {
		r.CertPEM, r.CertKey = res.CertPEM, res.KeyPEM
		r.NotAfter = res.NotAfter.UTC().Format(time.RFC3339)
	})
	if err != nil {
		return err
	}
	if !row.Enabled {
		return nil
	}
	slog.Info("App-Zugang: Zertifikat erhalten", "name", row.Host, "gültig_bis", row.NotAfter)
	return activate(ctx, row)
}

func need(row settingsRow, ip string) domains.Need {
	return domains.Need{Zone: row.Zone, Name: row.Host, Type: "A", Value: ip, Kind: "a", Source: "apphttps"}
}

// needs shows the A record on the Domains page.
func needs() ([]domains.Need, error) {
	row, err := load()
	if err != nil || !row.Enabled || row.Host == "" {
		return nil, err
	}
	ip, err := tunnelIPv4()
	if err != nil {
		return nil, nil
	}
	return []domains.Need{need(row, ip)}, nil
}

// firewallRule opens the HTTPS port on wg0 only.
func firewallRule() []string {
	return []string{"allow", "in", "on", iface, "to", "any", "port", strconv.Itoa(Port), "proto", "tcp"}
}

// runUFW is replaced in tests.
var runUFW = func(ctx context.Context, args ...string) core.CommandResult {
	return core.RunAdmin(ctx, "ufw", args...)
}

func firewall(ctx context.Context, open bool) {
	if res := runUFW(ctx, "status"); res.Err != nil {
		return
	}
	args := append(firewallRule(), "comment", "GoDeb HTTPS im Tunnel")
	if !open {
		args = append([]string{"delete"}, firewallRule()...)
	}
	if res := runUFW(ctx, args...); res.Err != nil {
		slog.Warn("App-Zugang: ufw", "fehler", res.Detail())
	}
}

// run restores the files after a restart (the state directory may have
// been reset) and renews the certificate in time.
func run(ctx context.Context) {
	if row, err := load(); err == nil && row.Enabled && row.CertPEM != "" {
		if err := activate(ctx, row); err != nil {
			slog.Warn("App-Zugang: Dateien nicht geschrieben", "fehler", err)
		}
	}
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
		if row, err := load(); err == nil && row.Enabled && needsRenewal(row, time.Now()) {
			startIssue()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// expiryCheck warns when renewal keeps failing: 14 days before the end,
// critical 3 days before.
func expiryCheck(context.Context) []metrics.Finding {
	row, err := load()
	if err != nil || !row.Enabled {
		return nil
	}
	return expiryFinding(row, time.Now())
}

func expiryFinding(row settingsRow, now time.Time) []metrics.Finding {
	f := metrics.Finding{ID: "apphttps", Detail: row.LastError, Fix: &core.Fix{Action: "renewCert", Path: base + "/renew"}}
	end, err := time.Parse(time.RFC3339, row.NotAfter)
	switch {
	case row.CertPEM == "" || err != nil:
		if row.LastError == "" {
			return nil
		}
		f.Status, f.Title = metrics.StatusWarning, "App-Zugang: kein Zertifikat"
	case end.Sub(now) < 3*24*time.Hour:
		f.Status, f.Title = metrics.StatusCritical, "App-Zugang: Zertifikat läuft ab"
	case end.Sub(now) < 14*24*time.Hour:
		f.Status, f.Title = metrics.StatusWarning, "App-Zugang: Zertifikat läuft bald ab"
	default:
		return nil
	}
	if f.Detail == "" {
		f.Detail = "gültig bis " + end.Format("02.01.2006")
	}
	return []metrics.Finding{f}
}
