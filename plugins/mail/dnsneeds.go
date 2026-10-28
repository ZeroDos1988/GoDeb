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

// dnsNeeds publishes the DNS records the mail server needs. MX, SPF, DKIM
// and DMARC belong to each mail domain; A and PTR of the mail host exist
// once, in the zone that contains the host name.
func dnsNeeds() ([]domains.Need, error) {
	s, ok, err := loadSettings()
	if err != nil || !ok {
		return nil, err
	}
	list, _, err := loadAll()
	if err != nil {
		return nil, err
	}
	ip := strings.TrimSpace(core.DefaultRoute(context.Background()).Src)
	hostZone := zoneOfHost(s.Hostname, list)
	seen := map[string]bool{}
	var out []domains.Need
	for _, d := range list {
		for _, r := range expectedRecords(s, d, ip) {
			zone := d.Name
			if r.Kind == "a" || r.Kind == "ptr" {
				zone = hostZone
			}
			// mtasts-host stays in the mail domain zone (mta-sts.<domain>).
			key := zone + "|" + r.Name + "|" + r.Type
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, domains.Need{Zone: zone, Name: r.Name, Type: r.Type, Value: r.Value, Kind: r.Kind, Source: "mail"})
		}
	}
	return out, nil
}

// zoneOfHost picks the longest mail domain the host lies in, otherwise the
// host name without its first label.
func zoneOfHost(host string, list []Domain) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	best := ""
	for _, d := range list {
		if (host == d.Name || strings.HasSuffix(host, "."+d.Name)) && len(d.Name) > len(best) {
			best = d.Name
		}
	}
	if best != "" {
		return best
	}
	if _, rest, ok := strings.Cut(host, "."); ok && strings.Contains(rest, ".") {
		return rest
	}
	return host
}
