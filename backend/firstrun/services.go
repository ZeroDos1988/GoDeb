package firstrun

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"godeb/backend/core"
	"godeb/backend/pluginmgr"
	"godeb/plugins/domains"
	mailgateway "godeb/plugins/mailgateway/backend"
	"godeb/plugins/wireguard"
)

// maxSetupDomains bounds one wizard request; more domains go through the
// domains page afterwards.
const maxSetupDomains = 25

// ServicesInfo tells the wizard what is already in place.
type ServicesInfo struct {
	MailRelayReady bool     `json:"mailRelayReady"` // Postfix and OpenDKIM installed
	MailServer     bool     `json:"mailServer"`     // the mail server plugin owns Postfix
	RelayDomains   []string `json:"relayDomains"`
	TokenSet       bool     `json:"tokenSet"` // Hetzner Cloud API token stored
}

// CurrentServices reads the state for the wizard.
func CurrentServices(ctx context.Context) ServicesInfo {
	info := ServicesInfo{MailServer: mailgateway.MailServerPresent(), TokenSet: domains.TokenSet(), RelayDomains: []string{}}
	info.MailRelayReady, _ = pluginmgr.PackagesReady(ctx, "mailgateway")
	if list, err := mailgateway.Domains(); err == nil {
		info.RelayDomains = list
	}
	return info
}

// InstallMailRelay installs Postfix and OpenDKIM for the mail gateway and
// switches the plugin on, through the same code as the plugin manager. It
// only runs on a hub: the gateway is the public entry for mail.
func InstallMailRelay(ctx context.Context) error {
	role, err := wireguard.CurrentRole()
	if err != nil {
		return err
	}
	if role.Role != wireguard.RoleHub {
		return errors.New("Das Mail-Gateway gehört auf den WireGuard-Server (VPS), der die Post annimmt")
	}
	if mailgateway.MailServerPresent() {
		return errors.New("Auf diesem Server läuft der Mailserver (Plugin E-Mail); Mail-Gateway und Mailserver schließen sich aus")
	}
	return pluginmgr.InstallAndEnable(ctx, "mailgateway")
}

// DomainsSpec is the domains step of the wizard.
type DomainsSpec struct {
	Token     string   `json:"token"`     // Hetzner Cloud API token; empty keeps a stored one
	Domains   []string `json:"domains"`   // apex domains
	Manual    bool     `json:"manual"`    // keep the records by hand even with a stored token
	Defaults  bool     `json:"defaults"`  // web, www, mail, webmail, MX, SPF, DMARC
	MailRelay bool     `json:"mailRelay"` // accept mail for these domains on the gateway
	RelayHost string   `json:"relayHost"` // next hop in the tunnel, default 10.8.0.2
}

// DomainResult is what happened to one domain.
type DomainResult struct {
	Name      string           `json:"name"`
	Mode      string           `json:"mode"`
	MailRelay bool             `json:"mailRelay"`
	Records   []domains.Record `json:"records"`
	Error     string           `json:"error,omitempty"`
}

// CleanDomains normalizes, deduplicates and bounds the domain list.
func CleanDomains(in []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		d, err := domains.NormalizeDomain(raw)
		if err != nil {
			return nil, errors.New("Domain ist ungültig: " + strings.TrimSpace(raw))
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("Keine Domain angegeben")
	}
	if len(out) > maxSetupDomains {
		return nil, errors.New("Zu viele Domains auf einmal; weitere später unter Domains eintragen")
	}
	return out, nil
}

// SetupDomains enters the domains in the domains plugin, sets up the mail
// gateway for them when asked and, with a Hetzner token, creates the DNS
// records right away. Without a token the domains are kept by hand and the
// result lists the records to enter at the DNS provider. A failure of one
// domain does not stop the others.
func SetupDomains(ctx context.Context, spec DomainsSpec) ([]DomainResult, error) {
	list, err := CleanDomains(spec.Domains)
	if err != nil {
		return nil, err
	}
	if token := strings.TrimSpace(spec.Token); token != "" {
		if err := domains.SetToken(ctx, token); err != nil {
			return nil, err
		}
	}
	mode := domains.ModeManual
	if domains.TokenSet() && !spec.Manual {
		mode = domains.ModeHetzner
	}
	relay := false
	if spec.MailRelay {
		if relay, _ = pluginmgr.PackagesReady(ctx, "mailgateway"); !relay {
			return nil, errors.New("Das Mail-Gateway ist noch nicht installiert")
		}
	}
	out := make([]DomainResult, 0, len(list))
	for _, name := range list {
		res := DomainResult{Name: name, Mode: mode, Records: []domains.Record{}}
		if _, err := domains.Ensure(name, mode, spec.Defaults); err != nil {
			res.Error = err.Error()
			out = append(out, res)
			continue
		}
		var problems []string
		if relay {
			if _, err := mailgateway.Configure(ctx, mailgateway.Request{Domain: name, RelayHost: spec.RelayHost}); err != nil {
				problems = append(problems, "Mail-Gateway: "+err.Error())
			} else {
				res.MailRelay = true
			}
		}
		var recs []domains.Record
		if mode == domains.ModeHetzner {
			recs, err = domains.Apply(ctx, name)
			if err != nil {
				problems = append(problems, "DNS: "+err.Error())
				recs, _ = domains.Records(ctx, name)
			}
		} else {
			recs, err = domains.Records(ctx, name)
			if err != nil {
				problems = append(problems, "DNS: "+err.Error())
			}
		}
		if recs != nil {
			res.Records = recs
		}
		res.Error = strings.Join(problems, "; ")
		out = append(out, res)
	}
	return out, nil
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MailRelay bool `json:"mailRelay"`
	}
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	if req.MailRelay {
		if err := InstallMailRelay(r.Context()); err != nil {
			core.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	core.WriteJSON(w, http.StatusOK, CurrentServices(r.Context()))
}

func handleDomains(w http.ResponseWriter, r *http.Request) {
	var spec DomainsSpec
	if !core.DecodeJSON(w, r, &spec) {
		return
	}
	res, err := SetupDomains(r.Context(), spec)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, map[string]any{"domains": res, "services": CurrentServices(r.Context())})
}
