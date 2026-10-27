package mail

import (
	"context"
	"strings"

	"godeb/backend/core"
	"godeb/plugins/domains"
)

func init() {
	domains.RegisterNeeds(dnsNeeds)
}

// dnsNeeds publishes the DNS records the mail server needs for every
// configured mail domain. The domains plugin shows or applies them.
func dnsNeeds() ([]domains.Need, error) {
	s, ok, err := loadSettings()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	list, _, err := loadAll()
	if err != nil {
		return nil, err
	}
	ip := strings.TrimSpace(core.DefaultRoute(context.Background()).Src)
	var out []domains.Need
	for _, d := range list {
		for _, r := range expectedRecords(s, d, ip) {
			zone := d.Name
			// A/PTR for the mail hostname belong to the zone that owns the host.
			if r.Kind == "a" || r.Kind == "ptr" {
				if z := zoneOfHost(s.Hostname); z != "" {
					zone = z
				}
			}
			if r.Kind == "ptr" {
				// Attach PTR to the mail domain so the domains UI can apply it
				// together with the zone's forward records.
				zone = d.Name
			}
			out = append(out, domains.Need{
				Zone:   zone,
				Name:   r.Name,
				Type:   r.Type,
				Value:  r.Value,
				Kind:   r.Kind,
				Source: "mail",
			})
		}
	}
	return out, nil
}

func zoneOfHost(host string) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if i := strings.Index(host, "."); i > 0 && i+1 < len(host) {
		return host[i+1:]
	}
	return ""
}
