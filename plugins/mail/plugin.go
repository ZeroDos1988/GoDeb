package mail

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"godeb/backend/core"
)

func init() {
	core.Register("mail", Mount)
}

const base = "/api/plugins/mail"

// Mount registers the mail server routes.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+base+"/status", handleStatus)
	mux.HandleFunc("POST "+base+"/install", handleInstall)
	mux.HandleFunc("POST "+base+"/clamav", handleClamAV)
	mux.HandleFunc("POST "+base+"/certificate", handleCertificate)
	mux.HandleFunc("POST "+base+"/domains", handleAddDomain)
	mux.HandleFunc("GET "+base+"/domains/{domain}", handleDomain)
	mux.HandleFunc("POST "+base+"/domains/{domain}", handleEditDomain)
	mux.HandleFunc("POST "+base+"/domains/{domain}/delete", handleDeleteDomain)
	mux.HandleFunc("GET "+base+"/domains/{domain}/dns", handleDNS)
	mux.HandleFunc("POST "+base+"/domains/{domain}/accounts", handleAddAccount)
	mux.HandleFunc("POST "+base+"/domains/{domain}/accounts/{local}", handleEditAccount)
	mux.HandleFunc("POST "+base+"/domains/{domain}/accounts/{local}/delete", handleDeleteAccount)
}

type serviceState struct {
	Unit   string `json:"unit"`
	Active string `json:"active"`
}

type domainSummary struct {
	Domain
	Accounts int `json:"accounts"`
}

type statusResponse struct {
	Installed   bool            `json:"installed"`
	Settings    Settings        `json:"settings"`
	LetsEncrypt bool            `json:"letsEncrypt"`
	PublicIP    string          `json:"publicIp"`
	HostOK      bool            `json:"hostResolves"`
	Gateway     int             `json:"gatewayDomains"`
	Services    []serviceState  `json:"services"`
	Domains     []domainSummary `json:"domains"`
	Job         *Job            `json:"job,omitempty"`
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	s, installed, err := loadSettings()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := statusResponse{Installed: installed, Settings: s, PublicIP: core.DefaultRoute(r.Context()).Src, Gateway: gatewayDomains(), Job: jobs.snapshot(), Domains: []domainSummary{}, Services: []serviceState{}}
	resp.LetsEncrypt = strings.HasPrefix(s.CertFile, "/etc/letsencrypt/")
	if installed {
		resp.HostOK = resolvesTo(r.Context(), s.Hostname, resp.PublicIP)
		units := []string{"postfix", "dovecot", "rspamd", "redis-server"}
		if s.ClamAV {
			units = append(units, "clamav-daemon")
		}
		for _, u := range units {
			st := strings.TrimSpace(core.RunQuiet(r.Context(), "systemctl", "is-active", u).Stdout)
			if st == "" {
				st = "unknown"
			}
			resp.Services = append(resp.Services, serviceState{Unit: u, Active: st})
		}
		domains, accounts, err := loadAll()
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, d := range domains {
			n := 0
			for _, a := range accounts {
				if a.Domain == d.Name {
					n++
				}
			}
			resp.Domains = append(resp.Domains, domainSummary{Domain: d, Accounts: n})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, resp)
}

type installRequest struct {
	Hostname string `json:"hostname"`
	ClamAV   bool   `json:"clamav"`
}

func handleInstall(w http.ResponseWriter, r *http.Request) {
	var req installRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	host, err := NormalizeHostname(req.Hostname)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if n := gatewayDomains(); n > 0 {
		core.WriteError(w, http.StatusConflict, "Das Mail-Gateway ist eingerichtet ("+strconv.Itoa(n)+" Relay-Domains). Beide konfigurieren Postfix; erst die Domains im Mail-Gateway entfernen.")
		return
	}
	s, _, err := loadSettings()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Hostname, s.ClamAV = host, req.ClamAV
	if err := jobs.start("install", func(ctx context.Context) error { return install(ctx, s) }); err != nil {
		core.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "Mailserver-Einrichtung gestartet", "hostname", host, "clamav", req.ClamAV)
	core.WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func handleClamAV(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !core.DecodeJSON(w, r, &req) || !requireInstalled(w) {
		return
	}
	if err := jobs.start("clamav", func(ctx context.Context) error { return setClamAV(ctx, req.Enabled) }); err != nil {
		core.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func handleCertificate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !core.DecodeJSON(w, r, &req) || !requireInstalled(w) {
		return
	}
	email := ""
	if strings.TrimSpace(req.Email) != "" {
		var err error
		if email, err = NormalizeAddress(req.Email); err != nil {
			core.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := jobs.start("certificate", func(ctx context.Context) error { return certificate(ctx, email) }); err != nil {
		core.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func requireInstalled(w http.ResponseWriter) bool {
	if _, ok, err := loadSettings(); err != nil || !ok {
		core.WriteError(w, http.StatusConflict, "Der Mailserver ist noch nicht eingerichtet")
		return false
	}
	return true
}

// applyAndRespond writes the configuration and answers with its notes.
func applyAndRespond(w http.ResponseWriter, r *http.Request, status int, body map[string]any) {
	log, err := apply(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	body["ok"] = true
	body["notes"] = log
	core.WriteJSON(w, status, body)
}

func handleAddDomain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !core.DecodeJSON(w, r, &req) || !requireInstalled(w) {
		return
	}
	name, err := NormalizeDomain(req.Name)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	domains, _, err := loadAll()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := findDomain(domains, name); err == nil {
		core.WriteError(w, http.StatusConflict, name+" ist bereits eingerichtet")
		return
	}
	priv, pub, err := newDKIMKey()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d := Domain{Name: name, Selector: "godeb", DKIMPublic: pub, Created: time.Now().UTC().Format(time.RFC3339)}
	if err := writeDKIM(d.Name, d.Selector, priv); err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := saveDomain(d); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "Maildomain angelegt", "domain", name)
	applyAndRespond(w, r, http.StatusCreated, map[string]any{"domain": d})
}

// pathDomain reads and validates {domain} and loads the data.
func pathDomain(w http.ResponseWriter, r *http.Request) (Domain, []Domain, []Account, bool) {
	name, err := NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return Domain{}, nil, nil, false
	}
	domains, accounts, err := loadAll()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return Domain{}, nil, nil, false
	}
	d, err := findDomain(domains, name)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, name+" ist keine Maildomain")
		return Domain{}, nil, nil, false
	}
	return d, domains, accounts, true
}

type accountView struct {
	Account
	Address string `json:"address"`
	UsedKB  int64  `json:"usedKB"` // -1 unknown
}

func handleDomain(w http.ResponseWriter, r *http.Request) {
	d, _, accounts, ok := pathDomain(w, r)
	if !ok {
		return
	}
	usage := quotaUsage(r.Context())
	list := []accountView{}
	for _, a := range accounts {
		if a.Domain != d.Name {
			continue
		}
		used, known := usage[a.Address()]
		if !known {
			used = -1
		}
		list = append(list, accountView{Account: a, Address: a.Address(), UsedKB: used})
	}
	s, _, _ := loadSettings()
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"domain":   d,
		"accounts": list,
		"records":  expectedRecords(s, d, core.DefaultRoute(r.Context()).Src),
		"client":   map[string]any{"host": s.Hostname, "imapPort": 993, "smtpPort": 465, "submissionPort": 587},
	})
}

func handleDNS(w http.ResponseWriter, r *http.Request) {
	d, _, _, ok := pathDomain(w, r)
	if !ok {
		return
	}
	s, _, _ := loadSettings()
	core.WriteJSON(w, http.StatusOK, map[string]any{"records": checkRecords(r.Context(), expectedRecords(s, d, core.DefaultRoute(r.Context()).Src))})
}

func handleEditDomain(w http.ResponseWriter, r *http.Request) {
	d, _, accounts, ok := pathDomain(w, r)
	if !ok {
		return
	}
	var req struct {
		CatchAll  string `json:"catchAll"`
		Suspended bool   `json:"suspended"`
	}
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	d.CatchAll = ""
	if strings.TrimSpace(req.CatchAll) != "" {
		addr, err := NormalizeAddress(req.CatchAll)
		if err != nil {
			core.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Catch-all into an address of the same domain must exist, or
		// Postfix would loop.
		if strings.HasSuffix(addr, "@"+d.Name) && !conflict(accounts, addr, "") {
			core.WriteError(w, http.StatusBadRequest, addr+" gibt es nicht")
			return
		}
		d.CatchAll = addr
	}
	d.Suspended = req.Suspended
	if err := saveDomain(d); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	applyAndRespond(w, r, http.StatusOK, map[string]any{"domain": d})
}

func handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	d, _, _, ok := pathDomain(w, r)
	if !ok {
		return
	}
	if err := deleteDomain(d.Name); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log, err := apply(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	removeDKIM(d.Name, d.Selector)
	if err := removeMailbox(d.Name, ""); err != nil {
		log = append(log, "Postfächer nicht gelöscht: "+err.Error())
	}
	slog.InfoContext(r.Context(), "Maildomain gelöscht", "domain", d.Name)
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "notes": log})
}

type accountRequest struct {
	Local       string   `json:"local"`
	Password    string   `json:"password"`
	QuotaMB     int      `json:"quotaMB"`
	Aliases     []string `json:"aliases"`
	Forward     []string `json:"forward"`
	ForwardOnly bool     `json:"forwardOnly"`
	Suspended   bool     `json:"suspended"`
}

func handleAddAccount(w http.ResponseWriter, r *http.Request) {
	d, _, accounts, ok := pathDomain(w, r)
	if !ok {
		return
	}
	var req accountRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	a, err := cleanAccount(Account{Domain: d.Name, Local: req.Local, QuotaMB: req.QuotaMB, Aliases: req.Aliases, Forward: req.Forward, ForwardOnly: req.ForwardOnly, Suspended: req.Suspended, Created: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := checkAccount(accounts, a, ""); err != nil {
		core.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if !a.ForwardOnly {
		if a.Hash, err = HashPassword(req.Password); err != nil {
			core.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := saveAccount(a); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "Postfach angelegt", "adresse", a.Address())
	applyAndRespond(w, r, http.StatusCreated, map[string]any{"address": a.Address()})
}

func pathAccount(w http.ResponseWriter, r *http.Request) (Account, []Account, bool) {
	d, _, accounts, ok := pathDomain(w, r)
	if !ok {
		return Account{}, nil, false
	}
	local, err := NormalizeLocal(r.PathValue("local"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return Account{}, nil, false
	}
	a, err := findAccount(accounts, local+"@"+d.Name)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, local+"@"+d.Name+" gibt es nicht")
		return Account{}, nil, false
	}
	return a, accounts, true
}

func handleEditAccount(w http.ResponseWriter, r *http.Request) {
	old, accounts, ok := pathAccount(w, r)
	if !ok {
		return
	}
	var req accountRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	a, err := cleanAccount(Account{Domain: old.Domain, Local: old.Local, Hash: old.Hash, QuotaMB: req.QuotaMB, Aliases: req.Aliases, Forward: req.Forward, ForwardOnly: req.ForwardOnly, Suspended: req.Suspended, Created: old.Created})
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := checkAccount(accounts, a, old.Address()); err != nil {
		core.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if req.Password != "" {
		if a.Hash, err = HashPassword(req.Password); err != nil {
			core.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if !a.ForwardOnly && a.Hash == "" {
		core.WriteError(w, http.StatusBadRequest, "Für ein Postfach ist ein Passwort nötig")
		return
	}
	if err := saveAccount(a); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.InfoContext(r.Context(), "Postfach geändert", "adresse", a.Address(), "passwort_neu", req.Password != "")
	applyAndRespond(w, r, http.StatusOK, map[string]any{"address": a.Address()})
}

func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	a, _, ok := pathAccount(w, r)
	if !ok {
		return
	}
	if err := deleteAccount(a.Address()); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log, err := apply(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := removeMailbox(a.Domain, a.Local); err != nil {
		log = append(log, "Postfach-Daten nicht gelöscht: "+err.Error())
	}
	slog.InfoContext(r.Context(), "Postfach gelöscht", "adresse", a.Address())
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "notes": log})
}

// quotaUsage returns the used storage in KB per address from Dovecot.
func quotaUsage(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	res := core.RunQuiet(ctx, "doveadm", "-f", "tab", "quota", "get", "-A")
	if res.Err != nil {
		return out
	}
	return parseQuota(res.Stdout)
}

// parseQuota reads `doveadm -f tab quota get -A`: a header line, then
// username, quota name, type, value, limit, percent.
func parseQuota(stdout string) map[string]int64 {
	out := map[string]int64{}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 2 {
		return out
	}
	head := strings.Split(lines[0], "\t")
	col := func(name string) int {
		for i, h := range head {
			if strings.EqualFold(strings.TrimSpace(h), name) {
				return i
			}
		}
		return -1
	}
	cu, ct, cv := col("username"), col("type"), col("value")
	if cu < 0 || ct < 0 || cv < 0 {
		return out
	}
	for _, l := range lines[1:] {
		f := strings.Split(l, "\t")
		if len(f) <= cv || len(f) <= cu || len(f) <= ct || !strings.EqualFold(f[ct], "STORAGE") {
			continue
		}
		if v, err := strconv.ParseInt(strings.TrimSpace(f[cv]), 10, 64); err == nil {
			out[strings.ToLower(f[cu])] = v
		}
	}
	return out
}
