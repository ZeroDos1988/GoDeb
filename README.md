# GoDeb

**Schlanke, modulare Admin-Oberfläche für Debian 13 (Trixie).**
Ein einziges statisches Go-Programm, eine React-Oberfläche und Plugins für die Dinge, die ein kleiner Server wirklich braucht: Updates, Firewall, Dienste, Zertifikate, WireGuard, Portweiterleitung, Spielserver und ein Mail-Relay.

[![CI](https://github.com/ZeroDos1988/GoDeb/actions/workflows/ci.yml/badge.svg)](https://github.com/ZeroDos1988/GoDeb/actions/workflows/ci.yml)

- **Nicht im Internet.** Die Oberfläche hört nur auf `127.0.0.1` und im WireGuard-Tunnel. Kein Klartext-HTTP auf LAN oder WAN.
- **Weboberfläche ohne root.** Was mit dem Netz spricht, läuft als eigener Benutzer in einer engen Sandbox. Änderungen am System macht ein kleiner root-Agent, der jeden 2FA-Code selbst prüft.
- **2FA ist Pflicht.** TOTP bei der Anmeldung, dazu eine frische Bestätigung vor kritischen Aktionen (Firewall, Dienste, Peers, Plugins).
- **Das Betriebssystem bleibt die Wahrheit.** GoDeb liest und schreibt `wg0.conf`, `main.cf`, systemd und apt direkt. SQLite hält nur Metadaten.
- **Ein Paket für alle Rollen.** Derselbe `.deb` wird zum WireGuard-Hub, zum Peer oder bleibt eigenständig, je nachdem, was im Einrichtungsassistenten gewählt wird.
- **Läuft auf kleinen Maschinen.** Ein Binary ohne CGO, ein paar MB RAM, keine Laufzeit wie Node oder Python auf dem Server.

---

## Zielbild

GoDeb wurde für dieses Szenario gebaut: ein kleiner VPS mit öffentlicher IP und ein kräftiger Rechner zu Hause, verbunden über WireGuard. Auf beiden läuft dasselbe Paket.

```text
                     Internet
                        |  51820/udp WireGuard, 15636-15637/udp Spiel
                        v
        +-------------------------------+
        | VPS (z. B. Hetzner, 2 GB)     |
        | GoDeb als Hub   10.8.0.1      |
        | Portweiterleitung DNAT/SNAT   |
        +---------------+---------------+
                        | WireGuard (Heim-VM wählt sich aus, kein Router-Portforwarding,
                        |            funktioniert hinter CGNAT / DS-Lite)
        +---------------+---------------+
        | Heim-VM (z. B. Proxmox)       |
        | GoDeb als Peer  10.8.0.2      |
        | Spielserver (Enshrouded)      |
        +-------------------------------+

  Handy / Laptop  --WireGuard-->  Hub  -->  beide Oberflächen im Tunnel
```

Die Android-App trennt den Tunnel automatisch, sobald das Handy im Heim-WLAN ist, und erkennt das an einem **signierten Trusted Endpoint** der Heim-VM, nicht am WLAN-Namen.

## Plugins

Jedes Plugin lässt sich im Plugin-Manager abschalten. Es verschwindet dann aus der Seitenleiste, und seine API antwortet mit 404.

| Plugin | Was es tut |
| --- | --- |
| **APT** | Aktualisierbare Pakete anzeigen und einspielen |
| **UFW** | Firewall-Status, Regeln anlegen und löschen |
| **Fail2ban** | Jails, gesperrte IPs, Entsperren |
| **Systemd** | Dienste auflisten, starten, stoppen, neu starten |
| **Certbot** | Let's-Encrypt-Zertifikate unter `/etc/letsencrypt` |
| **WireGuard** | Hub oder Peer, Clients mit QR-Code, Standorte (`godeb site add`), Trusted Endpoint |
| **Portweiterleitung** | Öffentliche Ports des Hubs an Standorte im Tunnel weiterleiten (eigene nftables-Tabelle) |
| **Spielserver** | Dedizierte Server über SteamCMD, ein Konto und eine systemd-Unit pro Spiel. Als erstes Spiel: Enshrouded |
| **Mail-Gateway** | Postfix-Relay über WireGuard mit OpenDKIM, Queue und Statistik |

Ein neues Spiel ist eine `Game`-Beschreibung in `plugins/steam/`, kein eigenes Plugin.

## Installation

Voraussetzung: Debian 13 (amd64), ein Konto mit `sudo`, eine Netzwerkverbindung. GoDeb kommt aus einem eigenen, signierten APT-Repo auf GitHub Pages:

```bash
sudo wget -O /usr/share/keyrings/godeb-archive-keyring.asc https://zerodos1988.github.io/GoDeb/godeb-archive-keyring.asc
sudo wget -O /etc/apt/sources.list.d/godeb.sources https://zerodos1988.github.io/GoDeb/godeb.sources
sudo apt update
sudo apt install --no-install-recommends godeb
```

Alle Befehle in dieser Anleitung laufen mit `sudo`. Wer direkt als root angemeldet ist (etwa auf einem frischen Hetzner-Server), lässt `sudo` weg. Fehlt `sudo` (Debian, bei dessen Installation ein root-Passwort gesetzt wurde), einmal als root: `apt install sudo && usermod -aG sudo <benutzer>`, dann neu anmelden.

Der Schlüssel hat den Fingerprint `85BB 4B5E C4EB BC47 DCD1  6D32 FF4E 70E4 D1A7 EBB9` (prüfen mit `sq inspect /usr/share/keyrings/godeb-archive-keyring.asc`). Ab dann kommt jede neue Version mit `sudo apt update && sudo apt upgrade`. Ohne Repo geht es weiter mit dem `.deb` aus den [Releases](https://github.com/ZeroDos1988/GoDeb/releases): `sudo apt install --no-install-recommends ./godeb_*_amd64.deb`. Ab Version 0.2.3 bringt das Paket Repo-Eintrag und Schlüssel selbst mit.

Am Ende der Installation steht in einem Kasten der Link zur Ersteinrichtung. Später lässt er sich jederzeit wieder abrufen:

```bash
sudo godeb setup-url
```

### Ersteinrichtung im Browser

Ein frisches GoDeb richtet nichts von selbst ein. Bis zum Abschluss öffnet es einen HTTPS-Assistenten auf Port `8443` (selbstsigniertes Zertifikat, Fingerprint steht neben dem Link, Zugang nur mit Einmal-Token):

1. **Admin-Konto** mit Passwort anlegen
2. **2FA** einrichten (QR-Code in einer Authenticator-App scannen)
3. **Rolle** wählen: WireGuard-Hub, Peer eines vorhandenen Hubs oder eigenständig
4. **Erreichbarkeit** festlegen: nur im Tunnel, zusätzlich per HTTPS im LAN, oder nur lokal
5. **Übersicht**: wie der Server jetzt eingerichtet ist (Adresse der Oberfläche im Tunnel, Tunnel, Firewall), zum Drucken oder als PDF. Später: Startseite → Einrichtungsübersicht oder `sudo godeb summary`

Für eine Heim-VM als Peer: auf dem Hub unter WireGuard einen Peer mit „Standort“ anlegen, die `.conf` wird heruntergeladen; im Assistenten der Heim-VM „Mit Hub verbinden“ wählen und die Datei hochladen.

Danach schließt sich der Assistent, und ufw lässt nur noch SSH, WireGuard (auf dem Hub) und die gewählten Zugänge durch.

Ohne Browser geht dasselbe auf der Kommandozeile, mit denselben Funktionen im Hintergrund:

```bash
sudo godeb setup --role hub --access tunnel            # VPS
sudo godeb site add heim-vm --address 10.8.0.2         # auf dem Hub: Standort anlegen
sudo godeb setup --join ~/heim-vm.conf --access lan    # auf der Heim-VM
```

Die vollständige Anleitung für VPS und Heim-VM, inklusive Portweiterleitung und Spielserver, steht in [DOCUMENTATION.md](DOCUMENTATION.md#zielbild-vps-und-heim-vm).

### Updates und Backups

Jeder Build trägt eine Version aus dem Git-Tag (`0.1.0` oder `0.1.0+git42.abc1234`), apt erkennt neuere Pakete als Update. Mit dem APT-Repo reicht `sudo apt update && sudo apt upgrade`. Die Version steht in der Seitenleiste, in `godeb version` und in `/api/health`.

Backups macht GoDeb bewusst nicht selbst. Dafür sind die Snapshots der Plattform da (Proxmox zu Hause, Hetzner-Backups auf dem VPS).

## Sicherheit

| Schutz | Umsetzung |
| --- | --- |
| Netz | Nur Loopback und `wg0`. Öffentlich nur der Einrichtungsassistent per HTTPS mit Token, LAN nur per HTTPS und nur aus dem eigenen Subnetz |
| Anmeldung | Passwort plus TOTP, Versuchslimit, serverseitige Sitzungen, optional Microsoft 365 oder Google per OIDC |
| Kritische Aktionen | Frischer TOTP-Code (Step-up) vor Änderungen an Firewall, Diensten, Peers, Plugins und Spielserver-Installationen |
| Befehle | Kein `sh -c`, Argumente als Liste, jede Eingabe gegen enge Muster geprüft, Secrets über stdin |
| Dateien | Pfad-Allowlist, Secrets mit `0600`, sensible DB-Felder mit AES-256-GCM verschlüsselt |
| Browser | CSP, Origin-Prüfung, `SameSite`-Cookies |
| Rechtetrennung | Weboberfläche als Benutzer `godeb` mit `ProtectSystem=strict` und ohne Capabilities; root-Agent nur über einen Unix-Socket mit Peer-Prüfung |
| 2FA beim Agenten | TOTP-Geheimnisse liegen nur beim root-Agenten, der jede Änderung selbst mit Code prüft. Eine übernommene Weboberfläche kann lesen, aber nichts ändern |
| Lieferkette | `govulncheck` und `npm audit` in der CI, `systemd-analyze security` als Schwelle (Weboberfläche 1.3) |

Notfall-Befehle wie `godeb user passwd`, `godeb 2fa reset` und `godeb sessions revoke` gibt es nur auf der Kommandozeile mit root-Rechten (`sudo godeb …`).

Sicherheitslücken bitte vertraulich melden, siehe [SECURITY.md](SECURITY.md).

## Architektur

```text
Browser ──wg0 / 127.0.0.1:8080──▶ godeb.service        Benutzer godeb, Sandbox
                                   ├─ Anmeldung, Sitzungen, React-Build
                                   ├─ SQLite /var/lib/godeb/godeb.db (Konten, Sitzungen)
                                   │
                                   │  Unix-Socket /run/godeb-agent/agent.sock
                                   ▼
                                  godeb-agent.service  root, kein Netzwerk
                                   ├─ Plugin-Routen /api/plugins/<id>/…, Plugin-Manager
                                   ├─ 2FA-Geheimnisse, eigene Code-Prüfung
                                   ├─ SQLite /etc/godeb/godeb.db (Plugin-Daten)
                                   └─ systemd · apt · ufw · wg · nft · certbot · fail2ban · postfix
```

Ein Binary (Go, statisch, `CGO_ENABLED=0`), zwei Dienste. Ohne `GODEB_AGENT_SOCKET` läuft alles in einem Prozess (Entwicklung, Tests).

| Verzeichnis | Inhalt |
| --- | --- |
| `backend/` | Weboberfläche, root-Agent (`agent`), CLI, Anmeldung, Ersteinrichtung (`firstrun`), Plugin-Manager |
| `plugins/<id>/` | Ein Fachgebiet pro Verzeichnis: Go-Paket, `manifest.json`, `plugin.json`, optional `ui/Panel.tsx` |
| `frontend/` | Oberfläche, ab 360 px Breite bedienbar, Deutsch und Englisch |
| `package/debian/` | Paketstruktur, systemd-Units, `postinst` |
| `scripts/` | `build-deb.sh`, `version.sh` |

## Entwicklung

```bash
go test ./...                         # Backend
cd frontend && npm ci && npm run build   # Oberfläche inkl. Typecheck
scripts/build-deb.sh                  # Paket bauen
```

Lokal ohne root starten (fehlende Werkzeuge liefern Beispieldaten mit `mocked: true`):

```bash
GODEB_DB=$PWD/.dev/godeb.db GODEB_AUTH_DIR=$PWD/.dev/auth \
GODEB_AUTH_LOG=$PWD/.dev/auth.log GODEB_MASTER_KEY=$PWD/.dev/master.key \
GODEB_ETC=$PWD/.dev/etc go run ./backend
```

Regeln für Beiträge (Sicherheit, Stil, Commits) stehen in [CLAUDE.md](CLAUDE.md).

## Begleit-Apps

WireGuard-Clients mit GoDeb-Erweiterungen (Trusted Endpoint, Auto-Disconnect):

- [godeb-tunnel-android](https://github.com/ZeroDos1988/godeb-tunnel-android): APK unter Releases; GoDeb liefert sie auch selbst unter `/api/plugins/wireguard/android/apk` aus, wenn sie unter `/etc/godeb/android/godeb-tunnel.apk` liegt
- [godeb-tunnel-arch](https://github.com/ZeroDos1988/godeb-tunnel-arch)
- Windows: noch nicht öffentlich

Die Apps hießen früher „GoDeb WireGuard“. Die Markenrichtlinie von WireGuard erlaubt den Namen nicht in Produktnamen. „WireGuard“ ist eine eingetragene Marke von Jason A. Donenfeld; GoDeb ist ein unabhängiges Projekt und nutzt die WireGuard-Werkzeuge von Debian.

## Weiterlesen

- [DOCUMENTATION.md](DOCUMENTATION.md): Architektur, Einrichtung, API, Sicherheitskonzept, Wartung
- [ROADMAP.md](ROADMAP.md): was fertig ist und was als Nächstes kommt
- [SECURITY.md](SECURITY.md): Sicherheitslücken melden

## Status

GoDeb ist eine Vorabversion (`0.x`): in der CI und auf ersten Servern getestet, aber noch jung. Vor Installation und Updates einen Snapshot der Maschine ziehen.

## Lizenz

GoDeb steht unter der [GNU General Public License v3.0](LICENSE). Du darfst es nutzen, verändern und weitergeben; wer veränderte Versionen weitergibt, muss deren Quellcode unter derselben Lizenz offenlegen.

Copyright (C) 2026 ZeroDos1988
