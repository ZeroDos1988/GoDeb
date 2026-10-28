package postgres

import (
	"errors"
	"regexp"
	"strings"
)

var identPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func cleanIdent(v, kind string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if !identPattern.MatchString(v) {
		return "", errors.New(kind + ": Kleinbuchstaben, Ziffern und Unterstrich, max. 63")
	}
	switch v {
	case "postgres", "template0", "template1", "public", "pg_catalog", "information_schema":
		return "", errors.New(kind + " ist reserviert")
	}
	return v, nil
}

func cleanPassword(p string) error {
	p = strings.TrimSpace(p)
	if len(p) < 10 || len(p) > 128 {
		return errors.New("Passwort: 10–128 Zeichen")
	}
	for _, r := range p {
		if r < 32 || r == 127 {
			return errors.New("Passwort enthält Steuerzeichen")
		}
	}
	return nil
}
