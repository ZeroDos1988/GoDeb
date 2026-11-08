// Package sites sets up the hosts behind a GoDeb hub in one step: the hub
// creates the WireGuard site with a profile (home server or GoDeb Play),
// points its mail gateway there, publishes the mail host, issues its
// certificate and hands out a setup script for the new host. On the site
// the same package fetches the certificate over the tunnel.
//
// Split of the work: the hub (VPS) keeps WireGuard, DNS, certificates and
// the mail entry and exit with DKIM; the home server keeps mailboxes, spam
// filter and virus scanner; GoDeb Play runs game servers.
package sites

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"godeb/backend/core"
	"godeb/plugins/domains"
	mailgateway "godeb/plugins/mailgateway/backend"
	"godeb/plugins/wireguard"
)

// mailLabel is the first label of the mail host of a home server.
const mailLabel = "postfach"

// Site is one site of this hub as the UI shows it.
type Site struct {
	Name    string                `json:"name"`
	Address string                `json:"address"`
	Profile wireguard.SiteProfile `json:"profile"`
	Created string                `json:"created,omitempty"`
	Cert    *CertState            `json:"cert,omitempty"`
	WebUI   string                `json:"webUI"`
}

// AddRequest describes a new site.
type AddRequest struct {
	Name     string `json:"name"`
	Profile  string `json:"profile"`  // home or play
	Endpoint string `json:"endpoint"` // public host[:port] of this hub; empty = as for the other clients
}

// AddResult is shown once: the join code and the script carry the site's
// private WireGuard key.
type AddResult struct {
	Site       Site     `json:"site"`
	JoinCode   string   `json:"joinCode"`
	Script     string   `json:"script"`
	ScriptName string   `json:"scriptName"`
	Notes      []string `json:"notes"`
}

// Hooks for tests.
var (
	createPeer   = wireguard.CreatePeer
	loadClients  = wireguard.LoadClients
	currentRole  = wireguard.CurrentRole
	relayDomains = mailgateway.Domains
	setRelay     = mailgateway.SetRelay
	publish      = domains.Publish
	tokenSet     = domains.TokenSet
	listenPort   = defaultListenPort
)

// Add creates the site on this hub. The CLI (`godeb site add --profile`)
// and the web UI both call it.
func Add(ctx context.Context, req AddRequest) (AddResult, error) {
	role, err := currentRole()
	if err != nil {
		return AddResult{}, err
	}
	if role.Role != wireguard.RoleHub {
		return AddResult{}, &wireguard.RequestError{Status: http.StatusConflict, Msg: "Standorte legt nur der WireGuard-Hub (VPS) an"}
	}
	profile := wireguard.SiteProfile{Kind: strings.TrimSpace(req.Profile)}
	if profile.Kind != wireguard.ProfileHome && profile.Kind != wireguard.ProfilePlay {
		return AddResult{}, &wireguard.RequestError{Status: http.StatusBadRequest, Msg: "Profil: home (Heim-Server) oder play (GoDeb Play)"}
	}
	var notes []string
	if profile.Kind == wireguard.ProfileHome {
		list, err := relayDomains()
		switch {
		case err != nil:
			notes = append(notes, "Mail-Gateway nicht lesbar ("+err.Error()+"); der Heim-Server bekommt keinen Mailserver.")
		case len(list) == 0:
			notes = append(notes, "Im Mail-Gateway ist keine Domain eingetragen; der Heim-Server bekommt keinen Mailserver. Erst Domains mit Mail-Relay einrichten, dann den Standort anlegen.")
		default:
			profile.MailDomains = list
			profile.MailHost = mailLabel + "." + list[0]
		}
	}
	endpoint := strings.TrimSpace(req.Endpoint)
	if endpoint == "" {
		endpoint = wireguard.DefaultEndpoint()
	}
	if endpoint == "" {
		return AddResult{}, &wireguard.RequestError{Status: http.StatusBadRequest, Msg: "Öffentliche Adresse dieses Hubs angeben (Endpoint)"}
	}
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		endpoint = net.JoinHostPort(endpoint, fmt.Sprint(listenPort()))
	}
	res, err := createPeer(ctx, wireguard.PeerRequest{Name: req.Name, Endpoint: endpoint, Site: true, Profile: profile})
	if err != nil {
		return AddResult{}, err
	}
	addr := strings.TrimSuffix(res.Peer.AllowedIPs[0], "/32")
	site := Site{Name: res.Peer.Name, Address: addr, Profile: profile, Created: res.Peer.Created, WebUI: "http://" + net.JoinHostPort(addr, "8080")}
	if res.Detail != "" {
		notes = append(notes, res.Detail)
	}
	if profile.MailHost != "" {
		notes = append(notes, setupMail(ctx, site)...)
	}
	code := wireguard.JoinCode(res.ClientConfig)
	script, err := Script(profile.Kind, site.Name, endpoint, code, time.Now())
	if err != nil {
		return AddResult{}, err
	}
	slog.Info("Standort angelegt", "name", site.Name, "adresse", addr, "profil", profile.Kind, "mail_host", profile.MailHost)
	return AddResult{Site: site, JoinCode: code, Script: script, ScriptName: ScriptName(profile.Kind, site.Name), Notes: notes}, nil
}

// setupMail hands the mail of the gateway domains to the new home server,
// publishes its mail host and starts the certificate. Failures become notes:
// the site exists either way, and every step can be repeated.
func setupMail(ctx context.Context, s Site) []string {
	var notes []string
	if err := setRelay(ctx, s.Address); err != nil {
		notes = append(notes, "Mail-Gateway nicht umgestellt: "+err.Error())
	} else {
		notes = append(notes, "Mail-Gateway gibt die Post für "+strings.Join(s.Profile.MailDomains, ", ")+" an "+s.Address+" weiter und verschickt für den Heim-Server.")
	}
	if !tokenSet() {
		notes = append(notes, fmt.Sprintf("Kein Hetzner-Token: A-Eintrag %s → %s bitte von Hand anlegen. Ohne Token gibt es kein Zertifikat; die Mailprogramme sehen dann ein selbstsigniertes.", s.Profile.MailHost, s.Address))
		return notes
	}
	if err := publish(ctx, mailHostNeed(s)); err != nil {
		notes = append(notes, "A-Eintrag "+s.Profile.MailHost+": "+err.Error())
		return notes
	}
	startIssue(s.Profile.MailHost, s.Name)
	return append(notes, "Zertifikat für "+s.Profile.MailHost+" wird geholt; der Heim-Server holt es sich nach der Einrichtung über den Tunnel.")
}

func mailHostNeed(s Site) domains.Need {
	zone := ""
	if len(s.Profile.MailDomains) > 0 {
		zone = s.Profile.MailDomains[0]
	}
	return domains.Need{Zone: zone, Name: s.Profile.MailHost, Type: "A", Value: s.Address, Kind: "a", Source: "sites"}
}

// List returns the sites of this hub with their profile.
func List() ([]Site, error) {
	clients, err := loadClients()
	if err != nil {
		return nil, err
	}
	certs, _ := loadCerts()
	out := []Site{}
	for _, c := range clients {
		if !c.Site || len(c.Addresses) == 0 {
			continue
		}
		addr := strings.TrimSuffix(strings.TrimSpace(c.Addresses[0]), "/32")
		s := Site{Name: c.Name, Address: addr, Profile: c.Profile, Created: c.Created, WebUI: "http://" + net.JoinHostPort(addr, "8080")}
		if st, ok := certs[c.Profile.MailHost]; ok && c.Profile.MailHost != "" {
			s.Cert = &st
		}
		out = append(out, s)
	}
	return out, nil
}

// siteFor finds the site with the mail host whose tunnel address is ip.
func siteFor(host, ip string) (Site, error) {
	list, err := List()
	if err != nil {
		return Site{}, err
	}
	for _, s := range list {
		if host != "" && s.Profile.MailHost == host && s.Address == ip {
			return s, nil
		}
	}
	return Site{}, errors.New("kein Standort")
}

func defaultListenPort() int {
	data, err := core.ReadAdminFile(wireguard.ConfigPath())
	if err == nil {
		var p int
		if _, err := fmt.Sscan(wireguard.ParseConfig(string(data)).Interface.ListenPort, &p); err == nil && p > 0 {
			return p
		}
	}
	return 51820
}
