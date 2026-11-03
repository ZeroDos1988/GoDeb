package domains

import (
	"context"
	"strings"
	"sync"

	"godeb/backend/core"
)

// NeedsFunc returns the DNS records a plugin wants published.
type NeedsFunc func() ([]Need, error)

var (
	needsMu        sync.Mutex
	needsProviders []NeedsFunc
)

// RegisterNeeds adds a provider. Plugins call this from init(); domains
// never imports them, so there is no cycle.
func RegisterNeeds(fn NeedsFunc) {
	if fn == nil {
		return
	}
	needsMu.Lock()
	needsProviders = append(needsProviders, fn)
	needsMu.Unlock()
}

// resetNeedsForTest clears providers between unit tests.
func resetNeedsForTest() {
	needsMu.Lock()
	needsProviders = nil
	needsMu.Unlock()
}

func collectNeeds(zone string) ([]Need, error) {
	all, err := allNeeds()
	if err != nil {
		return nil, err
	}
	var out []Need
	for _, n := range all {
		if n.Zone == zone {
			out = append(out, n)
		}
	}
	return out, nil
}

func allNeeds() ([]Need, error) {
	needsMu.Lock()
	providers := append([]NeedsFunc(nil), needsProviders...)
	needsMu.Unlock()
	var out []Need
	for _, fn := range providers {
		list, err := fn()
		if err != nil {
			return nil, err
		}
		out = append(out, list...)
	}
	list, err := listDomains()
	if err != nil {
		return nil, err
	}
	return withDefaults(out, list, publicIP()), nil
}

// publicIP is the source address of the default route. Tests replace it.
var publicIP = func() string {
	return strings.TrimSpace(core.DefaultRoute(context.Background()).Src)
}

// defaultNeeds are the records of a domain whose web and mail run behind
// this host: the apex and www for websites, mail and webmail, MX, SPF and
// DMARC. DKIM needs a key and comes from the mail plugins.
func defaultNeeds(zone, ip string) []Need {
	n := func(name, typ, value, kind string) Need {
		return Need{Zone: zone, Name: name, Type: typ, Value: value, Kind: kind, Source: "domains"}
	}
	var out []Need
	if ip != "" {
		out = append(out,
			n(zone, "A", ip, "a"),
			n("www."+zone, "A", ip, "a"),
			n("mail."+zone, "A", ip, "a"),
			n("webmail."+zone, "A", ip, "a"),
		)
	}
	return append(out,
		n(zone, "MX", "10 mail."+zone+".", "mx"),
		n(zone, "TXT", "v=spf1 mx -all", "spf"),
		n("_dmarc."+zone, "TXT", "v=DMARC1; p=quarantine; adkim=s; aspf=s", "dmarc"),
	)
}

// withDefaults adds the default records of every domain that has them
// switched on. A record a plugin already asks for (same zone, name and
// type) wins, so the mail server's MX or a website's A record are kept.
func withDefaults(needs []Need, list []Domain, ip string) []Need {
	key := func(n Need) string { return n.Zone + "|" + strings.ToLower(n.Name) + "|" + n.Type }
	seen := map[string]bool{}
	for _, n := range needs {
		seen[key(n)] = true
	}
	for _, d := range list {
		if !d.Defaults {
			continue
		}
		for _, n := range defaultNeeds(d.Name, ip) {
			if !seen[key(n)] {
				seen[key(n)] = true
				needs = append(needs, n)
			}
		}
	}
	return needs
}

// CheckAll looks up every record the plugins need in public DNS, over all
// zones (also PTR). Used by godeb doctor.
func CheckAll(ctx context.Context) ([]Record, error) {
	needs, err := allNeeds()
	if err != nil {
		return nil, err
	}
	return recordsFor(ctx, needs), nil
}

// ZoneFor returns the zone a host belongs to, so plugins can fill Need.Zone
// for subdomains: the longest configured zone containing host, otherwise
// host without its first label (www.example.com -> example.com) when that
// still has a dot, otherwise host itself.
func ZoneFor(host string) string {
	list, _ := listDomains()
	return zoneFor(host, list)
}

func zoneFor(host string, list []Domain) string {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
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
