package domains

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// HetznerZones lists the zones GoDeb may change through the Cloud API and
// whether a token is stored. Other plugins (apphttps) offer only these.
func HetznerZones() ([]string, bool, error) {
	s, _, err := loadSettings()
	if err != nil {
		return nil, false, err
	}
	list, err := listDomains()
	if err != nil {
		return nil, false, err
	}
	zones := []string{}
	for _, d := range list {
		if d.Mode == ModeHetzner {
			zones = append(zones, d.Name)
		}
	}
	return zones, s.TokenSet, nil
}

// hetznerFor returns the token and Hetzner zone for host, which must lie in
// a zone set to Hetzner.
func hetznerFor(ctx context.Context, host string) (string, hzZone, int64, error) {
	zones, _, err := HetznerZones()
	if err != nil {
		return "", hzZone{}, 0, err
	}
	zone := ""
	for _, z := range zones {
		if (host == z || strings.HasSuffix(host, "."+z)) && len(z) > len(zone) {
			zone = z
		}
	}
	if zone == "" {
		return "", hzZone{}, 0, fmt.Errorf("%s liegt in keiner Domain mit Hetzner-Modus", host)
	}
	s, token, err := loadSettings()
	if err != nil {
		return "", hzZone{}, 0, err
	}
	if token == "" {
		return "", hzZone{}, 0, errors.New("Kein Hetzner-API-Token hinterlegt")
	}
	z, err := findZone(ctx, token, zone)
	return token, z, s.ServerID, err
}

// Publish creates or updates one GoDeb-owned RRSet right away (the A record
// of the app address). An RRSet without the godeb label is a conflict.
func Publish(ctx context.Context, n Need) error {
	token, zone, serverID, err := hetznerFor(ctx, n.Name)
	if err != nil {
		return err
	}
	n.Zone = zone.Name
	recs, err := applyZone(ctx, token, zone.Name, []Need{n}, serverID)
	if err != nil {
		return err
	}
	for _, r := range recs {
		switch r.Status {
		case "ok":
		case "conflict":
			return fmt.Errorf("%s %s gibt es schon ohne GoDeb-Kennzeichnung (%s); bitte bei Hetzner entfernen", r.Name, r.Type, r.Found)
		default:
			return fmt.Errorf("%s %s: %s", r.Name, r.Type, r.Found)
		}
	}
	return nil
}

// SetACMEChallenge publishes the DNS-01 TXT record for host and returns the
// zone (for the propagation check) and a cleanup that removes the RRSet.
func SetACMEChallenge(ctx context.Context, host, value string) (string, func(context.Context), error) {
	token, zone, _, err := hetznerFor(ctx, host)
	if err != nil {
		return "", nil, err
	}
	rel := relativeName("_acme-challenge."+host, zone.Name)
	txt := splitTXT(value)
	cur, exists, err := getRRSet(ctx, token, zone, rel, "TXT")
	if err != nil {
		return "", nil, err
	}
	switch {
	case exists && !isManaged(cur):
		return "", nil, fmt.Errorf("_acme-challenge.%s TXT gibt es schon ohne GoDeb-Kennzeichnung; bitte bei Hetzner entfernen", host)
	case exists:
		err = setRecords(ctx, token, zone, rel, "TXT", txt)
	default:
		err = createRRSetTTL(ctx, token, zone, rel, "TXT", txt, 60)
	}
	if err != nil {
		return "", nil, err
	}
	cleanup := func(ctx context.Context) {
		_, _, _ = hzRequest(ctx, token, http.MethodDelete, rrsetPath(zone, rel, "TXT"), nil)
	}
	return zone.Name, cleanup, nil
}
