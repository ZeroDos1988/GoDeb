# GoDeb

GoDeb ist eine lokale Verwaltungsoberfläche für Debian 13 (Trixie). Ein statisch gebautes Go-Programm spricht mit dem Betriebssystem, eine React-Oberfläche zeigt den Zustand, und eingebettete SQLite-Dateien halten nur Metadaten. Laufende Dienste bleiben in ihren eigenen Dateien unter `/etc`. Dasselbe Programm läuft als zwei Dienste: die Weboberfläche als unprivilegierter Benutzer `godeb` und ein kleiner root-Agent ohne Netzwerkzugang, der alle Änderungen am System ausführt.

Die Oberfläche hört auf `127.0.0.1:8080` und auf der Adresse von `wg0` (üblich `10.8.0.1:8080`). Auf Wunsch kommt HTTPS auf der LAN-Adresse dazu (`<lan-ip>:8443`, nur für das LAN-Subnetz). Öffentlich erreichbar ist sie nur während der Ersteinrichtung per HTTPS mit Einmal-Token, nie als dauerhaftes Portal und nie per Klartext-HTTP. Sie ist ab 360 px Breite bedienbar, also auch vom Handy über den Tunnel.

## Architektur

```text
Browser --wg0 10.8.0.1:8080 / 127.0.0.1:8080-->  godeb.service  (Benutzer godeb, ohne Rechte)
                                 |-- Anmeldung, Sitzungen, Listener, React-Build
                                 |-- SQLite  /var/lib/godeb/godeb.db  (Konten, Sitzungen, OIDC)
                                 |-- Trusted Endpoint (LAN), signiert vom Agenten
                                 |
                                 |  HTTP über /run/godeb-agent/agent.sock
                                 |  (0660 root:godeb, Peer-Prüfung SO_PEERCRED)
                                 v
                           godeb-agent.service  (root, kein Netzwerk-Listener)
                                 |-- Plugin-Routen, Plugin-Manager, Setup-Schritte
                                 |-- 2FA-Geheimnisse und eigene 2FA-Prüfung
                                 |-- SQLite  /etc/godeb/godeb.db  (WireGuard-IPAM, Mail, Plugin-Status, 2FA)
                                 '-- systemd, apt, ufw, wg, nft, certbot, fail2ban, postfix, SteamCMD
```

| Teil | Ort | Rolle |
| --- | --- | --- |
| Weboberfläche | `backend/` (`godeb`) | HTTP, Anmeldung, Sitzungen, statische Dateien, leitet an den Agenten weiter |
| Agent | `backend/agent`, `godeb agent` | Führt Plugins und Setup-Schritte als root aus, prüft 2FA selbst |
| Plugins | `plugins/<id>/` | Ein Fachgebiet pro Verzeichnis, Registrierung in `init()` |
| Oberfläche | `frontend/` | Vite, React 19, Tailwind 4, Hash-Routen |
| Paket | `package/debian/` | Struktur für `dpkg-deb` |
| Web-Daten | `/var/lib/godeb/` (Benutzer `godeb`) | `godeb.db` (Konten, Sitzungen, Themes, OIDC), `master.key`, `auth/`, `setup.json`, `setup-token`, `access.json`, `tls/` |
| Agent-Daten | `/etc/godeb/` (nur root) | `godeb.db` (WireGuard-IPAM, Mail-Relay, Plugin-Status, 2FA), `master.key` (versiegelt 2FA und Plugin-Secrets wie Mail-Passwort-Hashes und den Hetzner-Token), `trust.*`, `portforward.*`, `smtp.json`, `android/` |

### Aufteilung Weboberfläche und Agent

Die Weboberfläche ist der Teil, der mit dem Netz spricht. Sie läuft als `godeb` mit `ProtectSystem=strict`, ohne Capabilities und mit Systemaufruf-Filter (`systemd-analyze security`: 1.3). Alles, was das System ändert, erledigt `godeb-agent.service` (root). Der Agent hat keinen Netzwerk-Listener, nur den Unix-Socket. Er nimmt dort nur Verbindungen von root und `godeb` an (`SO_PEERCRED`).

- **Weitergeleitet** werden `/api/plugins/*`, `/api/plugin-manager*` und die Setup-Schritte `info`, `hub`, `client`, `join`, `finish`, `summary`. Die Weboberfläche prüft vorher die Sitzung und schickt Benutzer und einen Hash der Sitzungs-ID mit, nie das Cookie.
- **2FA beim Agenten (Weg 2+):** TOTP-Geheimnisse und Backup-Codes liegen nur in der Agent-Datenbank, versiegelt mit `/etc/godeb/master.key`. Die Weboberfläche fragt Status, Einrichtung und Codes beim Agenten an. Für weitergeleitete Anfragen prüft der Agent den Code im Header `X-GoDeb-TOTP` selbst, mit eigenem Fenster pro Sitzung, fünf Fehlversuchen pro Sitzung und einer Sperre nach zehn falschen Codes pro Benutzer in 15 Minuten.
- **Neue 2FA-Einrichtungen** nimmt der Agent über den Socket nur für das erste Konto an oder wenn root sie freigegeben hat (`godeb user passwd`, `godeb 2fa reset`). Einen Reset über den Socket gibt es nicht.
- **Folge:** Eine kompromittierte Weboberfläche kann lesen, aber ohne aktuellen Code des Admins nichts am System ändern, keine eigenen Konten mit 2FA ausstatten und keine Schlüssel stehlen. Auch der Trusted-Endpoint-Schlüssel bleibt beim Agenten; die Weboberfläche lässt nur signieren.
- **Ohne `GODEB_AGENT_SOCKET`** läuft alles in einem Prozess, wie bei der Entwicklung und in den Tests. Die Befehle auf der Kommandozeile (`godeb setup`, `site`, `trust`, `user`, `2fa`) laufen als root ebenfalls direkt.

`backend/modules.go` importiert jedes Plugin leer, damit `init()` die Routen bei `core.Register` anmeldet. `core.Load` liest `plugins/<id>/manifest.json` und hängt den Handler nur ein, wenn Manifest und Registrierung zusammenpassen. Die Oberfläche lädt `plugins/*/ui/Panel.tsx` über `import.meta.glob`.

SQLite (`modernc.org/sqlite`, ohne CGO) ersetzt keine Live-Konfiguration. Postfix, OpenDKIM, WireGuard, UFW, systemd und apt bleiben die Quelle für den laufenden Zustand. Die Datenbank merkt sich, was die Oberfläche wieder anzeigen und erneut anwenden muss.

Der Build erzwingt `CGO_ENABLED=0` und `-tags netgo`. Das Paket legt das Programm nach `/usr/bin/godeb` und die Dienste nach `/lib/systemd/system/godeb.service` und `godeb-agent.service`.

### Umgebungsvariablen

| Variable | Standard | Bedeutung |
| --- | --- | --- |
| `GODEB_ADDR` | `127.0.0.1:8080` | Kommagetrennte Lauschadressen. Ein Interface-Name (`wg0:8080`) steht für dessen IPv4-Adresse und wird gebunden, sobald das Interface steht. Der Dienst setzt `127.0.0.1:8080,wg0:8080`. |
| `GODEB_STEAM_ROOT` | `/var/lib/godeb-steam` | Basisverzeichnis der Spielserver, ein Unterordner pro Spiel (für Entwicklung ohne root umbiegen) |
| `GODEB_ROOT` | Verzeichnis mit `go.mod` | Wurzel für `frontend/dist` und `plugins/` |
| `GODEB_DB` | `/var/lib/godeb/godeb.db` | Datenbank der Weboberfläche, Modus `0600` |
| `GODEB_AGENT_DB` | `/etc/godeb/godeb.db` | Datenbank des Agenten; ohne die Variable, aber mit `GODEB_DB` dieselbe Datei (Entwicklung) |
| `GODEB_MASTER_KEY` | `/var/lib/godeb/master.key` | Schlüssel für Passwort-Hashes und OIDC-Geheimnisse |
| `GODEB_AGENT_MASTER_KEY` | `master.key` neben der Agent-Datenbank | Schlüssel für alle Secrets der Agent-Datenbank (2FA, `vault:"secret"`-Felder der Plugins). Bis v0.2.7 versiegelte der Agent Plugin-Secrets versehentlich mit dem Web-Schlüssel; beim Start schlüsselt er solche Werte einmalig um (`db.RegisterAgentSecret`) |
| `GODEB_AGENT_SOCKET` | leer | Socket des Agenten; gesetzt leitet die Weboberfläche weiter (Dienst: `/run/godeb-agent/agent.sock`) |
| `GODEB_WEB_USER` | `godeb` | Benutzer der Weboberfläche: darf an den Socket, bekommt die Dateien unter `/var/lib/godeb` |
| `GODEB_STATE_DIR` | `/var/lib/godeb` | Verzeichnis der Weboberfläche |
| `GODEB_AUTH_DIR` | `/var/lib/godeb/auth` | JWT-Schlüssel, Import alter Benutzerdateien |
| `GODEB_AUTH_LOG` | `/var/log/godeb/auth.log` | Fehlgeschlagene Anmeldungen |
| `GODEB_METRICS_DB` | `/var/lib/godeb-agent/metrics.db` | Verlauf der Überwachung (Agent, root). Ohne die Variable, aber mit `GODEB_STATE_DIR` dort `metrics.db` (Entwicklung) |
| `GODEB_LOG_LEVEL` | `info` | `debug`, `info`, `warn` oder `error`. `debug` zeigt zusätzlich jeden Befehl und jede lesende Anfrage |
| `GODEB_PUBLIC_URL` | leer | Öffentliche Basis für OIDC-Redirects |
| `GODEB_COOKIE_SECURE` | leer | `1` setzt das Sitzungscookie nur über HTTPS |
| `GODEB_SETUP_ADDR` | `0.0.0.0:8443` | HTTPS-Adresse der Ersteinrichtung, nur bis zum Abschluss offen |
| `GODEB_ETC` | `GODEB_STATE_DIR` | Älterer Name: Ort von `setup.json`, `setup-token`, `access.json` und `tls/` |

## Zielbild: VPS und Heim-VM

Betrieb mit zwei Standorten (umgesetzt am 20.10.2026, `ROADMAP.md` Phase 3b). Die Einrichtung Schritt für Schritt steht am Ende des Abschnitts.

Zwei Installationen mit **demselben GoDeb-Paket**. Sie unterscheiden sich nur in der Rolle beim Setup und in den Plugins, die im Plugin-Manager aktiviert sind:

```text
                Internet
                   |  51820/udp (WireGuard), 15636-15637/udp (Enshrouded), 25/tcp (Mail, optional)
                   v
   +---------------------------------------+
   | Hetzner-VPS  (klein, öffentliche IP)  |
   | GoDeb, wg0 10.8.0.1/24 = Hub          |
   | nft: DNAT 15636-15637/udp -> 10.8.0.2 |
   +---------------------------------------+
        ^ WireGuard, aufgebaut von zu Hause (PersistentKeepalive)
        |                                    \
   +----------------------------+        Handy, Laptop: Peers 10.8.0.10+
   | Heim-VM (mehr Leistung)    |        erreichen 10.8.0.1:8080 (VPS-UI)
   | GoDeb, wg0 = Peer 10.8.0.2 |        und 10.8.0.2:8080 (Heim-UI)
   | Enshrouded-Server          |
   +----------------------------+
```

| | Hetzner-VPS | Heim-VM |
| --- | --- | --- |
| Rolle | Öffentlicher Eingang und WireGuard-Hub | Rechenleistung, Spielserver |
| `wg0` | Server `10.8.0.1/24`, `ListenPort 51820` | Client des VPS, feste Adresse `10.8.0.2`, `AllowedIPs 10.8.0.0/24` |
| Plugins | wireguard, ufw, fail2ban, apt, systemd, certbot, Portweiterleitung (neu), optional Mail-Gateway | ufw, fail2ban, apt, systemd, enshrouded |
| Web-UI | `10.8.0.1:8080` | `10.8.0.2:8080` (über den Hub) |
| Von außen offen | `22/tcp`, `51820/udp`, `15636–15637/udp` (weitergeleitet) | nichts; der Tunnel geht von innen nach außen |

**Entscheidungen und Gründe:**

- **Ein Tunnelnetz, der VPS ist der Hub.** Die Heim-VM hat keinen eigenen WireGuard-Server. Ihr `wg0` ist ein Client des VPS, deshalb bindet GoDeb dort von selbst `wg0:8080`. Ein Handy-Profil reicht für beide Oberflächen.
- **Die Verbindung baut die Heim-VM auf** (`PersistentKeepalive = 25`). Zu Hause braucht es dann keine Portweiterleitung am Router und keine DynDNS-Adresse, und es funktioniert auch hinter DS-Lite oder CGNAT.
- **Die Heim-VM schickt nur das Tunnelnetz durch den Tunnel** (`AllowedIPs 10.8.0.0/24`, kein Volltunnel). Ihr normaler Internetverkehr, etwa Steam-Updates, bleibt am Heimanschluss.
- **Weiterleitung auf dem VPS mit DNAT und SNAT:** Pakete an `<vps-ip>:15636–15637/udp` gehen nach `10.8.0.2`. Mit SNAT auf `10.8.0.1` laufen die Antworten zurück durch den Tunnel statt über den Heimrouter. Der Preis: Enshrouded sieht alle Spieler mit der Adresse `10.8.0.1`. Spieler lassen sich deshalb nicht per IP sperren, nur über die Gruppen-Passwörter. Echte Spieler-IPs bräuchten Policy-Routing auf der Heim-VM. Das ist eine spätere Option.
- **Technik auf dem VPS:** `net.ipv4.ip_forward=1` (`/etc/sysctl.d/90-godeb-forward.conf`). DNAT und SNAT stehen in einer eigenen nftables-Tabelle `godeb_portforward` (`/etc/godeb/portforward.nft`, beim Booten geladen von `godeb-portforward.service`). Das ist bewusst nicht `/etc/ufw/before.rules`: ufw leert dort beim Reload die NAT-Regeln nicht, und sie würden sich verdoppeln. Die FORWARD-Kette bleibt bei ufw (`DEFAULT_FORWARD_POLICY=DROP`). Durch kommt nur, was eine `ufw route allow`-Regel genau für diese Ports zu `10.8.0.2` erlaubt. Zwischen Peers (Handy → Heim-UI) erlaubt der Hub nur `10.8.0.2:8080/tcp`.
- **Firewall der Heim-VM:** `godeb setup --join` erlaubt auf `wg0` nur `8080/tcp` (plus SSH), das Spielserver-Plugin öffnet die Spielports dort ebenfalls nur auf `wg0`. Wird der VPS übernommen, erreicht der Angreifer zu Hause nur diese Ports, und die UI verlangt weiterhin Passwort und 2FA.
- **Mail-Gateway** (optional): dasselbe Muster. Der VPS nimmt Post an und gibt sie an `10.8.0.2:25` weiter (siehe „Mail-Gateway“).

### Einrichtung Schritt für Schritt

1. **VPS (Hub)** installieren wie unter „Installation auf einem Hetzner-VPS“: im Assistenten „WireGuard-Server“ und „Nur über WireGuard“. Das Handy wird der erste Client.
2. **Standort auf dem VPS anlegen.** Am einfachsten gleich im Assistenten des VPS: nach dem ersten Handy „+ Weiteres Gerät oder GoDeb-Server“ → „GoDeb-Server zu Hause“, Name `heim-vm`. Später geht das unter WireGuard → Neuer Peer → „Standort“ oder auf der Kommandozeile:

   ```sh
   sudo godeb site add heim-vm
   ```

   Alle drei Wege zeigen einen **Beitrittscode** (`godeb-join:…`, eine Zeile). Er ist die Client-Datei selbst, base64url-kodiert, damit sie sich zwischen zwei Browsern oder Terminals kopieren lässt. Wie die Datei enthält er den privaten Schlüssel des Standorts: nur auf der Heim-VM einfügen, nicht per Chat oder Mail weitergeben. In der WireGuard-Ansicht zeigt „Code“ beim Standort ihn erneut (mit 2FA wie `.conf` und QR). Alles läuft über `wireguard.CreatePeer`. Die Client-Datei bekommt nur das Tunnelnetz, kein DNS und Keepalive 25. Der Hub schaltet IP-Forwarding ein und erlaubt Tunnel-Clients den Zugriff auf `<standort>:8080/tcp`.

   **Adresse:** Der erste Standort bekommt `10.8.0.2`. Handys und Laptops lassen diese Adresse dafür frei, solange es keinen Standort gibt. Auf Hubs, deren erstes Handy noch `10.8.0.2` hat, bekommt der Standort die nächste freie Adresse; Assistent, Ausgabe und Übersicht zeigen sie an. Mit `--address` bzw. dem Feld „Adresse im Tunnel“ lässt sie sich festlegen.
3. **Heim-VM:** Paket installieren, `sudo godeb setup-url` aufrufen, den Link im Heimnetz öffnen, Administrator und 2FA anlegen, „Mit Hub verbinden“ wählen und den Beitrittscode einfügen (oder die Datei hochladen), dann „Heimnetz und Tunnel“ und den Trusted Endpoint. Ohne Browser:

   ```sh
   sudo godeb setup --join - --access lan   # Beitrittscode einfügen, Enter
   ```

   Beides schreibt `wg0.conf` als Peer, startet den Tunnel, wartet auf den Handshake und richtet ufw ein: SSH, `8080/tcp` auf `wg0`, HTTPS `8443/tcp` nur aus dem LAN, kein offener WireGuard-Port. `--join` nimmt auch eine Datei (Client-Datei oder Code).
4. **Enshrouded auf der Heim-VM:** In der Web-UI (`http://10.8.0.2:8080`, vom Handy über den Hub) „Spielserver“ → Enshrouded → „Server installieren“. Die Ports öffnen sich nur auf `wg0`, und die Ansicht zeigt die Adresse, unter der Spieler sich verbinden.
5. **Portweiterleitung auf dem VPS:** Plugin „Portweiterleitung“, Vorlage Enshrouded, Ziel `heim-vm` (`10.8.0.2`), „Weiterleiten“. Danach im Spiel `<vps-ip>:15637`.
6. **Handy für Auto-Disconnect:** Das Handy-Profil vom Setup des VPS ist älter als der Trusted Endpoint der Heim-VM. Auf dem VPS `sudo godeb site sync` ausführen (oder WireGuard → Trusted Endpoint → „Client-Dateien aktualisieren“) und den QR-Code in der GoDeb-Tunnel-App neu scannen. Neue Clients bekommen Endpoint und Schlüssel automatisch.

Die beim Beitritt eingefügte Datei wird streng geprüft: genau ein `[Interface]` und ein `[Peer]`, nur die Schlüssel `PrivateKey`, `Address` (eine /32), `MTU`, `PublicKey`, `PresharedKey`, `Endpoint`, `AllowedIPs` und `PersistentKeepalive`. `AllowedIPs` darf nur das Tunnelnetz enthalten. `PreUp`/`PostUp`/`PreDown`/`PostDown`, `Table` und `SaveConfig` werden abgelehnt, weil `wg-quick` sie als root ausführt bzw. das Routing umstellt. `DNS` wird weggelassen. GoDeb schreibt die Datei aus den geprüften Werten neu, nie die eingefügten Zeilen.

**Getestet:** Der Weg Internet → Hub → WireGuard → Heim-VM und zurück lief mit den Regeln des Plugins in Netzwerk-Namespaces (echter Kernel, echtes WireGuard und nftables). Ohne SNAT kam keine Antwort zurück, nicht freigegebene Ports blieben zu. Der Teil mit ufw (FORWARD-Freigabe) und der Zugriff Handy → Heim-UI über den Hub sind erst auf echten Servern prüfbar.

## Ersteinrichtung

Nach `sudo apt install` steht der Link zur Einrichtung in einem Kasten am Ende der Ausgabe (`sudo godeb setup-url` zeigt ihn erneut). Ist die Einrichtung schon abgeschlossen, zeigt dieselbe Stelle die Übersicht.

**Übersicht:** Der letzte Schritt des Assistenten zeigt, wie der Server eingerichtet ist: Adressen der Weboberfläche (Tunnel mit IP, Heimnetz, SSH-Notzugang), Rolle, Tunnel-Adresse und -Netz, Endpoint, Standorte und Clients, Trusted Endpoint, SSH-Port und Firewall-Regeln. „Drucken / als PDF speichern“ (oder Strg+P) druckt nur diese Übersicht als schlichten Bogen, schwarz auf weiß. Später: Startseite → „Einrichtungsübersicht“ (`GET /api/setup/summary`, mit Login) oder `sudo godeb summary`. Sie enthält keine Passwörter, Tokens oder privaten Schlüssel.

Ein frisch installiertes GoDeb richtet nichts von selbst ein. Alle Plugins sind dabei; was davon genutzt wird, entscheidet die Einrichtung (und später der Plugin-Manager). Es entsteht insbesondere kein WireGuard-Server, solange man ihn nicht wählt.

### Im Browser (Standard)

Nach `sudo apt install --no-install-recommends ./godeb_*.deb` gibt das Paket einen Link aus, jederzeit wieder abrufbar mit:

```sh
sudo godeb setup-url
```

```text
GoDeb Ersteinrichtung im Browser öffnen:
  https://203.0.113.7:8443/#/setup?token=…
Zertifikat (selbstsigniert), SHA-256-Fingerprint:
  1B:31:11:…
```

Bis die Einrichtung abgeschlossen ist, lauscht GoDeb zusätzlich auf `0.0.0.0:8443` (`GODEB_SETUP_ADDR`) per HTTPS mit einem selbstsignierten Zertifikat (`/var/lib/godeb/tls/`). Der Browser warnt beim ersten Aufruf; der Fingerprint in den Zertifikatsdetails muss mit der Ausgabe übereinstimmen.

1. **Administrator:** Benutzername und Passwort. Dafür ist der Einmal-Token aus dem Link nötig (`/var/lib/godeb/setup-token`, `0600`). Wer den Server zufällig zuerst im Netz findet, kann ihn also nicht übernehmen. Ein zweiter „erster“ Administrator wird abgelehnt.
2. **2FA:** die normale Anmeldung mit Einrichtung von TOTP und Backup-Codes. „Drucken“ gibt nur die Backup-Codes mit Konto, Server und Datum aus, nie den TOTP-Schlüssel.
3. **WireGuard:**
   - **WireGuard-Server** (VPS): `wg0.conf` mit `10.8.0.1/24` und `51820/udp`, dazu ein erster Client als QR-Code. Mit „+ Weiteres Gerät oder GoDeb-Server“ kommen beliebig viele weitere dazu: Handys und Laptops mit QR-Code und Datei, GoDeb-Server zu Hause (Standorte) mit Beitrittscode (`POST /api/setup/client`, `firstrun.AddClient`).
   - **Mit Hub verbinden** (Heim-VM): Beitrittscode oder Client-Datei des VPS einfügen (siehe „Zielbild“).
   - **Ohne WireGuard.**
4. **Erreichbarkeit und Firewall:**
   - **Nur über WireGuard:** Loopback und `wg0:8080`. Empfohlen für einen VPS.
   - **Heimnetz und Tunnel:** zusätzlich HTTPS auf der LAN-Adresse (`<lan-ip>:8443`), in ufw nur für das LAN-Subnetz freigegeben.
   - **Nur lokal:** nur Loopback, Zugriff per `ssh -L 8080:127.0.0.1:8080`.

   Optional der Trusted Endpoint für den Auto-Disconnect der App. ufw wird eingeschaltet, nachdem die SSH-Ports freigegeben sind; ohne erkannten SSH-Port bleibt es aus.

Mit „Einrichtung abschließen“ schließt sich der Port `8443` auf allen Adressen, der Token wird gelöscht. Danach gibt es nur noch die gewählten Zugänge; Klartext-HTTP nie außerhalb von Loopback und `wg0`. Die Einstellungen stehen in `/var/lib/godeb/setup.json` und `/var/lib/godeb/access.json`. Installationen aus der Zeit vor dem Assistenten gelten beim Update als eingerichtet; der Einrichtungsport öffnet sich dort nicht.

Ist ufw schon vor der Einrichtung aktiv, muss der Port kurz offen sein (`sudo ufw allow 8443/tcp`; `sudo godeb setup-url` weist darauf hin). Der Assistent entfernt die Regel am Ende. Eine Hetzner-Cloud-Firewall braucht für die Einrichtung ebenfalls `8443/tcp`.

### Ohne Browser

`godeb setup` macht dasselbe im Terminal und benutzt dieselben Funktionen (`backend/firstrun`):

```sh
sudo godeb setup --role hub --endpoint <vps-ip> --access tunnel
sudo godeb setup --join - --access lan          # Beitrittscode einfügen
sudo godeb setup --role none --access lan
```

Der Hub bekommt einen ersten Client als QR-Code im Terminal (`firstrun.AddClient`, standardmäßig nur das Tunnelnetz als `AllowedIPs`). Optional **Volltunnel** (`--full-tunnel` bzw. Checkbox im Assistenten): IP-Forwarding und MASQUERADE in der nftables-Tabelle `godeb_volltunnel` plus `ufw route allow` von `wg0` zur öffentlichen Schnittstelle; Geräte-Clients erhalten dann `0.0.0.0/0`. Standorte bleiben ohne Volltunnel. Später unter WireGuard umschaltbar. Danach fragt `godeb setup` nach weiteren Geräten oder GoDeb-Servern, bis der Name leer bleibt; Standorte bekommen einen Beitrittscode statt des QR-Codes. 2FA richtet der Administrator dann beim ersten Login in der Web-UI ein. Flags: `--role`, `--access`, `--lan-address`, `--address`, `--port`, `--ui-port`, `--client`, `--admin`, `--ssh-port`, `--no-firewall`, `--full-tunnel`, `--join DATEI|-` (Client-Datei oder Beitrittscode, `-` liest von stdin), `--yes` (unbeaufsichtigt, braucht `--role` und einen vorhandenen Admin).

## Installation auf einem Hetzner-VPS

Für einen Testserver. Was vor dem produktiven Einsatz noch fehlt, steht unter „Grenzen“ am Ende dieses Abschnitts.

**Server anlegen** (Hetzner Cloud Console):

- x86 (CX/CPX) oder ARM (CAX): Das Paket gibt es für `amd64` und `arm64`. Spielserver (Plugin Steam) laufen nur auf x86, weil SteamCMD und die Windows-Server unter Wine nur dafür existieren. Für GoDeb allein reicht ein kleiner Typ mit 2 GB RAM. Im Zielbild läuft Enshrouded nicht auf dem VPS, sondern auf der Heim-VM (siehe „Zielbild: VPS und Heim-VM“). Soll es doch auf dem VPS laufen, braucht er mindestens 8 GB RAM, und die Serverdateien belegen knapp 8 GB auf der Platte.
- Image **Debian 13**, eigenen SSH-Schlüssel hinterlegen.
- Eine Hetzner-Cloud-Firewall ist für den Test optional, für den Produktivbetrieb empfohlen (Ports siehe „Produktivbetrieb (Checkliste)“). Mindestens eingehend `22/tcp` und `51820/udp`; Port `8080` bleibt dort zu.

**Auf dem Server** (`ssh <benutzer>@<server-ip>`, ein Konto mit `sudo`), aus dem APT-Repo (siehe „APT-Repo“). Ein frischer Hetzner-Server kennt zunächst nur root; wer sich als root anmeldet, lässt `sudo` in allen Befehlen weg.

```sh
sudo apt update && sudo apt full-upgrade -y
sudo wget -O /usr/share/keyrings/godeb-archive-keyring.asc https://zerodos1988.github.io/GoDeb/godeb-archive-keyring.asc
sudo wget -O /etc/apt/sources.list.d/godeb.sources https://zerodos1988.github.io/GoDeb/godeb.sources
sudo apt update
sudo apt install -y --no-install-recommends godeb
```

Oder ein einzelnes Paket von der [Release-Seite](https://github.com/ZeroDos1988/GoDeb/releases) mit `sudo apt install --no-install-recommends ./godeb_*_amd64.deb`. Selbst gebaut geht auch: `./scripts/build-deb.sh` im Repository, dann `scp godeb_*_amd64.deb <benutzer>@<server-ip>:`. Die CI legt das Paket bei jedem Push als Artefakt `godeb-deb` ab, bei einem Tag `vX.Y.Z` als GitHub-Release.

`--no-install-recommends` lässt Postfix weg; das braucht nur das Mail-Gateway, und seine Installation fragt interaktiv. Am Ende steht der Link zur Ersteinrichtung (`sudo godeb setup-url`).

**Im Browser:** den Link öffnen, Fingerprint prüfen, dann Administrator, 2FA, „WireGuard-Server“ (erster Client als QR-Code: mit der GoDeb-Tunnel-App scannen) und „Nur über WireGuard“. Die Übersicht am Ende drucken oder als PDF speichern. Nach dem Abschluss ist die Oberfläche nur noch über den Tunnel erreichbar: Tunnel verbinden, die Adresse aus der Übersicht öffnen (meist `http://10.8.0.1:8080`). Ohne Browser: `sudo godeb setup --role hub --endpoint <server-ip> --access tunnel`.

**Prüfen** (auf dem Server):

```sh
sudo ufw status verbose            # deny (incoming), SSH, 51820/udp, 8080 nur auf wg0
sudo ss -ltnp | grep -E ':8080|:8443'  # 8080 nur auf 127.0.0.1 und 10.8.0.1, 8443 nicht mehr
sudo wg show                       # nach dem Verbinden: latest handshake beim Peer
timedatectl | grep synchronized    # TOTP braucht eine richtige Uhr: "yes"
godeb version
systemctl status godeb fail2ban
```

Von außen (ohne Tunnel) müssen `curl -m 5 http://<server-ip>:8080` und `curl -mk 5 https://<server-ip>:8443` scheitern.

**Enshrouded** (optional): In der Web-UI „Spielserver“ → Enshrouded → „Server installieren“. Der Job installiert Wine, legt das Konto `steam-enshrouded` an, lädt SteamCMD und die Serverdateien und öffnet `15636–15637/udp` in ufw. Danach im Spiel mit `<server-ip>:15637` verbinden; die Passwörter der Gruppen stehen in der Konfiguration. Liegen unter `/var/lib/godeb-steam/enshrouded/server` schon Serverdateien, lädt die Installation sie nicht neu. Sie übergibt sie dem Konto `steam-enshrouded`, und SteamCMD prüft sie nur (`validate`) und lädt Fehlendes nach. So lässt sich ein vorhandener Download einspielen, vor dem Klick auf „Server installieren“ und inklusive `steamapps/` (dort steht die Build-Nummer):

```sh
tar -C ~/godeb-cache/enshrouded -cf - server | ssh <benutzer>@<server-ip> 'cat > enshrouded-server.tar'
ssh -t <benutzer>@<server-ip> 'sudo mkdir -p /var/lib/godeb-steam/enshrouded && sudo tar -C /var/lib/godeb-steam/enshrouded -xf enshrouded-server.tar && rm enshrouded-server.tar'
```

Der Umweg über eine Datei im Home ist nötig, weil `sudo` nach dem Passwort fragt und dafür ein Terminal braucht; dort müssen dafür kurz rund 8 GB frei sein.

Das lohnt sich vor allem bei wiederholten Tests im Heimnetz. Ein Hetzner-Server lädt von Steam meist schneller, als ein Heimanschluss 8 GB hochlädt.

Lokal getestet sind Download und Konfiguration, noch nicht der Start unter Wine ohne X-Server. Startet der Dienst nicht: `sudo journalctl -u godeb-steam-enshrouded -e`.

**SSH und Updates**: `sudo sshd -T | grep -E 'passwordauthentication|permitrootlogin'` muss `passwordauthentication no` und `permitrootlogin without-password` zeigen. Steht dort `yes`, in `/etc/ssh/sshd_config.d/10-godeb.conf` `PasswordAuthentication no` eintragen und `sudo systemctl reload ssh` ausführen, aber erst, nachdem die Anmeldung per Schlüssel funktioniert hat. Sicherheitsupdates spielt GoDeb selbst ein (Plugin APT, siehe „Updates“); `unattended-upgrades` ist dafür nicht nötig.

**Update**: im Plugin APT „Alle aktualisieren“ oder `sudo apt update && sudo apt upgrade` (aus dem APT-Repo), sonst neues Paket kopieren und `sudo apt install --no-install-recommends ./godeb_*_amd64.deb`. Automatisch kommen nur Sicherheitsupdates, GoDeb selbst nie. Jeder Build hat eine höhere Version (siehe „Versionen und Updates“), apt erkennt ihn als Update. **Entfernen**: `sudo apt purge godeb`. `/etc/wireguard` und `/etc/godeb` bleiben dabei liegen.

**Backups** macht nicht GoDeb, sondern die Plattform: auf Hetzner die Backups bzw. Snapshots des VPS, zu Hause Proxmox für die VM. Beide sichern die ganze Maschine mit `/etc/godeb`, `/etc/wireguard` und den Spielständen. Vor größeren Änderungen einen Snapshot ziehen.

**Grenzen** (Stand `ROADMAP.md`, Phase 3): Die Weboberfläche läuft ohne root, Änderungen macht der root-Agent nur mit gültigem 2FA-Code. Die Web-UI ist nur über WireGuard (oder per HTTPS im Heimnetz) erreichbar, und jede Admin-Aktion verlangt einen 2FA-Code.

## Sicherheitskonzept

Die Anmeldung hat zwei Stufen.

1. `POST /api/auth/login` prüft das bcrypt-Passwort. Bei Erfolg setzt der Daemon das Cookie `godeb_pending` (fünf Minuten, Pfad `/api/auth/2fa`).
2. `POST /api/auth/2fa/authenticate` prüft das TOTP oder einen Backup-Code. Danach gilt `godeb_session` zwölf Stunden.

Ist noch kein TOTP bestätigt, liefert der Login die Stufe `enroll`. `POST /api/auth/2fa/enroll` erzeugt Geheimnis, QR-Code und Backup-Codes. `POST /api/auth/2fa/verify` bestätigt den ersten Code. Geheimnis und Backup-Codes (gehasht) liegen beim Agenten in `/etc/godeb/godeb.db`, nie in der Datenbank der Weboberfläche. Weitere Konten dürfen 2FA nur einrichten, wenn root es mit `godeb user passwd` oder `godeb 2fa reset` freigegeben hat.

Alle `/api`-Routen außer Gesundheit, Login, Logout, 2FA und den öffentlichen OIDC-Startpunkten verlangen eine volle Sitzung. Zustandsändernde Anfragen von einer anderen Origin erhalten `403`. Fehlversuche schreibt der Daemon als `FAILED_LOGIN ip=…` nach `/var/log/godeb/auth.log`.

### Benutzermenü und Sprache

Alles, was das eigene Konto betrifft, liegt hinter dem Avatar oben rechts: Sprache, Darstellung, Sitzungen, Einrichtungsübersicht, Abmelden und die Version. Auf dem Handy sitzt der Avatar in der Kopfzeile neben dem Logo, die Seiten selbst bleiben frei davon.

Die Sprache folgt standardmäßig dem Browser (erste unterstützte Sprache aus `navigator.languages`, sonst Englisch). Wer eine Sprache wählt, legt sie im Browser (`localStorage`) und im Konto (`/api/user/settings`) ab; „Auto“ kehrt zum Browser zurück. Anmeldung und Ersteinrichtung haben oben rechts eine eigene Auswahl, die nur im Browser gilt. Nach der Anmeldung gewinnt eine im Konto gespeicherte Sprache.

### Sitzungen

Jede Anmeldung ist eine Zeile in der SQLite-Tabelle `sessions`. Gespeichert wird nur der SHA-256 der Token-ID aus dem signierten Cookie; ein Datenbankleck liefert also keine verwendbaren Cookies. Logout löscht die Zeile. Unter „Sitzungen“ im Benutzermenü (Avatar oben rechts) stehen alle eigenen Anmeldungen mit Gerät, IP und letzter Aktivität; einzelne oder alle anderen lassen sich beenden. Ein neues Passwort oder ein neues 2FA-Geheimnis beendet alle Sitzungen. Pro Login-Versuch und pro Sitzung sind fünf falsche Codes erlaubt; danach ist die Anmeldung verworfen.

Ohne Web-UI, per SSH:

```sh
sudo godeb sessions revoke admin
```

### 2FA für Admin-Aktionen

Eine Sitzung allein reicht für keine Änderung am Host. Jede `POST`/`PUT`/`DELETE`-Anfrage auf `/api` verlangt einen TOTP- oder Backup-Code im Header `X-GoDeb-TOTP`. Für die Routen des Agenten prüft der Agent den Code selbst (siehe „Aufteilung Weboberfläche und Agent“), für die übrigen die Weboberfläche über den Agenten. Fehlt er, antwortet der Daemon mit `403` und `step_up_required`. Die Oberfläche zeigt dann den Bestätigungsdialog und schickt die Aktion danach erneut ab. Neue Plugin-Routen sind ohne eigenes Zutun geschützt. Ausnahmen stehen mit Begründung in `noStepUp` (`backend/auth/stepup.go`), derzeit nur Theme/Sprache und das Neueinlesen der Plugin-Liste.

- **Fenster:** Ein bestätigter Code gilt fünf Minuten für normale Aktionen, gespeichert serverseitig an der Sitzung.
- **Immer neu (kritisch):** Plugin-Manager, OIDC-Anbieter, UFW, systemd, neue WireGuard-Peers, der Beitritt zu einem Hub, die Portweiterleitung, das Mail-Gateway-Setup, Domains-Token bzw. Apply, der Volltunnel, geplante Aufgaben (Plugin Cron, laufen als root), Webseiten (nginx, PHP, Ports 80/443) und die Enshrouded-Installation bzw. das Update verlangen bei jeder Anfrage Passwort und Code. Das Passwort kommt im Header `X-GoDeb-Password`; die Weboberfläche prüft es gegen die Konten in `/var/lib/godeb` **bevor** der Code geprüft wird, damit ein falsches Passwort keinen Code verbraucht. Anschließend entfernt sie den Header; der Agent sieht ihn nie. Fehlt das Passwort, antwortet GoDeb mit `403`, `critical: true` und `password_required: true`.
- **Downloads mit privaten Schlüsseln** (`GET …/peers/{id}/config`, `/qr`) brauchen das Fenster, obwohl es Lesezugriffe sind; kein Passwort.
- Fehlversuche (falscher Code oder falsches Passwort) zählen in dasselbe Limit und dasselbe fail2ban-Log wie beim Login. Die Regel lässt sich in der Oberfläche nicht abschalten.

### Security-Header

Jede Antwort trägt eine Content-Security-Policy (nur eigene Origin, Skripte ohne `unsafe-inline`), `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` und eine restriktive `Permissions-Policy` (`backend/headers.go`).

### Notfall-Reset

Wenn das Authenticator-Gerät fehlt:

```sh
sudo godeb 2fa reset admin
```

Der Befehl löscht Geheimnis, Bestätigung und Backup-Codes in der Agent-Datenbank und gibt eine neue Einrichtung frei. Die nächste Anmeldung richtet 2FA neu ein. Ein Passwort setzt (und erlaubt einem neuen Konto die 2FA-Einrichtung):

```sh
sudo godeb summary                               # Übersicht: Adressen, Tunnel, Zugang, Firewall
sudo godeb user passwd admin
```

`sudo godeb doctor` prüft den Server nur lesend und gibt einen Bericht zum Weitergeben aus (ohne Passwörter, Tokens oder private Schlüssel, öffentliche WireGuard-Schlüssel auf 8 Zeichen gekürzt). Er umfasst: Version und verfügbares Paket-Update, `godeb`/`godeb-agent` und fehlgeschlagene Units, Ports außerhalb von Loopback (Warnung bei GoDeb auf allen Adressen und bei Datenbank-, Redis-, Rspamd- oder HTTP-Ports außerhalb des Tunnels), ufw, WireGuard-Handshakes, Let's-Encrypt-Zertifikate (Warnung unter 14 Tagen), die DNS-Einträge aus dem Domains-Plugin, Fehler im Journal seit dem Start, ausstehende (Sicherheits-)Updates und „Neustart nötig“ die Belegung von `/` und die SQLite-Datenbanken (`PRAGMA quick_check`: Agent-Datenbank und Messwerte; die Web-Datenbank prüft die Weboberfläche selbst und hängt das Ergebnis an `/api/doctor` an, weil root keine Dateien von `godeb` öffnen soll). Eine beschädigte Datenbank ist `FAIL`: Server aus dem letzten Snapshot zurückholen (siehe „Notfall“). Exit-Code 1, wenn ein Befund `FAIL` ist.

Das Passwort muss 12 bis 72 Zeichen lang sein. `sudo` ist nötig: Nur root darf 2FA zurücksetzen oder freigeben, die Weboberfläche kann es nicht.

### Fail2ban

`scripts/setup-fail2ban.sh` und die Dateien unter `package/debian/etc/fail2ban/` richten ein Filter und ein Jail für GoDeb ein. Das Filter erkennt `FAILED_LOGIN`. Das Jail gilt für die Ports `8080` und `8443`, mit drei Versuchen in zehn Minuten und einer Sperre von einer Stunde. Das Fail2ban-Plugin zeigt Jails und hebt Sperren mit `fail2ban-client unban` auf.

### OIDC

Microsoft 365 und Google sind vorbereitet. Der Start nutzt PKCE und eine Nonce. Nur freigegebene E-Mail-Adressen werden auf lokale Benutzer abgebildet. Der Callback endet wieder in der 2FA-Stufe. Client-Geheimnisse bleiben in SQLite; ein leeres Geheimnis beim Speichern behält den alten Wert.

## Plugins

Jedes Plugin hat ein `manifest.json` (`id`, `name`, `description`, `icon`, `order`) und einen Go-Handler. Fehlt das Werkzeug auf dem Host, liefert die API Beispieldaten und setzt `mocked`, statt die Oberfläche zu leeren.

| Plugin | Laufzeit | Metadaten |
| --- | --- | --- |
| APT | `apt list --upgradable`, `godeb-apt.service` (`apt-get`) | `/etc/godeb/apt.json` (Automatik), Log und Status in `/var/lib/godeb-agent/apt/` |
| UFW | `ufw status`, `ufw --force` | keine |
| Dienste (systemd) | `systemctl show`/`list-units`, `/proc` | keine; die Auswahl der Serverdienste steht in `plugins/systemd/overview.go` |
| Certbot | `/etc/letsencrypt/live` | keine; private Schlüssel werden nicht gelesen |
| Fail2ban | `fail2ban-client` | keine |
| WireGuard | `/etc/wireguard/wg0.conf`, `wg` | IPAM in `wireguard_peers` |
| Mail-Gateway | Postfix, OpenDKIM, DNS | Domains, öffentlicher DKIM-Teil, Queue-Historie |
| Spielserver (Steam) | SteamCMD, Wine, `godeb-steam-<spiel>.service` | keine; Konfiguration bleibt in der Datei des Spiels, z. B. `enshrouded_server.json` |
| Protokoll | `journalctl -u godeb -u godeb-agent` (nur diese beiden Units) | keine |
| Überwachung | `/proc`, `statfs`, `systemctl --failed`, `apt list --upgradable`, `timedatectl`, Quellen und Checks der anderen Plugins | Verlauf in `metrics.db` |

### WireGuard

Der Server bleibt in `wg0.conf`. Neue Peers bekommen die nächste freie Adresse aus dem dort konfigurierten Netz, üblich `10.8.0.0/24`. Die Reservierung steht zusätzlich in `wireguard_peers`, damit ein Neustart dieselbe Adresse nicht ein zweites Mal vergibt. Client-Dateien liegen unter `/etc/wireguard/clients`. Die Oberfläche erzeugt einen QR-Code und kann die Datei über das SMTP-Konto in `/etc/godeb/smtp.json` senden. Schlägt nur das Speichern der Reservierung fehl, bleibt der Peer in der Live-Konfiguration und die Antwort nennt den Grund.

Rollen: Ein Hub hat `ListenPort` und Clients als Peers. Ein Peer-Host (Zeile `# GoDeb-Role: peer` in `wg0.conf`, oder genau ein Peer mit Endpoint und kein `ListenPort`) hängt an einem anderen GoDeb und legt selbst keine Clients an. Client-Dateien von Standorten tragen `# GoDeb-Role: site`. Für jeden Standort hält der Hub eine `ufw route`-Regel mit dem Kommentar `GoDeb-Hub <name>` (`godeb site sync` setzt sie neu).

### Trusted Endpoint

Damit die GoDeb-Tunnel-App den Tunnel im Heim-WLAN abschaltet und außerhalb wieder einschaltet. Die App fragt den Heimserver über das WLAN an, am Tunnel vorbei, und schickt eine Zufallszahl mit. Der Server signiert sie mit seinem Schlüssel `/etc/godeb/trust.key` (ECDSA P-256, `0600`). Die App vertraut dem Netz nur, wenn die Signatur zu dem Schlüssel passt, der in ihrer Tunnel-Konfiguration gepinnt ist. Ein fremdes WLAN kann das nicht nachmachen, auch wenn es unter derselben Adresse antwortet. Die frühere Prüfung (`/api/trusted`, nur ein Header) ist deshalb entfernt.

- **Listener:** eigener HTTP-Server nur an der LAN-Adresse (z. B. `192.168.1.50:8787`) mit genau einer Route, `GET /godeb-trust?nonce=…`. Die Web-UI bleibt nur im Tunnel erreichbar. Höchstens 8 gleichzeitige Signaturen, kurze Timeouts.
- **Firewall:** `ufw allow in on <lan-if> from <lan-netz> to <lan-ip> port 8787 proto tcp`.
- **Einschalten:** `sudo godeb trust enable [--address IP] [--port 8787]`, in `godeb setup --join` oder in der Web-UI (WireGuard → Trusted Endpoint). Die Einstellung liegt in `/etc/godeb/trust.json`; der Dienst übernimmt Änderungen nach spätestens 5 Sekunden ohne Neustart. Ein- und Ausschalten braucht einen frischen 2FA-Code.
- **Verteilung:** Beim Anlegen eines Clients holt der Hub über den Tunnel `GET /api/plugins/wireguard/trust/info` von jedem Standort (öffentlicher Schlüssel und LAN-URL, ohne Login, nur über `wg0` erreichbar) und schreibt in die Client-Datei:

  ```text
  # GoDeb-Trusted-Endpoint: http://192.168.1.50:8787/godeb-trust
  # GoDeb-Trusted-Key: MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE…
  ```

  Die App liest das beim Import (QR oder Datei) und schaltet Auto-Disconnect für diesen Tunnel ein. Ein Hub ohne Standorte, der selbst zu Hause steht, trägt seinen eigenen Endpoint ein. `godeb site sync` bzw. „Client-Dateien aktualisieren“ schreibt die Zeilen in vorhandene Client-Dateien neu.
- **Protokoll `godeb-trust-v1`:** Antwort `{"v":1,"nonce":"…","sig":"…"}`, signiert wird `"godeb-trust-v1\n" + nonce` mit SHA-256, Signatur DER-kodiert, Schlüssel als Base64-SubjectPublicKeyInfo. Geprüft mit Javas `SHA256withECDSA`. Die Beschreibung für die App steht in `GODEB.md` im App-Repository.
- **Heim-VM als VM:** Die Netzwerkkarte muss im LAN hängen (Bridge), sonst erreicht das Handy die LAN-Adresse nicht.

### Überwachung

Das Plugin `monitor` sammelt im Agenten alle 10 Sekunden Messwerte und prüft jede Minute den Zustand. Es ändert nichts am System.

- **Messwerte**: CPU, I/O-Wartezeit, RAM, Swap, Last, Belegung jeder Platte, Netz pro Interface, Lesen/Schreiben pro Platte; dazu die Quellen der Plugins (WireGuard: Traffic pro Peer und verbundene Peers). Zähler (Bytes) werden als Rate pro Sekunde gespeichert.
- **Verlauf**: SQLite `metrics.db` in drei Stufen: Rohwerte 26 Stunden, 5-Minuten-Mittel 31 Tage, Stundenmittel 400 Tage (jeweils mit Maximum). Wenige MB.
- **Ampel**: Checks liefern Befunde (`warning`, `critical`, `info`): Platte ab 85/95 %, RAM unter 10/5 % frei, Last über 15 Minuten über 2 × Kerne, ausgefallene systemd-Units, Neustart nötig (`/run/reboot-required`), Uhr nicht synchron, **Sicherheitsupdates offen nur wenn die Automatik aus ist** (sonst nur Info, bis `godeb-apt` sie einspielt), fehlgeschlagener automatischer APT-Lauf (`/var/lib/godeb-agent/apt/status.json`), WireGuard-Tunnel aus bzw. kein Handshake zum Hub (Peer), Zertifikate 14/3 Tage vor Ablauf. Warnungen und Störungen gehen bei eingeschalteten Alarmen als Mail bzw. ntfy raus.
- **Ereignisse**: Neue und behobene Befunde und Meldungen der Plugins (z. B. Spiel-Update) landen als Ereignis im Verlauf und als Markierung in den Diagrammen.
- **Spielserver**: Für jedes eingerichtete Spiel `game:<id>:up`, und solange es läuft, Spieler und Plätze per Steam-Query (A2S_INFO an `127.0.0.1:<queryPort>`, mit Challenge, ohne Abhängigkeit). Installation, Update, Start, Stopp und Neustart werden mit Benutzer und Dauer als Ereignis markiert. Ein aktivierter, aber nicht laufender Server ergibt eine Warnung.
- **Traffic pro Dienst**: Der Agent legt die nftables-Tabelle `inet godeb_acct` an: benannte Zähler und Ketten mit `policy accept` ohne jedes Urteil (Priorität 10, also nach dem Filter, verworfene Pakete zählen nicht). Sie kann nichts blockieren und nichts öffnen. Gezählt werden SSH (Ports aus `sshd -T`), GoDeb (8080, 8443), HTTP/HTTPS, Mail, die UDP-Ports jedes Spiels und auf dem Hub jede Portweiterleitung (in der `forward`-Kette). Die Tabelle wird bei jeder Änderung der Dienste und nach einem Neustart neu angelegt; `apt remove` löscht sie. Ansehen: `nft list table inet godeb_acct`.
- **Verbindungen**: live aus `/proc/net/nf_conntrack`, nach Gegenstelle gruppiert, mit Richtung (ein- und ausgehend, weitergeleitet) und Dienst. Tunnel-Adressen tragen den Peer-Namen. Bytes pro Verbindung gibt es nur mit `net.netfilter.nf_conntrack_acct=1` (GoDeb setzt das nicht).
- **Zugriffe**: auf Abruf aus dem Journal (bis 7 Tage): SSH-Anmeldungen und Fehlversuche (`sshd`/`sshd-session`), GoDeb-Anmeldungen und Änderungen (eigenes Log), fail2ban-Sperren (`/var/log/fail2ban.log` oder Journal), ufw-Blocks aus dem Kernel-Log, mit den häufigsten Absendern.
- **Normalverhalten**: Für CPU, Interfaces und Dienste lernt GoDeb den üblichen Wert pro Wochenstunde (Median der Stundenmittel der letzten vier Wochen; solange weniger Verlauf da ist, dieselbe Tagesstunde der letzten Woche, ab drei Tagen). Liegt der Mittelwert der letzten 15 Minuten deutlich darüber, gibt es einen Hinweis (Traffic ab dem Dreifachen und 100 KB/s mehr, CPU 30 Punkte und doppelt), ab dem Zehnfachen (und 1 MB/s mehr) bzw. 50 Punkten eine Warnung. Weniger als üblich meldet GoDeb nie. Die Diagramme zeigen das Übliche gestrichelt.
- **Alarme**: `/etc/godeb/monitor-alerts.json` (root, 0600; ändern nur mit 2FA): Mail über `/etc/godeb/smtp.json` und/oder Push an ein ntfy-Thema (nur `https://`, ohne Zugangsdaten). Ab Stufe Warnung oder nur Störung, auf Wunsch mit Entwarnung. Änderungen werden 30 Sekunden gesammelt; dieselbe Meldung kommt frühestens nach 6 Stunden wieder, außer sie wird schlimmer.
- **Standorte**: Der Hub fragt jede Minute jeden Standort (`godeb site add`) über den Tunnel nach `GET http://<tunnel-ip>:8080/api/plugins/monitor/site`. Der Standort antwortet nur, wenn er Peer ist und die Anfrage von der Tunnel-Adresse des Hubs kommt (erste Adresse des Tunnel-Netzes; WireGuard bindet sie an den Schlüssel des Hubs), sonst `404`. Die Antwort enthält nur Ampel, Titel der Befunde, CPU/RAM/Platte/Last und Spieler; der Hub kürzt sie und übernimmt die Befunde in seine eigene Ampel, damit Alarme auf dem Hub auch die Heim-VM abdecken. Ein Standort, der 3 Minuten nicht antwortet, ist eine Störung.
- **Hub von außen** (Gegenrichtung): Ein Hub, der ganz ausfällt, kann selbst nicht alarmieren. Deshalb fragt jeder Peer (die Heim-VM) jede Minute `GET http://<hub-tunnel-ip>:8080/api/health` über den Tunnel. Kommt 3 Minuten lang keine Antwort, meldet seine Ampel „Hub nicht erreichbar“ als Störung, und die Alarme der Heim-VM gehen raus (ntfy oder Mail; Mail funktioniert nur, wenn sie nicht über den ausgefallenen VPS läuft, also ntfy einschalten). Öffentliche Dienste des VPS (Mail, Webseiten) zusätzlich mit einem externen Uptime-Dienst prüfen; die Heim-VM sieht nur den Tunnel.
- Plugins melden sich über `metrics.RegisterSource`, `metrics.RegisterCheck` und `metrics.Emit` an; der Sammler läuft als Dienst des Plugins (`core.RegisterService`) nur im Agenten.

### Portweiterleitung

Nur auf dem Hub. Eine Regel besteht aus Name, Protokoll, Portbereich und einem Ziel, das als Peer in `wg0.conf` steht. Sie liegt in `/etc/godeb/portforward.json` (`0600`). Beim Speichern prüft GoDeb alle Regeln, testet das erzeugte Skript mit `nft -c` und ersetzt die Tabelle `godeb_portforward` atomar. Danach gleicht es die `ufw route`-Regeln mit dem Kommentar `GoDeb-PF …` ab. Scheitert ufw, wird die vorige Tabelle wiederhergestellt. Abgelehnt werden Ports unter 1024, die SSH-Ports, der WireGuard-Port, `8080/tcp`, Bereiche über 1000 Ports, Überschneidungen und Ziele außerhalb des Tunnelnetzes. Nur IPv4.

### Spielserver (Steam)

Ein Plugin für alle Spiele, deren dedizierter Server über SteamCMD kommt. Die Oberfläche zeigt pro Spiel einen Reiter. Das Plugin übernimmt alles, was für jedes Spiel gleich ist. Ein Spiel beschreibt sich in Go (`plugins/steam/game.go`, Typ `Game`): App-ID, Plattform (Linux oder Windows unter Wine), Startdatei und Argumente, zusätzliche APT-Pakete, Stoppsignal, Ports und optional eine Konfigurationsdatei mit Standardinhalt und Prüfung. Enshrouded steht in `plugins/steam/enshrouded.go`. Ein Formular für die Konfiguration ist optional (`plugins/steam/ui/games/`); ohne Formular gibt es den Datei-Editor.

Jedes Spiel bekommt ein eigenes Systemkonto `steam-<spiel>`, ein eigenes Verzeichnis `/var/lib/godeb-steam/<spiel>/` (`steamcmd/`, `server/`, bei Windows-Servern `wine/`) und eine eigene Unit `godeb-steam-<spiel>.service`. Ein übernommener Spielserver kommt so nicht an die Dateien eines anderen Spiels. Es läuft immer nur ein Install- oder Update-Job zugleich.

Ein Klick auf „Server installieren“ startet einen Hintergrundjob, dessen Log die Oberfläche live zeigt:

1. APT-Pakete aus `plugins/steam/plugin.json` (`lib32gcc-s1`, `ca-certificates`) und die des Spiels (Enshrouded: `wine`) nachinstallieren, falls sie fehlen.
2. Systemkonto `steam-<spiel>` und Verzeichnisse anlegen.
3. SteamCMD von Valve laden und entpacken, dann die App installieren (Windows-Server mit `+@sSteamCmdForcePlatformType windows`). SteamCMD läuft als das Spielkonto, nicht als root. Ein frisches SteamCMD scheitert beim ersten Lauf oft mit „Missing configuration“, deshalb sind drei Versuche erlaubt. Vorhandene Serverdateien werden nur geprüft.
4. Bei Windows-Servern das Wine-Prefix anlegen. Gibt es eine Konfigurationsdatei, die Standardkonfiguration schreiben (Enshrouded: zufällige Gruppenpasswörter, `0640`, Eigentümer ist das Spielkonto).
5. `/etc/systemd/system/godeb-steam-<spiel>.service` schreiben: eigenes Konto, `ProtectSystem=strict`, Schreibrecht nur auf das Spielverzeichnis, keine Capabilities.
6. UFW: die Ports des Spiels öffnen (Enshrouded: `15636–15637/udp`), hinter einem Hub nur auf `wg0`. Dann den Dienst aktivieren und starten.

„Aktualisieren“ stoppt den Server, zieht den neuesten Build und startet ihn wieder, wenn er lief. Installation und Update verlangen bei jeder Anfrage einen frischen 2FA-Code.

**Enshrouded:** Die Konfiguration hat zwei Ansichten mit einem Umschalter:

- **Einfach**: Servername, Spielerplätze, Schwierigkeit (Preset), Ports, Sprach-/Textchat und Benutzergruppen mit Passwort und Rechten.
- **Datei**: Editor für `enshrouded_server.json` mit Live-JSON-Prüfung.

Beide Ansichten bearbeiten dasselbe Objekt. Schlüssel, die das Formular nicht kennt (etwa einzelne `gameSettings` oder neue Felder späterer Serverversionen), bleiben erhalten. Der Server prüft vor dem Schreiben Typen und Wertebereiche (Ports 1024–65535, 1–16 Plätze, relative Verzeichnisse, keine Steuerzeichen in Namen/Passwörtern). Änderungen gelten nach einem Neustart; „Speichern und neu starten“ erledigt beides.

Hinweis: Enshrouded hat keinen nativen Linux-Server. Der Windows-Server läuft unter Wine; das ist der übliche Weg, aber Wine-Versionen können sich unterschiedlich verhalten. Der Server braucht etwa 6–8 GB RAM.

**Weiteres Spiel hinzufügen:** In `plugins/steam/` eine Datei mit `register(&Game{…})` in `init()` anlegen und in einem Test prüfen, dass Standardkonfiguration und Prüfung zusammenpassen. Optional ein Formular unter `ui/games/` in `forms` eintragen.

### Domains und DNS (Plugin Domains)

Domains werden einmal in GoDeb eingetragen. Andere Plugins (zuerst Mail) melden die DNS-Einträge, die sie brauchen. Pro Domain gibt es zwei Betriebsarten:

- **Manuell:** GoDeb zeigt die Einträge zum Kopieren und prüft sie live (wie der DNS-Reiter im Mail-Plugin).
- **Hetzner Cloud-API:** Ein API-Token (im Agenten versiegelt, Projekt mit den Zonen) legt fehlende RRSets an und ändert nur Einträge mit Label `godeb=1`. Fremde Einträge bleiben unberührt und erscheinen als Konflikt. Reverse-DNS (PTR) setzt GoDeb über `change_dns_ptr` am Cloud-Server mit derselben öffentlichen IP.

Kein eigener Port 53 auf dem VPS. Die alte DNS-API unter `dns.hetzner.com` wird nicht genutzt.

### Mailserver (Plugin E-Mail)

Ein vollständiger Mailserver nach dem Vorbild von HestiaCP: Maildomains, Postfächer mit Quota, Aliase, Weiterleitungen (mit oder ohne eigene Kopie), reine Weiterleitungsadressen, Catch-all, Sperren von Postfach oder Domain, DKIM, SPF/DMARC-Vorgaben und Spamfilter. Bedienung: Domainliste, je Domain die Reiter Postfächer, DNS, Einstellungen und Mailprogramm. Gedacht ist er zuerst für den VPS; später soll der VPS nur weiterleiten und die Postfächer auf der Heim-VM liegen (siehe `ROADMAP.md`). Mailserver und Mail-Gateway schließen sich aus, weil beide Postfix konfigurieren.

**Bausteine (Debian 13):** Postfix 3.10 (SMTP, Submission), Dovecot 2.4 (IMAP, LMTP, Sieve, Quota), Rspamd 3.12 (Spamfilter, DKIM-Signatur und ARC, Prüfung von SPF/DKIM/DMARC) mit Redis, optional ClamAV. „Einrichten“ installiert die Pakete, legt das Konto `vmail` und `/var/vmail` an, schreibt die Konfiguration, startet die Dienste und öffnet in ufw `25`, `465`, `587` und `993/tcp`.

**Sicherheit:**

- Port 25 nimmt nur Post für die eigenen Domains an (`reject_unauth_destination`, `reject_unlisted_recipient`), ohne Anmeldung. Versenden geht nur über 465 (TLS) oder 587 (STARTTLS, Pflicht) mit Anmeldung, und nur mit eigener Adresse oder eigenem Alias als Absender (`reject_sender_login_mismatch` mit `/etc/postfix/godeb/senders`).
- IMAP nur verschlüsselt auf 993 (Port 143 ist aus), keine Anmeldung ohne TLS (`auth_allow_cleartext = no`), TLS ab 1.2. Kein POP3.
- GoDeb schreibt die ganze `dovecot.conf` selbst (das Original bleibt als `dovecot.conf.godeb-orig`): Debians Fassung meldet Systembenutzer per PAM an, hier dürfen nur die Postfächer aus GoDeb. Dovecot-Updates übernehmen die Datei nicht (`UCF_FORCE_CONFFOLD`).
- Passwörter speichert GoDeb nur als Argon2id-Hash (19 MiB, t=2), in der Agent-Datenbank zusätzlich versiegelt. Das Passwort wird einmal nach dem Anlegen angezeigt.
- Gesperrte Postfächer stehen in `/etc/dovecot/godeb/deny` und werden vor der Passwortprüfung abgewiesen; sie empfangen weiter. Gesperrte Domains nehmen keine Post mehr an.
- Fail2ban überwacht Postfix (`mode = aggressive`) und Dovecot über das Journal (`/etc/fail2ban/jail.d/godeb-mail.conf`).
- Rspamd: ab 4 Punkten Greylisting, ab 6 Kopfzeile `X-Spam: Yes` (ein Sieve-Skript legt die Mail in „Junk“), ab 15 Ablehnung. ClamAV lehnt Viren ab.
- Zertifikat: bis zur Ausstellung das selbstsignierte von `ssl-cert`; „Let's-Encrypt-Zertifikat holen“ ruft `certbot --standalone` für den Hostnamen auf. Port 80 ist nur dabei offen (auch beim Verlängern, über die gespeicherten Hooks), danach laden Postfix und Dovecot neu.

**Dateien**, alle von GoDeb erzeugt und bei jeder Änderung neu geschrieben: `/etc/postfix/godeb/{domains,mailboxes,aliases,senders}` (texthash, kein `postmap` nötig), Einstellungen in `main.cf`/`master.cf` per `postconf`, `/etc/dovecot/dovecot.conf`, `/etc/dovecot/godeb/{users,deny,spam.sieve}`, `/etc/rspamd/local.d/*.conf`, DKIM-Schlüssel `/var/lib/rspamd/dkim/<domain>.<selector>.key` (`root:_rspamd`, `0640`). Die Metadaten (Domains, Postfächer, Hashes) liegen in der Agent-Datenbank `/etc/godeb/godeb.db` (`mailserver_*`).

**DNS pro Domain:** MX auf den Hostnamen, SPF `v=spf1 mx -all`, DKIM (Selector `godeb`, RSA 2048), DMARC `p=quarantine`, dazu einmal der A-Eintrag des Hostnamens und der Reverse-DNS (PTR) beim Hoster. Der Reiter DNS zeigt alle Einträge zum Kopieren und prüft sie live.

**Prüfen ohne Server:** `scripts/check-mail-config.sh` erzeugt die Konfiguration für ein Beispiel und prüft sie mit den echten Werkzeugen aus Debian 13 (`postconf`, `postmap`, `doveconf`, `sievec`, `doveadm pw`, `rspamadm configtest`). Die Pakete werden nur entpackt und in einem User-Namespace ausgeführt, ohne root.

### Mail-Gateway

Der Hetzner-V-Server nimmt Post für eingetragene Domains an und gibt sie über den WireGuard-Tunnel an den Homeserver weiter. Postfix ist der Store-and-Forward-Puffer: ist `10.8.0.x:25` nicht erreichbar, bleibt die Post in der Queue und geht nach dem Reconnect mit `postqueue -f` oder dem normalen Queue-Lauf hinaus.

Es ist kein offenes Relay. `mynetworks` bleibt lokal, `smtpd_relay_restrictions` endet auf `reject_unauth_destination`, und `relay_domains` enthält nur Domains aus der Oberfläche. Der nächste Hop muss im Tunnelnetz `10.8.0.0/24` liegen. Der private DKIM-Schlüssel steht mit Modus `0600` unter `/etc/opendkim/keys/` und nicht in SQLite. Die Datenbank hält Domain, Selector, Relay-Ziel, den öffentlichen DKIM-Teil und die Queue-Historie.

`postconf -e` ändert einzelne Schlüssel in `/etc/postfix/main.cf`. Die übrige Postfix-Konfiguration bleibt stehen. OpenDKIM signiert auf `127.0.0.1:8891`, bevor Postfix an den Tunnel übergibt. TLS zum Homeserver ist opportunistisch (`smtp_tls_security_level = may`).

## API

Sitzung nötig, soweit nichts anderes steht. Antworten sind JSON.

### Ersteinrichtung

| Methode | Pfad | Bedeutung |
| --- | --- | --- |
| GET | `/api/setup/status` | ohne Login: `required`, `hasAdmin` |
| POST | `/api/setup/admin` | ohne Login, mit Header `X-GoDeb-Setup-Token`: erster Administrator |
| GET | `/api/setup/info` | öffentliche IP, LAN-Adressen, SSH-Ports, ufw, Zertifikat |
| POST | `/api/setup/hub` | WireGuard-Server und erster Client (`first`: Datei, QR als Data-URL) |
| POST | `/api/setup/client` | weiterer Client auf dem Hub (`name`, `site`, `address`): Datei und QR bzw. Beitrittscode |
| POST | `/api/setup/join` | Peer eines Hubs |
| POST | `/api/setup/finish` | Erreichbarkeit, Trusted Endpoint, Firewall, Abschluss |

Alle außer `status` antworten nach dem Abschluss mit `410`.

### Kern und Konto

| Methode | Pfad | Bedeutung |
| --- | --- | --- |
| GET | `/api/health` | öffentlich, Dienst lebt |
| GET | `/api/system` | Hostname, OS, Kernel, CPU, RAM, Platte |
| GET | `/api/doctor` | Server-Diagnose wie `godeb doctor` (nur lesen; mit Agent über `/internal/doctor`, Ausgabe ohne Geheimnisse) |
| GET | `/api/plugins` | Sidebar-Metadaten |
| POST | `/api/auth/login` | Passwort, setzt Pending-Cookie |
| POST | `/api/auth/2fa/enroll` | TOTP anlegen |
| POST | `/api/auth/2fa/verify` | Einrichtung bestätigen |
| POST | `/api/auth/2fa/authenticate` | TOTP oder Backup-Code |
| POST | `/api/auth/logout` | Sitzung beenden |
| GET | `/api/auth/me` | aktueller Benutzer |
| GET | `/api/auth/sessions` | eigene Sitzungen mit Gerät, IP, letzter Aktivität |
| POST | `/api/auth/sessions/{id}/revoke`, `/api/auth/sessions/revoke-others` | eine bzw. alle anderen Sitzungen beenden |
| GET | `/api/plugin-manager` | Plugins mit Paketen, Units, an/aus |
| POST | `/api/plugin-manager/scan`, `/api/plugin-manager/{id}/install`, `/api/plugin-manager/{id}/enable`, `/api/plugin-manager/{id}/disable` | neu einlesen, Pakete installieren, ein- und ausblenden (frischer 2FA-Code außer bei `scan`) |
| GET, POST | `/api/user/settings` | Theme und Sprache (`language` leer = wie der Browser) |
| GET | `/api/auth/oidc/providers` | öffentliche, fertig konfigurierte Anbieter |
| GET, PUT | `/api/auth/oidc/config`, `/api/auth/oidc/providers/{id}` | OIDC-Konfiguration |
| GET | `/api/auth/oidc/{id}/start` | Browser-Redirect zum Anbieter |
| GET | `/api/auth/oidc/{id}/callback` | Rückkehr vom Anbieter |
| GET | `/api/auth/oidc/resume` | Pending-Sitzung nach dem Redirect |

### Fach-Plugins

| Methode | Pfad |
| --- | --- |
| GET | `/api/plugins/apt/upgradable` (Pakete mit `security`, Automatik, letzter Lauf mit Log) |
| POST | `/api/plugins/apt/check` (prüfen; mit Automatik für Sicherheitsupdates wie `security`) |
| POST | `/api/plugins/apt/security` (prüfen und Sicherheitsupdates installieren) |
| POST | `/api/plugins/apt/upgrade` (alles aktualisieren, frischer 2FA-Code) |
| POST | `/api/plugins/apt/settings` (`autoCheck`, `interval` 5–1440 Minuten, `autoSecurity`) |
| GET | `/api/plugins/ufw/status` |
| POST | `/api/plugins/ufw/rules` |
| GET | `/api/plugins/systemd/overview` (Eckdaten des Servers, ausgewählte Dienste mit Status, Laufzeit, CPU, RAM) |
| GET | `/api/plugins/systemd/services` |
| POST | `/api/plugins/systemd/action` (`unit`, `action`: `start`, `stop`, `restart`; nicht für GoDebs eigene Units) |
| GET | `/api/plugins/certbot/certificates` |
| GET | `/api/plugins/fail2ban/status` |
| POST | `/api/plugins/fail2ban/unban` |
| GET | `/api/plugins/wireguard/status` |
| GET, POST | `/api/plugins/wireguard/peers` (anlegen: optional `publicKey` vom Gerät – dann keine Client-Datei mit privatem Schlüssel) |
| GET | `/api/plugins/wireguard/peers/{id}/config` |
| POST | `/api/plugins/wireguard/peers/{id}/delete-config` (Client-`.conf` mit privatem Schlüssel löschen; Peer bleibt) |
| GET | `/api/plugins/wireguard/android`, `/api/plugins/wireguard/android/apk` (GoDeb-Tunnel-App: Infos und Download) |
| GET | `/api/plugins/wireguard/peers/{id}/qr` |
| GET | `/api/plugins/wireguard/peers/{id}/code` (Beitrittscode eines Standorts) |
| POST | `/api/plugins/wireguard/peers/{id}/send-email` |
| POST | `/api/plugins/wireguard/join` (`config`: Beitrittscode oder Client-Datei des Hubs) |
| GET, POST | `/api/plugins/wireguard/trust` (`enabled`, `address`, `port`) |
| POST | `/api/plugins/wireguard/trust/sync` |
| GET | `/api/plugins/wireguard/trust/info` (ohne Login, für den Hub) |
| GET, POST | `/api/plugins/wireguard/android/token` (Download-Token der APK; `action`: `generate` oder `clear`) |
| GET, POST | `/api/plugins/wireguard/volltunnel` (`enabled`; nur auf dem Hub, frischer 2FA-Code) |
| GET | `/api/plugins/portforward/status` |
| POST | `/api/plugins/portforward/rules` (`name`, `proto`, `from`, `to`, `target`) |
| POST | `/api/plugins/portforward/rules/{name}/delete` |
| GET | `/api/plugins/domains/status` (Domains, Token hinterlegt?, öffentliche IP) |
| POST | `/api/plugins/domains/token` (`token` oder `clear`, optional `serverId`; frischer 2FA-Code) |
| POST | `/api/plugins/domains/domains` (`name`, `mode`: `manual` oder `hetzner`) |
| GET, POST | `/api/plugins/domains/domains/{domain}` (Ansicht; ändern: `mode`) |
| POST | `/api/plugins/domains/domains/{domain}/delete` |
| GET | `/api/plugins/domains/domains/{domain}/records` (Soll + Live-Prüfung; bei Modus `hetzner` zusätzlich `hetznerError` und `zoneMissing`, wenn die Zone im Projekt fehlt) |
| POST | `/api/plugins/domains/domains/{domain}/apply` (bei Modus `hetzner`; frischer 2FA-Code) |
| GET | `/api/plugins/mail/status` (Einrichtung, Dienste, Domains, laufende Aufgabe) |
| POST | `/api/plugins/mail/install` (`hostname`, `clamav`; frischer 2FA-Code) |
| POST | `/api/plugins/mail/clamav` (`enabled`), `/api/plugins/mail/certificate` (`email`) |
| POST | `/api/plugins/mail/domains` (`name`) |
| GET, POST | `/api/plugins/mail/domains/{domain}` (Postfächer, DNS-Einträge, Mailprogramm; ändern: `catchAll`, `suspended`) |
| POST | `/api/plugins/mail/domains/{domain}/delete` |
| GET | `/api/plugins/mail/domains/{domain}/dns` (Live-Prüfung) |
| POST | `/api/plugins/mail/domains/{domain}/accounts`, `/api/plugins/mail/domains/{domain}/accounts/{local}` (`local`, `password`, `quotaMB`, `aliases`, `forward`, `forwardOnly`, `suspended`; leeres Passwort behält das alte) |
| POST | `/api/plugins/mail/domains/{domain}/accounts/{local}/delete` (mit allen Mails) |
| GET | `/api/plugins/mail/queue` (Postfix-Warteschlange, `postqueue -j`) |
| POST | `/api/plugins/mail/queue/flush`, `/api/plugins/mail/queue/{id}/retry`, `/api/plugins/mail/queue/{id}/delete` (Queue-ID 6–25 Buchstaben/Ziffern) |
| GET | `/api/plugins/cron/status` (systemd-Timer, `/etc/cron.d` nur lesend, Allowlist) |
| POST | `/api/plugins/cron/jobs` (`name`, `calendar`, `command` als Liste, `description`; frischer 2FA-Code), `/api/plugins/cron/jobs/{name}/delete` |
| GET | `/api/plugins/websites/status` |
| POST | `/api/plugins/websites/sites`, `/api/plugins/websites/sites/{name}` (`domain`, `mode`: `static` oder `proxy`, `php`, `proxyUrl`, `tls`, `public`, `enabled`; frischer 2FA-Code) |
| POST | `/api/plugins/websites/sites/{name}/delete`, `/api/plugins/websites/sites/{name}/certificate` (`email`) |
| GET | `/api/plugins/postgres/status`, `/api/plugins/postgres/databases/{name}/tables`, `/api/plugins/postgres/databases/{name}/tables/{table}` (erste 100 Zeilen, nur lesend) |
| POST | `/api/plugins/postgres/databases` (`name`, `owner`), `/api/plugins/postgres/roles` (`name`, `password`), jeweils `…/{name}/delete` |
| GET | `/api/plugins/mailgateway/status` |
| POST | `/api/plugins/mailgateway/setup` |
| GET | `/api/plugins/mailgateway/queue` |
| POST | `/api/plugins/mailgateway/queue/flush` |
| GET | `/api/plugins/mailgateway/dns-check?domain=` |
| GET | `/api/plugins/steam/games` |
| GET | `/api/plugins/steam/games/{game}/status` |
| GET | `/api/plugins/steam/games/{game}/job` |
| POST | `/api/plugins/steam/games/{game}/install` |
| POST | `/api/plugins/steam/games/{game}/update` |
| POST | `/api/plugins/steam/games/{game}/action` (`start`, `stop`, `restart`) |
| GET, PUT | `/api/plugins/steam/games/{game}/config` |
| GET | `/api/plugins/steam/games/{game}/logs` |
| GET | `/api/plugins/monitor/summary` (Ampel, Befunde, aktuelle Werte) |
| GET | `/api/plugins/monitor/series?names=cpu,net:*&range=1h\|6h\|24h\|7d\|30d\|1y` |
| GET | `/api/plugins/monitor/events?range=` |
| GET | `/api/plugins/monitor/connections` |
| GET | `/api/plugins/monitor/access?range=1h\|6h\|24h\|7d` |
| GET | `/api/plugins/monitor/baseline?names=cpu,svc:ssh:in&range=` (bis 7 Tage) |
| GET, POST | `/api/plugins/monitor/alerts` (`enabled`, `email`, `ntfy`, `level`, `resolved`) |
| POST | `/api/plugins/monitor/alerts/test` |
| GET | `/api/plugins/monitor/sites` (Hub) |
| GET | `/api/plugins/monitor/site` (ohne Login, nur von der Tunnel-Adresse des Hubs) |
| GET | `/api/plugins/logs/entries?source=all\|web\|agent&level=error\|warning\|info\|debug&since=15m\|1h\|24h\|7d\|all&lines=&q=` |

**Geplante Aufgaben (Cron):** Neue Aufgaben sind systemd-Timer `godeb-cron-<name>` ohne Shell. Erlaubt sind `true`, `systemctl reload|reload-or-restart|try-reload-or-restart <unit>`, `certbot renew` (nur `--quiet`, `-q`, `--no-random-sleep-on-renew`, keine Hooks), `logrotate [-f]` mit `/etc/logrotate.conf` oder `/etc/logrotate.d/<datei>` sowie Skripte in `/usr/local/sbin` und `/etc/godeb/scripts`. `%` wird für systemd verdoppelt.

**Webseiten:** Das Dokumentverzeichnis `/var/www/godeb/<name>` gehört root und bekommt beim Anlegen eine `index.html`; danach fasst GoDeb den Inhalt nicht mehr an (kein rekursives `chown`). Dateien per SSH/SFTP mit `sudo` pflegen. Fällt `nginx -t` durch, stellt GoDeb die vorherige Konfiguration der Seite wieder her.

**PostgreSQL:** SQL geht per stdin an `runuser -u postgres -- psql`, nie als Argument. Passwörter neuer Rollen erreichen PostgreSQL nur als SCRAM-SHA-256-Verifier; sie sind 10–128 druckbare ASCII-Zeichen.

`POST /api/plugins/mailgateway/setup` erwartet `domain`, `relayHost` und optional `selector`. Der Handler erzeugt ein 2048-Bit-DKIM-Paar, schreibt OpenDKIM und setzt das Postfix-Relay. `dns-check` bewertet PTR, SPF, DKIM und DMARC und liefert die TXT-Zeilen, die beim DNS-Anbieter einzutragen sind. `queue/flush` führt `postqueue -f` aus.

## Wartung

```sh
sudo systemctl status godeb godeb-agent
sudo systemctl restart godeb-agent godeb
sudo journalctl -u godeb -u godeb-agent -e
sudo godeb user passwd admin
sudo godeb 2fa reset admin
sudo tail -f /var/log/godeb/auth.log
sudo godeb site add heim-vm                      # Hub: Standort anlegen, zeigt den Beitrittscode
sudo godeb site sync                             # Hub: Standort-Weiterleitungen und Trusted Endpoints abgleichen
sudo godeb trust enable                          # Heim: Trusted Endpoint für Auto-Disconnect
sudo godeb trust show
sudo nft list table ip godeb_portforward         # Hub: aktive Portweiterleitungen
```

### Produktivbetrieb (Checkliste)

Einmal vor dem Produktivbetrieb abhaken, danach nach jeder größeren Änderung. Hintergrund: `ROADMAP.md`, „Produktionsreife“.

- **Hetzner-Backups** für den VPS einschalten (Server → Backups; täglich, 7 Stände) und **vor jedem GoDeb-Update einen Snapshot** ziehen. Zu Hause dasselbe mit Proxmox (Backup-Job plus Snapshot vor Updates). Ein tägliches Backup heißt: Mails und Spielstände der letzten bis zu 24 Stunden können fehlen.
- **Hetzner Cloud Firewall** als zweite Schicht vor ufw, eingehend nur: `22/tcp`, `51820/udp`; mit Mailserver `25/tcp`, `465/tcp`, `587/tcp`, `993/tcp`; mit Webseiten oder Zertifikaten `80/tcp`, `443/tcp`; mit Portweiterleitung die Spielports (Enshrouded `15636-15637/udp`). `8080`, `5432` und `6379` nie.
- **SSH nur mit Schlüssel** (siehe „SSH und Updates“ oben). `godeb doctor` und die Ampel melden Passwort-Login als Warnung.
- **Mail** vorab: A-Eintrag `mail.<domain>`, Reverse-DNS in der Hetzner-Konsole auf `mail.<domain>`, Freigabe von ausgehend `25/465` bei Hetzner beantragen. Danach mail-tester.com: mindestens 9/10.
- **Alarme** einschalten (Überwachung → Alarme): ntfy-Thema auf beiden Servern, Mail zusätzlich. Die Heim-VM meldet einen ausgefallenen VPS („Hub von außen“), aber nur per ntfy zuverlässig.
- **Externe Prüfung** der öffentlichen Dienste (Mail-Ports, Webseiten) mit einem Uptime-Dienst.
- **Nur gebrauchte Plugins** eingeschaltet lassen (Plugins → Schalter).
- **Notfallzugang testen**: Backup-Codes ausgedruckt, SSH-Schlüssel auf einem zweiten Gerät, einmal `sudo godeb 2fa reset` an einem Testkonto ausprobiert.
- **Updates**: Produktivserver nur aus `trixie` (stabile Releases). Vorabversionen höchstens auf der Heim-VM (`godeb-testing.sources`, siehe „APT-Repo“).

### Notfall

Alle Befehle per SSH mit einem Konto mit `sudo`. Geht SSH nicht mehr: Hetzner Cloud Console → Server → „Konsole“ (bzw. Proxmox → VM → Console), dort mit dem Passwort eines lokalen Kontos anmelden.

**Handy mit der 2FA-App verloren:** Ein Backup-Code ersetzt den TOTP-Code bei Anmeldung und Aktionen. Neu einrichten geht nur per SSH:

```sh
sudo godeb 2fa reset admin          # nächste Anmeldung richtet 2FA neu ein
sudo godeb sessions revoke admin    # alle offenen Sitzungen beenden
```

**Weboberfläche nicht erreichbar:**

```sh
sudo wg show                        # Handshake mit dem Handy? Sonst Tunnel/Endpoint/ufw 51820
sudo systemctl status godeb godeb-agent
sudo journalctl -u godeb -u godeb-agent -e
sudo godeb doctor                   # Gesamtbefund, ohne Geheimnisse
sudo systemctl restart godeb-agent godeb
```

Über SSH geht es auch ohne Tunnel: `ssh -L 8080:127.0.0.1:8080 <benutzer>@<server-ip>`, dann `http://127.0.0.1:8080` im Browser.

**Update hat etwas kaputt gemacht:** Am sichersten den Snapshot von vor dem Update zurückspielen. Ältere Versionen bleiben im APT-Repo, ein Zurückgehen ist also möglich (`apt policy godeb` zeigt sie, `sudo apt install --allow-downgrades godeb=<version>`). Datenbanken werden aber nur vorwärts migriert, deshalb ist das nur eine Notlösung bis zum Fix-Release.

**`godeb doctor` meldet eine beschädigte Datenbank:** Server aus dem letzten Backup bzw. Snapshot zurückholen. `master.key` und Datenbank gehören zusammen; einzeln zurückkopiert passen die versiegelten Felder nicht mehr.

**VPS ist weg (gelöscht, kompromittiert):** neuen VPS aus dem Hetzner-Backup erstellen; die IP ändert sich dabei meist. Danach: Endpoint in den Client-Dateien anpassen (WireGuard → Clients neu herunterladen bzw. QR neu scannen), Heim-VM neu verbinden (am Hub Standort neu anlegen; auf der Heim-VM `/etc/wireguard/wg0.conf` beiseitelegen, denn `godeb setup --join -` ersetzt eine abweichende Datei nicht, dann den neuen Beitrittscode einfügen), DNS-Einträge (Domains → Anwenden) und Reverse-DNS auf die neue IP. Ohne Backup: frisch installieren, Ersteinrichtung als Hub, Standort neu anlegen, Postfächer neu anlegen (Mails sind dann verloren). Bei Verdacht auf Einbruch nie das alte System weiterverwenden, sondern neu aufsetzen und alle Passwörter, das Hetzner-API-Token und die WireGuard-Schlüssel erneuern.

**Heim-VM ist weg:** Proxmox-Backup zurückspielen. Ohne Backup: Debian 13 neu, GoDeb installieren, am Hub den Standort löschen und neu anlegen, mit dem neuen Beitrittscode verbinden, Enshrouded neu installieren (Spielstände aus dem Backup nach `/var/lib/godeb-steam/enshrouded/`).

### Logs

Beide Prozesse schreiben strukturiert (`log/slog`) ins systemd-Journal, mit Stufe als Journal-Priorität. Eine Zeile sieht so aus:

```
level=INFO msg=Aktion komponente=agent methode=POST pfad=/api/plugins/wireguard/peers status=200 dauer=412ms benutzer=admin ip=10.8.0.5 2fa=bestätigt
```

- **Aktionen**: Jede ändernde Anfrage loggt der Agent selbst mit Benutzer, Pfad, Status, Dauer und 2FA-Ergebnis (`verlangt`, `bestätigt`, `falsch`, `gesperrt`). Abgelehnte Anfragen kommen als Warnung, Serverfehler als Fehler, beide mit dem Grund (`fehler=…`, dieselbe Meldung wie in der Oberfläche, gekürzt auf 300 Zeichen).
- **Hintergrundjobs** (Spielserver installieren/aktualisieren, Mailserver installieren): Start, Ende und Dauer; bei einem Fehler zusätzlich der Grund und die letzten acht Zeilen des Job-Logs (`letzte_zeilen`), denn das Job-Log in der Oberfläche überlebt keinen Neustart. Eine übernommene Weboberfläche kann die IP falsch angeben, aber nicht verbergen, dass eine Änderung lief.
- **Befehle**: Fehlgeschlagene Befehle als Warnung mit Exit-Code und gekürztem stderr, erfolgreiche nur bei `debug`. stdin (Secrets) wird nie geloggt. Fehlende Werkzeuge bei reinen Abfragen und Zustandsabfragen, deren Exit-Code die Antwort ist (`dpkg-query` für nicht installierte Pakete, `systemctl is-enabled`, `nft list table` einer noch nicht angelegten Tabelle), bleiben auf `debug` (`core.RunAdminQuiet`), damit Warnungen im Journal echte Probleme sind.
- **Anmeldungen**: Erfolg, Abmeldung und Fehlversuch mit Benutzer und IP. Fehlversuche stehen weiterhin auch in `/var/log/godeb/auth.log` (für fail2ban).
- In der Oberfläche zeigt **Protokoll** dieselben Einträge mit Filtern.

```sh
sudo journalctl -u godeb -u godeb-agent -f               # live
sudo journalctl -u godeb -u godeb-agent -p warning -b    # nur Warnungen und Fehler seit Boot
sudo journalctl -u godeb-agent -g 'msg=Aktion' --since today
sudo journalctl -u godeb -u godeb-agent -g 'fehler=' --since '-1h'   # was ist in der letzten Stunde schiefgegangen
sudo journalctl -u godeb-agent -g 'Job' --since today                # Installationen und Updates
sudo systemctl edit godeb-agent   # [Service] Environment=GODEB_LOG_LEVEL=debug, danach restart
```

Paket bauen, aus dem Repository:

```sh
./scripts/build-deb.sh
sudo apt install --no-install-recommends ./godeb_*_amd64.deb
```

### Update auf die Version mit Agent

Ab dieser Version läuft die Weboberfläche nicht mehr als root. `postinst` erledigt den Umzug: Es kopiert `godeb.db`, `master.key`, `auth/`, `setup.json`, `setup-token`, `access.json` und `tls/` von `/etc/godeb` nach `/var/lib/godeb` (Besitzer `godeb`), richtet `godeb-agent.service` ein und startet beide Dienste neu. `/etc/godeb` bleibt unverändert und gehört weiter root; ein Downgrade funktioniert daher. Beim ersten Start entfernt die Weboberfläche die 2FA-Geheimnisse aus ihrer Kopie der Datenbank, sobald der Agent dieselben Einrichtungen bestätigt. Anmeldung und 2FA bleiben gleich.

Prüfen nach dem Update:

```sh
systemctl status godeb godeb-agent
ps -o user=,cmd= -C godeb          # godeb /usr/bin/godeb, root /usr/bin/godeb agent
ls -l /run/godeb-agent/agent.sock  # srw-rw---- root godeb
```

### Versionen und Updates

`scripts/version.sh` bestimmt die Version: Ein Commit mit Tag `vX.Y.Z` ist genau `X.Y.Z`, jeder andere `X.Y.Z+git<Anzahl Commits>.<Hash>` (mit `.dirty` bei nicht committeten Änderungen). Debian sortiert das immer über den vorigen Build, ein neues Paket ist für apt also ein Update. Die Version steht im Paketnamen, in `godeb version`, in `/api/health` und im Benutzermenü oben rechts. Ein Release entsteht, wenn ein Tag gepusht wird:

```sh
git tag v0.2.0 && git push origin v0.2.0   # CI baut und veröffentlicht das .deb
```

### Updates (Plugin APT)

Der Agent sucht standardmäßig alle 10 Minuten nach neuen Paketen (`apt-get update`) und installiert danach ausstehende **Sicherheitsupdates** selbst. Als Sicherheitsupdate gilt ein Paket, dessen neue Version aus einer Sicherheitsquelle kommt (`trixie-security` in `apt list --upgradable`); installiert wird es mit `apt-get install --only-upgrade`. Alles andere, auch GoDeb selbst, bleibt beim Knopf „Alle aktualisieren“ (`apt-get --with-new-pkgs upgrade`, geänderte Konfigurationsdateien bleiben erhalten, frischer 2FA-Code). Intervall (5–1440 Minuten) und beide Automatiken lassen sich im Plugin abschalten; sie stehen in `/etc/godeb/apt.json`. Ein neuer Kernel wird installiert, aber nicht neu gebootet.

Jeder Lauf ist die eigene Unit `godeb-apt.service` (`systemd-run`, führt `godeb apt-run check|security|upgrade` aus), nie ein Kindprozess des Agenten. Aktualisiert sich GoDeb selbst, startet `postinst` den Agenten neu; ein Kindprozess würde dabei mitten in `dpkg` abgebrochen. Die Unit läuft weiter, schreibt Log und Ergebnis nach `/var/lib/godeb-agent/apt/` (nur root), und die Oberfläche zeigt beides nach dem Neustart wieder an. Es läuft immer nur ein Lauf; hält gerade Debians eigener `apt-daily` die Sperre, wartet `apt-get` bis zu fünf Minuten. Von Hand: `sudo godeb apt-run security`.

### APT-Repo

Das Repo liegt auf GitHub Pages unter `https://zerodos1988.github.io/GoDeb` (Komponente `main`, `amd64` und `arm64`) mit zwei Suiten: `trixie` enthält nur Releases, `trixie-testing` zusätzlich alle Vorabversionen. Solange es noch kein Release ohne Vorab-Kennzeichen gibt (vor 1.0), enthält auch `trixie` die Vorabversionen. Ein neuer Tag wird immer als Vorabversion veröffentlicht; erst das Entfernen des Häkchens „Set as a pre-release“ auf GitHub nimmt ihn in `trixie` auf (der Workflow baut das Repo dann neu). Zum Testen auf einem Server zusätzlich einbinden: `sudo wget -O /etc/apt/sources.list.d/godeb-testing.sources https://zerodos1988.github.io/GoDeb/godeb-testing.sources && sudo apt update`. Nach jedem Release baut der Workflow `apt-repo.yml` es aus allen veröffentlichten Releases neu (`scripts/build-apt-repo.sh`: `apt-ftparchive`, signiert mit `sq`, vor dem Hochladen mit `sqv` gegen den mitgelieferten Schlüssel geprüft). Von Hand neu veröffentlichen: Actions → „APT-Repo“ → „Run workflow“. Vorabversionen landen mit im Repo; ein gelöschtes Release verschwindet beim nächsten Lauf.

- **Paket:** Es bringt `/etc/apt/sources.list.d/godeb.sources` (Conffile) und `/usr/share/keyrings/godeb-archive-keyring.asc` mit. Die Dateien auf der Pages-Seite sind dieselben, wer sie vorher von Hand anlegt, bekommt beim Installieren keine Rückfrage.
- **Schlüssel:** Primärschlüssel `85BB 4B5E C4EB BC47 DCD1  6D32 FF4E 70E4 D1A7 EBB9` (nur Zertifizierung, bis 2036), Signier-Unterschlüssel `C59E 5F9F EE98 E052 817E  B9AC B93E E13B A71E 8187` (bis 22.10.2028). Die CI hat nur den Unterschlüssel (Secret `APT_SIGNING_KEY`). Vor Ablauf den Unterschlüssel offline verlängern oder ersetzen und die neue Fassung von `package/apt/godeb-archive-keyring.asc` mit einem Release ausliefern.
- **Lokal bauen:** `scripts/build-apt-repo.sh <verzeichnis-mit-debs> <ausgabe> <schlüsseldatei>`.

### Plugins abschalten

Alle Plugins werden mitgeliefert. Im Plugin-Manager lässt sich jedes abschalten: Es verschwindet aus der Navigation, seine Routen unter `/api/plugins/<id>/` antworten mit 404, und GoDeb erledigt dafür nichts mehr im Hintergrund (automatische Updates, Überwachung). Die Systemdienste dahinter bleiben unberührt: Abschalten von WireGuard oder UFW stoppt weder den Tunnel noch die Firewall, und ein nicht installierter Dienst (Postfix beim Mail-Gateway) verhindert das Abschalten nicht mehr. Der Plugin-Manager selbst bleibt erreichbar, um es wieder einzuschalten. Der Zustand steht in der Tabelle `plugin_states` (`disabled`) der Agent-Datenbank. Auf dem VPS etwa „Spielserver“ abschalten, auf der Heim-VM „Portweiterleitung“.

Postfix und OpenDKIM richtet nicht das Paket-Skript ein, sondern die Mail-Gateway-Ansicht, nachdem die Relay-Domain eingetragen ist. Fehlt eines der Pakete, installiert GoDeb sie nicht selbst. Auf dem Host:

```sh
sudo apt-get install postfix opendkim opendkim-tools pflogsumm
```

Queue und Signatur von Hand prüfen:

```sh
mailq
sudo postqueue -f
sudo postconf -h relayhost
sudo opendkim-testkey -d example.org -s godeb -vvv
```

`pflogsumm` fasst `/var/log/mail.log` zusammen, wenn das Log vorhanden ist. Die Queue-Wahrheit bleibt `mailq` beziehungsweise `/var/spool/postfix`.

## Entwicklung

```sh
cd frontend && npm ci && npm run build
CGO_ENABLED=0 go test ./backend/... ./plugins/...
CGO_ENABLED=0 go build -tags netgo -o /tmp/godeb ./backend
```

`go.mod` pinnt die Toolchain (`toolchain go1.26.10`); ein älteres lokales Go lädt sie automatisch. Tests setzen `GODEB_DB` und `GODEB_AUTH_DIR` auf temporäre Pfade. Der Dienst auf Port `8080` ist die Installation unter `/usr/bin/godeb` und wird von Tests nicht ersetzt.
