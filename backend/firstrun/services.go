package firstrun

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"godeb/backend/core"
	"godeb/backend/pluginmgr"
	"godeb/plugins/domains"
	"godeb/plugins/mail"
	mailgateway "godeb/plugins/mailgateway/backend"
	"godeb/plugins/wireguard"
)

// maxSetupDomains bounds one wizard request; more domains go through the
// domains page afterwards.
const maxSetupDomains = 25

// ServicesInfo tells the wizard what is already in place.
type ServicesInfo struct {
	MailRelayReady bool      `json:"mailRelayReady"` // Postfix and OpenDKIM installed
	MailServer     bool      `json:"mailServer"`     // the mail server plugin is set up (owns Postfix)
	MailHostname   string    `json:"mailHostname,omitempty"`
	MailJob        *mail.Job `json:"mailJob,omitempty"` // installation or certificate running or done
	RelayDomains   []string  `json:"relayDomains"`
	TokenSet       bool      `json:"tokenSet"` // Hetzner Cloud API token stored
}

// CurrentServices reads the state for the wizard.
func CurrentServices(ctx context.Context) ServicesInfo {
	info := ServicesInfo{
		MailServer: mail.Installed() || mailgateway.MailServerPresent(), MailHostname: mail.Hostname(),
		MailJob: mail.JobState(), TokenSet: domains.TokenSet(), RelayDomains: []string{},
	}
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
	if mailgateway.MailServerPresent() || mail.Installed() {
		return errors.New("Auf diesem Server läuft der Mailserver (Plugin E-Mail); Mail-Gateway und Mailserver schließen sich aus")
	}
	return pluginmgr.InstallAndEnable(ctx, "mailgateway")
}

// StartMailServer switches the mail plugin on and starts the installation
// of the full mail server (Postfix, Dovecot, Rspamd, Redis) in the
// background, the same job as the button on the mail page. Mail gateway
// and mail server exclude each other.
func StartMailServer(ctx context.Context, hostname string) error {
	if list, err := mailgateway.Domains(); err == nil && len(list) > 0 {
		return errors.New("Das Mail-Gateway ist schon für Domains eingerichtet; Mail-Gateway und Mailserver schließen sich aus")
	}
	if err := pluginmgr.InstallAndEnable(ctx, "mail"); err != nil {
		return err
	}
	return mail.StartInstall(hostname, false)
}

// WaitMailJob waits for the background job of the mail plugin (command
// line; the wizard polls /api/setup/info instead).
func WaitMailJob(ctx context.Context) error {
	for {
		job := mail.JobState()
		if job == nil || !job.Running {
			if job != nil && !job.OK {
				return errors.New("Mailserver: " + job.Error)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// DomainsSpec is the domains step of the wizard.
type DomainsSpec struct {
	Token      string   `json:"token"`      // Hetzner Cloud API token; empty keeps a stored one
	Domains    []string `json:"domains"`    // apex domains
	Manual     bool     `json:"manual"`     // keep the records by hand even with a stored token
	Defaults   bool     `json:"defaults"`   // web, www, mail, webmail, MX, SPF, DMARC
	MailRelay  bool     `json:"mailRelay"`  // accept mail for these domains on the gateway
	MailServer bool     `json:"mailServer"` // mail domains with postmaster mailbox on this host
	// First personal mailbox (mail server only), e.g. anna@example.com; its
	// domain must be in Domains. RolesToMailbox sends postmaster, abuse,
	// dmarc and tls-reports of every domain there instead of postmaster
	// mailboxes of their own.
	Mailbox         string `json:"mailbox"`
	MailboxPassword string `json:"mailboxPassword"`
	RolesToMailbox  bool   `json:"rolesToMailbox"`
	RelayHost       string `json:"relayHost"` // next hop in the tunnel, default 10.8.0.2
}

// DomainResult is what happened to one domain.
type DomainResult struct {
	Name       string           `json:"name"`
	Mode       string           `json:"mode"`
	MailRelay  bool             `json:"mailRelay"`
	MailServer bool             `json:"mailServer"`
	Records    []domains.Record `json:"records"`
	Error      string           `json:"error,omitempty"`
	// Mail server: the postmaster mailbox created now and its password,
	// shown once (only the hash is stored).
	Mailbox    string   `json:"mailbox,omitempty"`
	Password   string   `json:"password,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	Personal   string   `json:"personal,omitempty"`   // first personal mailbox created in this domain
	RoleTarget string   `json:"roleTarget,omitempty"` // where the role addresses go, if not postmaster@
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
	if spec.MailServer {
		if job := mail.JobState(); job != nil && job.Running {
			return nil, errors.New("Der Mailserver wird noch eingerichtet; bitte warten")
		}
		if !mail.Installed() {
			return nil, errors.New("Der Mailserver ist noch nicht eingerichtet")
		}
	}
	if spec.MailServer && relay {
		return nil, errors.New("Mail-Gateway und Mailserver schließen sich aus")
	}
	personal, personalDomain := "", ""
	if spec.MailServer && strings.TrimSpace(spec.Mailbox) != "" {
		if personal, err = mail.NormalizeAddress(spec.Mailbox); err != nil {
			return nil, err
		}
		_, personalDomain, _ = strings.Cut(personal, "@")
		if !slices.Contains(list, personalDomain) {
			return nil, errors.New("Die Domain von " + personal + " fehlt in der Liste")
		}
		if err := mail.CheckMailboxPassword(spec.MailboxPassword); err != nil {
			return nil, err
		}
		// The mailbox domain first, so the role addresses of the other
		// domains can point to the new mailbox.
		list = append([]string{personalDomain}, slices.DeleteFunc(list, func(d string) bool { return d == personalDomain })...)
	}
	roleTarget := ""
	if personal != "" && spec.RolesToMailbox {
		roleTarget = personal
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
		if spec.MailServer {
			opt := mail.DomainOptions{RoleTarget: roleTarget}
			if name == personalDomain {
				opt.Mailbox, _, _ = strings.Cut(personal, "@")
				opt.Password = spec.MailboxPassword
			}
			setup, err := mail.AddDomain(ctx, name, opt)
			switch {
			case err == nil, errors.Is(err, mail.ErrDomainExists):
				res.MailServer = true
				res.Mailbox, res.Password, res.Aliases = setup.Postmaster, setup.Password, setup.Aliases
				res.Personal, res.RoleTarget = setup.Mailbox, setup.RoleTarget
			default:
				problems = append(problems, "Mailserver: "+err.Error())
				if name == personalDomain {
					// Without the mailbox the other domains get their own
					// postmaster instead of forwarding into nothing.
					roleTarget = ""
				}
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
		MailRelay  bool   `json:"mailRelay"`
		MailServer bool   `json:"mailServer"`
		Hostname   string `json:"hostname"` // MX host name of the mail server
	}
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	if req.MailRelay && req.MailServer {
		core.WriteError(w, http.StatusBadRequest, "Mail-Gateway und Mailserver schließen sich aus")
		return
	}
	if req.MailRelay {
		if err := InstallMailRelay(r.Context()); err != nil {
			core.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.MailServer {
		if err := StartMailServer(r.Context(), req.Hostname); err != nil {
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
