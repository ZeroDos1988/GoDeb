// Package domains keeps a registry of DNS zones and can publish the records
// other plugins need (today: mail) either as a checklist or through the
// Hetzner Cloud DNS API.
package domains

import (
	"errors"
	"regexp"
	"strings"
)

// Mode is how GoDeb treats a zone.
const (
	ModeManual  = "manual"  // show and check; admin copies records
	ModeHetzner = "hetzner" // create and update RRSets via the Cloud API
)

// Domain is one apex zone managed here.
type Domain struct {
	Name    string `json:"name"`
	Mode    string `json:"mode"`
	Created string `json:"created"`
}

// Settings holds the Hetzner Cloud API token (sealed at rest) and optional
// server id for reverse DNS.
type Settings struct {
	TokenSet  bool   `json:"tokenSet"`
	ServerID  int64  `json:"serverId,omitempty"`
	TokenHint string `json:"tokenHint,omitempty"` // last 4 chars when set
}

// Need is one DNS record a plugin wants published.
type Need struct {
	Zone   string `json:"zone"`   // apex, empty for PTR
	Name   string `json:"name"`   // FQDN or IP for PTR
	Type   string `json:"type"`   // A, MX, TXT, PTR
	Value  string `json:"value"`  // as shown to the admin / sent to the API
	Kind   string `json:"kind"`   // mx, spf, dkim, dmarc, a, ptr
	Source string `json:"source"` // "mail", …
}

// Record is a Need plus the live lookup result and whether GoDeb owns the
// matching Hetzner RRSet.
type Record struct {
	Need
	ZoneLine string `json:"zoneLine"`
	Status   string `json:"status"` // ok, missing, wrong, unknown, conflict
	Found    string `json:"found,omitempty"`
	Managed  bool   `json:"managed"` // Hetzner RRSet carries the godeb label
}

var domainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

// NormalizeDomain lowercases and checks an apex domain.
func NormalizeDomain(v string) (string, error) {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
	if len(d) > 253 || !domainPattern.MatchString(d) {
		return "", errors.New("Domain ist ungültig")
	}
	return d, nil
}

func normalizeMode(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", ModeManual:
		return ModeManual, nil
	case ModeHetzner:
		return ModeHetzner, nil
	default:
		return "", errors.New("Modus ist ungültig (manual oder hetzner)")
	}
}

// relativeName turns an FQDN into the RRSet name Hetzner expects (@ for apex).
func relativeName(fqdn, zone string) string {
	fqdn = strings.TrimSuffix(strings.ToLower(fqdn), ".")
	zone = strings.TrimSuffix(strings.ToLower(zone), ".")
	if fqdn == zone {
		return "@"
	}
	suf := "." + zone
	if strings.HasSuffix(fqdn, suf) {
		return strings.TrimSuffix(fqdn, suf)
	}
	return fqdn
}
