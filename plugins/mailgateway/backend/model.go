package mailgateway

import (
	"errors"
	"net"
	"regexp"
	"strings"
)

const defaultRelay = "10.8.0.2"

var (
	domainPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	selectorPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,31})?$`)
	errDomain       = errors.New("Domain ist ungültig")
	errRelay        = errors.New("Relay-Ziel muss eine IPv4-Adresse im WireGuard-Netz 10.8.0.0/24 sein")
	errSelector     = errors.New("DKIM-Selector ist ungültig")
)

func normalizeDomain(value string) (string, error) {
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	if len(domain) > 253 || !domainPattern.MatchString(domain) {
		return "", errDomain
	}
	return domain, nil
}

func normalizeSelector(value string) (string, error) {
	selector := strings.ToLower(strings.TrimSpace(value))
	if selector == "" {
		selector = "godeb"
	}
	if !selectorPattern.MatchString(selector) {
		return "", errSelector
	}
	return selector, nil
}

func normalizeRelay(value string) (string, error) {
	host := strings.TrimSpace(value)
	if host == "" {
		host = defaultRelay
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		return "", errRelay
	}
	v4 := ip.To4()
	if v4[0] != 10 || v4[1] != 8 || v4[2] != 0 {
		return "", errRelay
	}
	return v4.String(), nil
}
