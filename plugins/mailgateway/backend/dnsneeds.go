package mailgateway

import (
	"context"
	"strings"

	"godeb/backend/core"
	"godeb/plugins/domains"
)

func init() {
	domains.RegisterNeeds(dnsNeeds)
}

// dnsNeeds publishes what the relay needs per domain: MX to mail.<domain>,
// its A record on this host, SPF, DKIM and DMARC. The values match what
// the dns-check of this plugin expects.
func dnsNeeds() ([]domains.Need, error) {
	conn, err := openStore()
	if err != nil {
		return nil, err
	}
	list, err := listDomains(conn)
	if err != nil {
		return nil, err
	}
	ip := strings.TrimSpace(core.DefaultRoute(context.Background()).Src)
	return needsFor(list, ip), nil
}

func needsFor(list []storedDomain, ip string) []domains.Need {
	var out []domains.Need
	for _, d := range list {
		host := "mail." + d.Domain
		add := func(name, typ, value, kind string) {
			out = append(out, domains.Need{Zone: d.Domain, Name: name, Type: typ, Value: value, Kind: kind, Source: "mailgateway"})
		}
		add(d.Domain, "MX", "10 "+host+".", "mx")
		spf := "v=spf1 mx a -all"
		if ip != "" {
			add(host, "A", ip, "a")
			spf = "v=spf1 mx a ip4:" + ip + " -all"
		}
		add(d.Domain, "TXT", spf, "spf")
		if d.DKIMPublic != "" {
			selector := d.Selector
			if selector == "" {
				selector = "godeb"
			}
			add(selector+"._domainkey."+d.Domain, "TXT", "v=DKIM1; h=sha256; k=rsa; p="+d.DKIMPublic, "dkim")
		}
		add("_dmarc."+d.Domain, "TXT", "v=DMARC1; p=quarantine; rua=mailto:dmarc@"+d.Domain, "dmarc")
	}
	return out
}
