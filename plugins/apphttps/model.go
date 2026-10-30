// Package apphttps gives the web UI a real HTTPS address inside the tunnel
// (for example https://godeb.example.com:8443 -> wg0 address), so phones
// can install it as an app. The agent publishes the A record and gets a
// Let's Encrypt certificate by DNS-01 through the Hetzner token of the
// domains plugin; the web process serves HTTPS on wg0 only.
package apphttps

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"godeb/backend/state"
)

// Port is the HTTPS port on wg0. 443 would need a capability for the web
// user and collides with nginx on hubs that host websites.
const Port = 8443

const iface = "wg0"

var labelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// hostFor joins label and zone and checks that zone is one of zones.
func hostFor(label, zone string, zones []string) (string, error) {
	label = strings.ToLower(strings.TrimSpace(label))
	zone = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zone)), ".")
	if !labelPattern.MatchString(label) {
		return "", errors.New("Name ist ungültig (Buchstaben, Ziffern, Bindestrich)")
	}
	for _, z := range zones {
		if z == zone {
			host := label + "." + zone
			if len(host) > 253 {
				return "", errors.New("Name ist zu lang")
			}
			return host, nil
		}
	}
	return "", fmt.Errorf("Domain %q ist nicht im Hetzner-Modus eingetragen", zone)
}

// tunnelIPv4 is the first IPv4 address of wg0. Tests replace it.
var tunnelIPv4 = func() (string, error) {
	ifc, err := net.InterfaceByName(iface)
	if err != nil {
		return "", errors.New("Kein WireGuard-Tunnel (wg0) auf diesem Server")
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return "", err
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
			return n.IP.String(), nil
		}
	}
	return "", errors.New("wg0 hat keine IPv4-Adresse")
}

// Listen is what the agent hands to the web process in the state
// directory: whether to serve, under which name, and on which port.
type Listen struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

func dir() string { return filepath.Join(state.Dir(), "apphttps") }

// ConfigPath, CertPath and KeyPath are the files the web process reads.
func ConfigPath() string { return filepath.Join(dir(), "listen.json") }
func CertPath() string   { return filepath.Join(dir(), "cert.pem") }
func KeyPath() string    { return filepath.Join(dir(), "key.pem") }

// LoadListen reads the listen file; the web process calls it. A missing or
// broken file means off.
func LoadListen() Listen {
	raw, err := os.ReadFile(ConfigPath())
	if err != nil {
		return Listen{}
	}
	var l Listen
	if json.Unmarshal(raw, &l) != nil || l.Port < 1024 || l.Port > 65535 {
		return Listen{}
	}
	return l
}

// writeFiles hands certificate, key and listen file to the web process.
// Root writes into the state directory only through state.WriteFile.
func writeFiles(l Listen, certPEM, keyPEM []byte) error {
	if l.Enabled {
		if err := state.WriteFile(KeyPath(), keyPEM, 0o600); err != nil {
			return err
		}
		if err := state.WriteFile(CertPath(), certPEM, 0o600); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFile(ConfigPath(), append(raw, '\n'), 0o600)
}
