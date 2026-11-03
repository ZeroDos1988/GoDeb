package domains

import (
	"context"
	"errors"
	"strings"
)

var errNotFound = errors.New("Domain nicht gefunden")

// SetToken checks a Hetzner Cloud API token against the API and stores it
// sealed in the agent database. The plugin view and the setup wizard use it.
func SetToken(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if !tokenOK(token) {
		return errors.New("Token ist ungültig")
	}
	if err := validateToken(ctx, token); err != nil {
		return err
	}
	return upsertSettings(&token, nil)
}

// TokenSet reports whether a Hetzner token is stored.
func TokenSet() bool {
	s, _, err := loadSettings()
	return err == nil && s.TokenSet
}

// Ensure adds a domain or updates mode and default records of an existing
// one. mode is ModeManual or ModeHetzner.
func Ensure(name, mode string, defaults bool) (Domain, error) {
	name, err := NormalizeDomain(name)
	if err != nil {
		return Domain{}, err
	}
	if mode, err = normalizeMode(mode); err != nil {
		return Domain{}, err
	}
	_, ok, err := getDomain(name)
	if err != nil {
		return Domain{}, err
	}
	if ok {
		err = updateDomain(name, mode, defaults)
	} else {
		err = addDomain(name, mode, defaults)
	}
	if err != nil {
		return Domain{}, err
	}
	d, _, err := getDomain(name)
	return d, err
}

// Apply creates or updates the zone's records at Hetzner: everything the
// plugins need plus the default records when they are switched on.
func Apply(ctx context.Context, name string) ([]Record, error) {
	d, ok, err := getDomain(name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errNotFound
	}
	if d.Mode != ModeHetzner {
		return nil, errors.New("Domain ist nicht auf Hetzner gestellt")
	}
	s, token, err := loadSettings()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errors.New("Kein Hetzner-API-Token hinterlegt")
	}
	needs, err := collectNeeds(name)
	if err != nil {
		return nil, err
	}
	if len(needs) == 0 {
		return nil, errors.New("Keine Einträge von Plugins für diese Domain")
	}
	return applyZone(ctx, token, name, needs, s.ServerID)
}

// Records returns the zone's records with their state in public DNS, for
// domains kept by hand.
func Records(ctx context.Context, name string) ([]Record, error) {
	needs, err := collectNeeds(name)
	if err != nil {
		return nil, err
	}
	return recordsFor(ctx, needs), nil
}
