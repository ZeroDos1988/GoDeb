package domains

import "sync"

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
	needsMu.Lock()
	providers := append([]NeedsFunc(nil), needsProviders...)
	needsMu.Unlock()
	var out []Need
	for _, fn := range providers {
		all, err := fn()
		if err != nil {
			return nil, err
		}
		for _, n := range all {
			if n.Zone == zone {
				out = append(out, n)
			}
		}
	}
	return out, nil
}
