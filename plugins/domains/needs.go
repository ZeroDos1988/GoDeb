package domains

import (
	"context"
	"strings"
	"sync"
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
	return out, nil
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
