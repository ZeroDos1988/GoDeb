package sites

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"godeb/plugins/wireguard"
)

// Public keys and source files of the APT repositories. They are copies of
// package/apt and the conffiles of both packages (godeb-play from its own
// repository); TestEmbeddedKeysMatchPackage keeps the GoDeb pair in sync.
var (
	//go:embed keys/godeb-archive-keyring.asc
	godebKey string
	//go:embed keys/godeb.sources
	godebSources string
	//go:embed keys/godeb-play-archive-keyring.asc
	playKey string
	//go:embed keys/godeb-play.sources
	playSources string
)

// product is what a profile installs.
type product struct {
	Package string // Debian package and program name
	Title   string
	Key     string // armored public key of the repository
	KeyFile string // under /usr/share/keyrings
	Sources string // deb822 file, identical to the package's conffile
	SrcFile string // under /etc/apt/sources.list.d
}

func productFor(kind string) (product, error) {
	switch kind {
	case wireguard.ProfileHome:
		return product{"godeb", "Heim-Server (GoDeb)", godebKey, "godeb-archive-keyring.asc", godebSources, "godeb.sources"}, nil
	case wireguard.ProfilePlay:
		return product{"godeb-play", "GoDeb Play", playKey, "godeb-play-archive-keyring.asc", playSources, "godeb-play.sources"}, nil
	}
	return product{}, fmt.Errorf("Profil %q ist unbekannt", kind)
}

// ScriptName is the file name the admin copies to the new host.
func ScriptName(kind, name string) string {
	return "godeb-" + kind + "-" + name + ".sh"
}

// Script renders the setup script for a new site. It carries the join code
// (with the site's private WireGuard key), so the hub hands it out once and
// the script deletes itself at the end. Everything it writes is fixed text
// from this hub: the repository key and sources, the join code. The only
// questions on the new host are the administrator's name and password.
func Script(kind, name, hub, joinCode string, created time.Time) (string, error) {
	p, err := productFor(kind)
	if err != nil {
		return "", err
	}
	if !wireguard.CompleteJoinCode(joinCode) {
		return "", errors.New("Beitrittscode ist unvollständig")
	}
	for _, part := range []string{p.Key, p.Sources, joinCode} {
		// The parts go into quoted here-documents; a line equal to a
		// terminator would end them early.
		for _, line := range strings.Split(part, "\n") {
			if line == "GODEB_KEY" || line == "GODEB_SOURCES" || line == "GODEB_JOIN" {
				return "", errors.New("unerwarteter Inhalt im Skript")
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, `#!/bin/sh
# %s %q an den GoDeb-Hub %s anbinden.
# Erzeugt vom Hub am %s.
#
# Ausführen auf dem neuen Rechner (Debian 13, frisch installiert), mit einem
# Konto mit sudo:
#
#     sudo sh %s
#
# Das Skript bindet das APT-Repository ein, installiert %s, verbindet sich
# über WireGuard mit dem Hub und richtet alles ein, was der Hub vorgibt.
# Gefragt werden nur Benutzername und Passwort des Administrators; die
# Zwei-Faktor-Anmeldung richtest du beim ersten Login im Browser ein.
#
# ACHTUNG: Die Datei enthält den privaten WireGuard-Schlüssel dieses
# Standorts. Nicht weitergeben. Nach dem Lauf löscht sie sich selbst.
set -eu
umask 077

fail() { echo "FEHLER: $*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || fail "bitte mit sudo starten: sudo sh $0"
. /etc/os-release
[ "${VERSION_CODENAME:-}" = trixie ] || fail "nur für Debian 13 (trixie), gefunden: ${PRETTY_NAME:-unbekannt}"
[ ! -e /etc/wireguard/wg0.conf ] || fail "/etc/wireguard/wg0.conf existiert schon; dieser Rechner hat bereits einen Tunnel"
self=$(readlink -f "$0")

echo "==> APT-Repository einbinden"
cat >/usr/share/keyrings/%[7]s <<'GODEB_KEY'
%[8]sGODEB_KEY
chmod 0644 /usr/share/keyrings/%[7]s
cat >/etc/apt/sources.list.d/%[9]s <<'GODEB_SOURCES'
%[10]sGODEB_SOURCES
chmod 0644 /etc/apt/sources.list.d/%[9]s

echo "==> %[6]s installieren"
apt-get update
apt-get install -y --no-install-recommends %[6]s

echo "==> Mit dem Hub verbinden und einrichten"
join=$(mktemp)
trap 'rm -f "$join"' EXIT
cat >"$join" <<'GODEB_JOIN'
%[11]s
GODEB_JOIN
%[6]s setup --join "$join" </dev/tty

rm -f "$self"
echo "Fertig. Diese Datei wurde gelöscht."
`, p.Title, name, hub, created.Format("02.01.2006 15:04"), ScriptName(kind, name), p.Package,
		p.KeyFile, withNewline(p.Key), p.SrcFile, withNewline(p.Sources), strings.TrimSpace(joinCode))
	return b.String(), nil
}

func withNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
