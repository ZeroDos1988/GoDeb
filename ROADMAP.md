# Fahrplan

Stand: 24.10.2026. Reihenfolge = Priorität. `[x]` erledigt, `[ ]` offen. Neue Ideen kommen hierher (passende Phase oder Phase 7), getroffene Entscheidungen unten unter „Entscheidungen“.

## Aktueller Stand (25.10.)

- **Veröffentlicht** (alle öffentlich, nur unter ZeroDos1988):
  - `ZeroDos1988/GoDeb`: GPL-3.0, Release **v0.2.3** (Vorabversion, 25.10.), das erste aus dem APT-Repo `https://zerodos1988.github.io/GoDeb`.
  - `ZeroDos1988/godeb-tunnel-android`: Apache-2.0, Release **1.1.0** mit signierter `godeb-tunnel.apk`.
  - `ZeroDos1988/godeb-tunnel-arch`: GPL-3.0, Release **1.0.1**.
  - Privat bleibt nur `godeb-wireguard-windows`.
- **Lokal:**
  - Repos unter `~/Projects/` (`GoDeb`, `godeb-tunnel-android`, `godeb-tunnel-arch`, `godeb-wireguard-windows`).
  - Alte Historien (mit Klarnamen, nicht veröffentlichen) und ein Export der alten GitHub-Releases unter `~/Projects/Archiv/`.
- **Auf den Servern:** Der Hetzner-VPS ist am 25.10. frisch aus dem APT-Repo mit v0.2.3 installiert. Rückmeldung: Installation lief; die gedruckte Übersicht war leer (behoben, siehe unten); weitere Clients und der Standort sollen gleich im Assistenten entstehen (umgesetzt, siehe Phase 8).
- **Nächste Schritte, in dieser Reihenfolge:**
  1. Servertest von v0.2.5 (veröffentlicht 25.10.) auf VPS und Heim-VM: Benutzermenü und Sprache, Standort per Code, Übersicht und Backup-Codes drucken, Überwachung, Alarme.
  2. Auf dem Server die APK als `/etc/godeb/android/godeb-tunnel.apk` ablegen.
  3. Phase 8: Heim-VM als Gateway (Route im Router) und optional Ausgang ins Internet über den Hub.
  4. Windows-Client umbenennen (ProductName, Installer, Dienste) und veröffentlichen.
  5. ~~APT-Repo über GitHub Pages~~ (24.10. umgesetzt, Paket bringt Quelle und Schlüssel ab v0.2.3 mit).

## Leitplanken für alle Phasen

- **Erreichbarkeit nur über den Tunnel.** Die Web-UI lauscht auf Loopback und `wg0`, nie im Klartext auf LAN/WAN.
- **Handy first.** Jede Ansicht muss ab 360 px ohne horizontales Scrollen bedienbar sein. Breite Tabellen scrollen in ihrem Container, Formulare sind unter `md` einspaltig. Neue Ansichten werden vor dem Merge in der Mobilansicht geprüft.
- **Eine Logik für CLI und UI.** Was `godeb setup` anlegt, entsteht über dieselben Funktionen wie in der Web-UI und erscheint dort sofort.

## Phase 1 – Grundlage (erledigt, 19.10.)

- [x] Bind-Adresse: `127.0.0.1:8080,wg0:8080`; `wg0` wird gebunden, sobald das Interface steht (Retry, kein Neustart nötig).
- [x] systemd-Härtung der GoDeb-Unit (Score 9.2 → 7.4).
- [x] Go-Toolchain 1.26.9 (seit 21.10. 1.26.10), `golang.org/x/text` 0.39, `go-jose` 4.1.4: `govulncheck` meldet 0 erreichbare Schwachstellen (vorher 21).
- [x] `godeb setup`: Admin, `wg0`, UFW, erster Client als QR im Terminal; der Client steht danach in der Web-UI.
- [x] Peer-Anlage als gemeinsame Funktion `wireguard.CreatePeer` (Web-UI und CLI).
- [x] Endpoint-Vorschlag für neue Peers aus vorhandenen Clients statt aus der Browser-Adresse.
- [x] Mobil: Seitenbreite auf dem Handy repariert (Navigation blähte die Seite auf ~1070 px auf).
- [x] `build-deb.sh` packt jetzt auch `plugin.json` (vorher war der Plugin-Manager im Paket leer).
- [x] Enshrouded-Plugin: Ein-Klick-Installation, Start/Stop/Update, Konfiguration mit Umschalter Formular ↔ Datei-Editor, Server-Log.

## Phase 2 – Testen auf der Heim-VM

1. Paket aus den [Releases](https://github.com/ZeroDos1988/GoDeb/releases) (`v0.1.0` am 22.10., `v0.2.0` mit Logging und Überwachung am 23.10.; das Repo ist privat, also per Browser oder `gh release download` laden) oder selbst gebaut installieren: `apt install --no-install-recommends ./godeb_*_amd64.deb`.
2. Ersteinrichtung im Browser (Link aus der Installation bzw. `godeb setup-url`) und den QR-Code des ersten Clients mit der WireGuard-App scannen.
3. Über den Tunnel `http://10.8.0.1:8080` am Handy öffnen. Login, 2FA, der Peer aus dem Setup muss in „WireGuard“ stehen.
4. Enshrouded → „Server installieren“ und das Job-Log beobachten. Danach im Spiel mit `<IP>:15637` verbinden.
5. Formular ↔ Datei umschalten, etwas ändern, „Speichern und neu starten“.

Offene Punkte, die beim Test auffallen können:

- [x] SteamCMD und Serverdownload lokal ohne root getestet (20.10.): Die Download-URL war falsch, und `app_update` scheitert beim ersten Lauf mit „Missing configuration“. Beides ist behoben (drei Versuche).
- [x] API-Ablauf mit Dev-Instanz getestet: Login, 2FA-Einrichtung, Fenster und kritische Aktionen, Replay-Schutz, fremder Origin, Logout.
- [ ] Wine unter Debian 13: Läuft `enshrouded_server.exe` ohne X-Server? Falls nicht, `xvfb-run` als Option im Unit-Template.
- [ ] Enshrouded-Ports: Neuere Serverversionen kennen nur `queryPort`. GoDeb öffnet zusätzlich `queryPort-1`; nach dem Test ggf. entfernen.

## Phase 3 – VPS-Reife (Pflicht vor dem produktiven Einsatz auf Hetzner)

Eine Testinstallation auf einem Hetzner-VPS ist ab 20.10. möglich. Die Anleitung steht in `DOCUMENTATION.md` unter „Installation auf einem Hetzner-VPS“. Die Weboberfläche läuft seit 22.10. ohne root; offen sind vor allem der Test auf den echten Servern und die Punkte unten.

- [x] **Sitzungen widerrufbar**: Sitzungen liegen in SQLite. Logout, „Sitzungen“ in der UI und `godeb sessions revoke` beenden sie serverseitig.
- [x] **TOTP-Versuchslimit im Code**: 5 Versuche pro Pending-Login und pro Sitzung.
- [x] **2FA für jede Admin-Aktion**: Jede schreibende `/api`-Anfrage braucht einen Code. Kritische Aktionen verlangen jedes Mal einen neuen, die übrigen gelten 5 Minuten.
- [x] **Security-Header**: CSP ohne Inline-Skripte, `X-Frame-Options: DENY`, `Referrer-Policy`, `Permissions-Policy`.
- [x] **CI**: `go test`, `go vet`, `govulncheck`, `npm audit`, Frontend-Build, `.deb` als Artefakt, `systemd-analyze security` (Weboberfläche Schwelle 2.0, Agent 8.0).
- [x] **Setup schaltet ufw ein**, aber erst, nachdem der SSH-Port freigegeben ist. Ohne erkannten SSH-Port bleibt ufw aus.
- [x] **Ersteinrichtung im Browser** (21.10.): Admin mit Einmal-Token, 2FA, Rolle (Hub, Peer, ohne WireGuard), Erreichbarkeit (Tunnel, Heimnetz per HTTPS, lokal) und Firewall; der öffentliche Einrichtungsport schließt sich danach. Kein WireGuard-Server mehr ohne Auswahl.
- [x] **Plugins abschaltbar** (21.10.): aus Navigation und API entfernt, Dienste gestoppt.
- [x] **Versionen** (21.10.): aus Git-Tags, apt erkennt jeden Build als Update, Release per Tag.
- [x] **Weboberfläche ohne root** (22.10., „Weg 2“): `godeb.service` läuft als `godeb` mit `ProtectSystem=strict`, ohne Capabilities (systemd-analyze 1.3 statt 7.4). Systemänderungen macht `godeb-agent.service` (root, nur Unix-Socket mit Peer-Prüfung). Getrennte Datenbanken, damit die Weboberfläche keine Daten schreibt, nach denen der Agent handelt. Statt sudoers-Allowlist, weil die Plugins zu viele verschiedene Befehle brauchen.
- [x] **2FA beim Agenten** (22.10., „Weg 2+“): TOTP-Geheimnisse nur beim Agenten, der jede weitergeleitete Änderung selbst mit Code prüft (eigenes Fenster pro Sitzung, Sperre gegen Durchprobieren). Neue 2FA-Konten nur mit Freigabe durch root. Eine übernommene Weboberfläche kann damit nur lesen.
- [x] **Einrichtungsübersicht** (23.10., Wunsch nach dem ersten echten Setup): Link zur Einrichtung nach `apt install` als Kasten; am Ende des Assistenten eine Übersicht aller Einstellungen (Adressen mit Tunnel-IP, Tunnel, Firewall), druckbar bzw. als PDF; später unter Startseite → Einrichtungsübersicht und `godeb summary`.
- [x] **Logging** (23.10.): `slog` ins Journal mit Prioritäten, `GODEB_LOG_LEVEL`. Der Agent protokolliert jede Aktion mit Benutzer, Status und 2FA-Ergebnis, fehlgeschlagene Befehle kommen mit Exit-Code. Ansicht „Protokoll“ (Plugin `logs`).
- [ ] **Agent auf den echten Servern testen**: Update von der Version davor (Umzug nach `/var/lib/godeb`), Assistent, Trusted Endpoint, Steam-Installation.
- [ ] **Client-Schlüssel**: Option „Schlüssel auf dem Gerät erzeugen“ (Public Key einfügen). Client-Dateien nach dem Download optional löschen. Mailversand nur mit Warnhinweis.
- [x] ~~**Master-Key** getrennt von der Datenbank~~ (entschieden 21.10.): entfällt. Gesichert wird die ganze VM (Proxmox bzw. Hetzner), ein VM-Backup enthält Schlüssel und Datenbank ohnehin zusammen; ein systemd-Credential läge auf derselben Platte.
- [ ] **Volltunnel**: IP-Forwarding und NAT über `ufw` (`before.rules`, `route allow`) als bewusste Option im Setup.
- [ ] **Unauthentifizierter APK-Download**: nur über den Tunnel erreichbar (Bind-Adresse); optional mit Token. (`/api/trusted` ist seit 20.10. entfernt.)
- [x] ~~**Backups**~~ (entschieden 21.10.): nicht Aufgabe von GoDeb. Proxmox (Heim-VM) und Hetzner (VPS) sichern die ganze Maschine; vor größeren Änderungen einen Snapshot ziehen.
- [ ] VPS-Spezifika: SSH nur per Schlüssel und unattended-upgrades prüft bzw. installiert das Setup noch nicht (steht bisher nur in der Anleitung).
- [ ] Paket auch für `arm64` (Hetzner CAX).
- [ ] **Updates aus der Oberfläche** (Wunsch 25.10.): Das APT-Plugin zeigt bisher nur `apt list --upgradable` aus dem vorhandenen Paket-Cache und kann nichts installieren; ohne `apt update` erscheint eine neue GoDeb-Version dort nicht. Ziel: „Paketlisten aktualisieren“ (`apt-get update`) und „Alle aktualisieren“ (`apt-get upgrade`) im Agenten, mit frischem 2FA-Code, Fortschritt und Log in der Oberfläche. GoDeb aktualisiert sich dabei selbst: `postinst` startet den Agenten neu, der das Update ausführt. Deshalb läuft der Vorgang in einer eigenen systemd-Unit (`systemd-run`), die den Neustart überlebt; die Oberfläche verbindet sich danach neu und zeigt das Ergebnis.
- [x] **Eigenes APT-Repo** (24.10.): signiertes Repo auf GitHub Pages (`zerodos1988.github.io/GoDeb`, Suite `trixie`), nach jedem Release aus allen Releases neu gebaut (`apt-repo.yml`). Das Paket bringt `godeb.sources` und Schlüssel mit, danach kommt GoDeb mit `apt upgrade`. Die CI hat nur den Signier-Unterschlüssel (bis 2028).

## Phase 3b – Standortverbindung VPS ↔ Heim-VM (umgesetzt 20.10.)

Ziel ist das Zielbild aus `DOCUMENTATION.md` („Zielbild: VPS und Heim-VM“): Der Hetzner-VPS ist öffentlicher Eingang und WireGuard-Hub. Die Heim-VM hängt als Peer `10.8.0.2` daran und betreibt Enshrouded. Der VPS leitet `15636–15637/udp` an sie weiter. Beide laufen mit demselben Paket, nur mit anderen Plugins.

- [x] **Setup-Rolle „Peer“**: `godeb setup --join <datei>` und „Mit einem Hub verbinden“ in der UI, beides über `wireguard.JoinHub`. Strenge Prüfung der Datei (kein `PostUp` & Co., kein Volltunnel). Die Firewall öffnet nur auf `wg0` `8080/tcp`.
- [x] **Standort-Peer** (Hub): `godeb site add` bzw. „Standort“ beim neuen Peer. Nur das Tunnelnetz, kein DNS, Keepalive 25, Kennzeichen in der UI.
- [x] **Plugin Portweiterleitung** (Hub): eigene nftables-Tabelle (DNAT plus SNAT), `ufw route allow` per Kommentar abgeglichen, Boot-Unit `godeb-portforward.service`, frischer 2FA-Code bei jeder Änderung. Getestet in Netzwerk-Namespaces.
- [x] **Hub-Weiterleitung zwischen Peers**: Handy → `<standort>:8080/tcp`, sonst nichts (`GoDeb-Hub`-Regeln, `godeb site sync`).
- [x] **Enshrouded hinter dem Hub**: Ports nur auf `wg0`; die UI zeigt die Hub-Adresse und welche Weiterleitung nötig ist.
- [ ] **Test auf echten Servern**: Handy → VPS-UI und Heim-UI, Spiel-Client → `<vps-ip>:15637`, Tunnel trennen und wieder verbinden (Keepalive), Neustart beider Seiten (Tabelle und ufw-Regeln kommen wieder).
- [ ] Später: echte Spieler-IPs per Policy-Routing auf der Heim-VM statt SNAT.
- [ ] Später: IPv6-Weiterleitung (heute nur IPv4).

Die Portweiterleitung schreibt `/etc/sysctl.d/90-godeb-forward.conf`, `/etc/godeb/portforward.*` und ruft `nft`, `sysctl` und `ufw route` auf; das erledigt der root-Agent.

## Phase 4 – Spielserver und Enshrouded ausbauen

- [ ] Formular für `gameSettings` beim Preset „Custom“ (wichtigste Faktoren mit Schiebereglern).
- [ ] Savegame-Backup vor jedem Update und auf Knopfdruck, Wiederherstellen aus der UI.
- [ ] Zeitgesteuerte Updates/Neustarts (systemd-Timer) mit Warnfenster.
- [ ] Spielerzahl/Status per Steam-Query anzeigen.
- [x] Gemeinsames Gerüst „SteamCMD-Spiel“ (20.10.): Plugin `steam` mit Spielen als `Game`-Beschreibung, Enshrouded ist das erste. Weitere Spiele brauchen nur App-ID, Startbefehl, Ports und optional Konfiguration und Formular.

## Phase 4b – Überwachung (umgesetzt 23.10., Plugin `monitor`)

Ziel: auf einen Blick sehen, ob der Server gesund ist, ob der Traffic normal aussieht und wer wann zugegriffen hat. Gebaut als Plugin `monitor` mit Statuskachel auf der Startseite statt Netdata oder Prometheus/Grafana, weil die eigenes Login und eigene Ports brauchen, auf dem kleinen VPS viel RAM kosten und GoDeb-Begriffe (Peers, Spiele, Weiterleitungen) nicht kennen. Plugins liefern ihre Messwerte über eine kleine Schnittstelle im Kern („Metrik-Quellen“).

- [x] **M1 Systemzustand**: CPU, RAM, Platte, Load, Netz pro Interface; live und Verlauf 24 h/30 Tage. Ampel mit Gründen: fehlgeschlagene Units, Updates, Neustart nötig, Zertifikat läuft ab, Platte voll. Sammler in der Weboberfläche (`/proc`, ohne root); was nur root lesen darf, über eine interne Agent-Route. Verlauf in eigener Datei `/var/lib/godeb/metrics.db` mit Ausdünnung (10 s für 24 h, 5 min für 30 Tage, 1 h für 1 Jahr).
- [x] **M2 Netz und Zugriffe**: Traffic pro WireGuard-Peer (`wg show dump`), pro Dienst bzw. Port über Zähler in einer eigenen nftables-Tabelle (zählt nur, blockt nichts), Live-Verbindungen (conntrack, Top-Gegenstellen). Zugriffs-Zeitleiste aus GoDeb-Anmeldungen und -Aktionen, SSH-Logins, fail2ban-Sperren und ufw-Blocks.
- [x] **M3 Spielserver**: Spielerzahl per Steam-Query (A2S, UDP, ohne Abhängigkeit), Traffic pro Spielport. Ereignisse (Update, Neustart, Installation) als Markierung im Diagramm, damit mehr Traffic beim Update erklärt ist.
- [x] **M4 Normalverhalten und Alarme**: Grundlinie pro Wochenstunde (Median der letzten Wochen), Abweichungen markieren. Benachrichtigung per Mail (Mail-Gateway) bzw. Push an die App. Standortübersicht: Der Hub zeigt den Zustand der Heim-VM über den Tunnel.
- Entschieden: Verlauf 26 h roh, 31 Tage 5-Minuten-Mittel, 400 Tage Stundenmittel; der Hub zeigt die Heim-VM; Alarme per Mail und ntfy.
- [ ] Auf den echten Servern prüfen: Zähler-Tabelle `inet godeb_acct` neben ufw und `godeb_portforward`, Steam-Query von Enshrouded, Standort-Abfrage Hub → Heim-VM, Schwellen der Grundlinie nach ein paar Wochen.
- [ ] Push direkt in die GoDeb-App statt ntfy (braucht einen Push-Dienst).

## Phase 5 – GoDeb-Tunnel-Clients (Android, Windows, Arch)

- [x] **Umbenannt in „GoDeb Tunnel“** (23.10.): Die Markenrichtlinie von WireGuard verbietet „WireGuard“ im Produktnamen. Android und Arch sind öffentlich; App-ID `de.godeb.wireguard` und Signierschlüssel bleiben (Updates). GoDeb liefert `godeb-tunnel.apk` aus und nimmt sonst die alte `godeb-wireguard.apk`.
- [ ] **Windows umbenennen und veröffentlichen**: ProductName „WireGuard“, Installer, Dienstnamen und Texte ersetzen, auf Windows testen, Repo wie Android/Arch bereinigen (Commit mit Klarnamen umschreiben) und als `godeb-tunnel-windows` veröffentlichen. Den vorgebauten `wireguard.dll` (WireGuard-NT) unverändert lassen; seine Lizenz erlaubt die Weitergabe nur so.

- [x] Challenge-Response statt reinem HTTP-Header (20.10.): Die App schickt eine Nonce, die Heim-VM signiert sie (ECDSA P-256 statt Ed25519, weil Android P-256 ohne Zusatzbibliothek prüft). Der Public Key steht als `# GoDeb-Trusted-Key:` in der Client-Konfiguration. Die alte Header-Prüfung `/api/trusted` ist entfernt, ebenso die Subnetz-Suche in der App.
- [x] Auto-Disconnect sicher: Ohne gepinnten Schlüssel bleibt es aus, mit Schlüssel wird es beim Import automatisch eingeschaltet.
- [ ] Test am echten Handy: im Heim-WLAN trennt der Tunnel, im Mobilnetz verbindet er wieder; ein anderes WLAN mit gleicher IP gilt nicht als Zuhause.
- [x] Release-APK signiert (20.10., Schlüssel außerhalb des Repos); liegt bereit zum Ablegen unter `/etc/godeb/android/`.
- [ ] Android: `usesCleartextTraffic` einschränken. Für die Vertrauensprüfung nicht mehr nötig, weil die Antwort signiert ist; allgemeine Härtung.
- [ ] **Windows/Arch: dasselbe Protokoll `godeb-trust-v1`** (Probe an das physische Interface binden, keine Redirects, Signatur prüfen). Dringend: Beide prüfen noch den alten Header `X-GoDeb-Trusted`, den GoDeb nicht mehr liefert. Auto-Disconnect funktioniert dort derzeit nicht.
- [ ] **Trusted Endpoint ohne GoDeb zu Hause** (Idee 22.10.):
  - eigenständiges Programm `godeb-trustd` (Linux und FreeBSD) für jedes Gerät, das zu Hause immer läuft (Proxmox-Host, NAS, Raspberry Pi);
  - auf dem Hub einen externen Trusted Endpoint eintragen (Adresse und Schlüssel, UI und `godeb trust add-external`), der in Client-Dateien und QR-Codes landet;
  - danach **OPNsense-Plugin `os-godeb-trust`** (Einstellungsseite, Dienst, Schlüssel in der Konfigurationssicherung). Paket für den Eigenbedarf per Hand installieren; eigenes Paketlager oder offizielle Aufnahme später.

## Phase 6 – Mobile Feinschliff

- [ ] Kompakte Kopfzeile mit Menü-Button statt der vollen Seitenleiste auf dem Handy.
- [x] **Benutzermenü wie bei GitHub** (Wunsch und Umsetzung 25.10.): Avatar oben rechts mit Sprache, Darstellung, Sitzungen, Einrichtungsübersicht, Abmelden und Version. Die Sprache folgt standardmäßig dem Browser („Auto“); Login und Ersteinrichtung haben eine eigene Sprachwahl.
- [ ] Touch-Ziele mindestens 44 px, Tabellen als Karten unter `sm` (WireGuard-Peers, UFW-Regeln).
- [ ] Playwright-Smoke-Test mit iPhone- und Pixel-Viewport in CI (prüft `scrollWidth == innerWidth`).

## Phase 7 – Weitere Plugins (Wunschliste, 20.10.)

- [ ] **Webseiten und Reverse-Proxy** (nginx): eigene Webseiten hosten und interne Dienste veröffentlichen, mit Let's-Encrypt-Zertifikaten (vorhandenes Certbot-Plugin). Auf dem VPS als Eingang, Ziele auch hinter dem Tunnel (z. B. die Heim-VM).
- [ ] **Cron-Aufgaben**: geplante Aufgaben anzeigen und verwalten (gab es im alten Prototyp aus PR #1 als eigene Seite).
- [ ] **Datenbanken**: Datenbankserver installieren (z. B. MariaDB, PostgreSQL), Datenbanken und Benutzer anlegen, dazu ein eingebauter Admin-Client zum Ansehen von Tabellen und Daten. Zugangsdaten mit `security.Seal`, Datenbankports nie öffentlich.

## Phase 8 – Hub und Standorte enger koppeln (Ideen 23.10.)

- [x] **Beitritt per Datei** (23.10.): Standort anlegen lädt die `.conf` herunter, der Assistent der Heim-VM nimmt sie per Upload.
- [x] **Weitere Clients und Standorte im Assistenten, Beitritt per Code** (Wunsch und Umsetzung 25.10.): Auf dem Hub legt „+“ beliebig viele Handys/Laptops (QR und Datei) und GoDeb-Server zu Hause (Beitrittscode) an; `godeb setup` fragt dasselbe ab. Der Beitrittscode ist die Client-Datei als eine Zeile (`godeb-join:` + base64url) und wird in „Mit Hub verbinden“ oder `godeb setup --join -` eingefügt. Der erste Standort bekommt `10.8.0.2`, Geräte lassen die Adresse dafür frei.
- [x] **Druckansichten** (25.10.): Übersicht und Backup-Codes drucken als eigener Bogen (`PrintSheet`), statt der Glas-Oberfläche. Vorher kamen leere Seiten heraus, und beim 2FA-Druck stand der TOTP-Schlüssel mit auf dem Papier.
- [ ] **Gekoppelte Verwaltung**: Hub und Standort vertrauen einander (gegenseitig signierte Anfragen über den Tunnel, kein offener Port). Auf der Heim-VM eine Weiterleitung anlegen, und der Hub legt DNAT und ufw-Freigabe selbst an; Spielserver-Installation auf der Heim-VM schlägt die Weiterleitung am Hub automatisch vor.
- [ ] **Netzwerk-Scanner**: Hosts im Heimnetz bzw. Tunnel finden (ARP/Ping), offene Ports prüfen (nur TCP-Connect und bekannte UDP-Abfragen wie Steam A2S, kein aggressives Scannen), bekannte Dienste erkennen (z. B. „Enshrouded auf 192.168.1.40, 15636–15637/udp“) und per Klick „über den Hub erreichbar machen“ (Weiterleitung Hub → Standort → Zielhost). Nur auf eigenen Netzen, mit Hinweis.
- [ ] **Heim-VM als Gateway ins Tunnelnetz** (Wunsch 23.10.): Geräte im Heimnetz erreichen den Hub und alle Tunnel-Adressen (z. B. `http://10.8.0.1:8080`) ohne eigenen WireGuard-Client. Im Router wird eine statische Route `10.8.0.0/24 → <LAN-IP der Heim-VM>` eingetragen; GoDeb auf der Heim-VM schaltet IP-Forwarding ein, erlaubt Weiterleitung LAN → `wg0` (eigene nftables-Tabelle wie bei `portforward`, mit SNAT auf die Tunnel-Adresse der Heim-VM, damit der Hub ohne Rückroute antwortet) und erweitert das Tunnelnetz in den AllowedIPs bei Bedarf. Die Übersicht nennt die genaue Route für den Router (FRITZ!Box: Heimnetz → Netzwerk → Statische IPv4-Routen).
- [ ] **Ausgang ins Internet über den Hub** (Wunsch 23.10., Ausbaustufe des Gateways): Wer die Heim-VM als Standard-Gateway nutzt (einzelne Geräte per DHCP/Hand oder das ganze Heimnetz), geht über den Tunnel und den VPS ins Internet; die öffentliche IP ist dann die des VPS. Heim-VM: `AllowedIPs = 0.0.0.0/0` für den Hub-Peer mit Policy-Routing (nur weitergeleiteter LAN-Verkehr in den Tunnel, die Heim-VM selbst und der WireGuard-Handshake bleiben auf der normalen Leitung), Hub: NAT ins Internet für die Heim-Netze. Bewusst als Option mit Warnhinweis (Bandbreite und Traffic des VPS, Kill-Switch: fällt der Tunnel, kein Ausweichen auf die Heimleitung, wenn gewählt). Baut auf „Volltunnel“ (Phase 3) auf.

## Entscheidungen

Festgehalten, damit sie nicht wieder neu diskutiert werden.

- **Ein Paket für alle Rollen** (20.10.): VPS und Heim-VM bekommen dasselbe GoDeb, Unterschiede nur über Setup-Rolle und eingeschaltete Plugins. Alle Plugins werden mitgeliefert und sind abschaltbar.
- **Kein WireGuard-Server ohne Auswahl** (21.10.): Die Ersteinrichtung fragt die Rolle (Hub, Peer, ohne WireGuard) im Browser-Assistenten ab.
- **Backups macht die Plattform** (21.10.): Proxmox bzw. Hetzner sichern die ganze Maschine, GoDeb hat keine eigene Backup-Funktion.
- **Spielserver nur über das Plugin `steam`** (20.10.): ein neues Spiel ist eine `Game`-Beschreibung, kein eigenes Plugin.
- **Weboberfläche ohne root, root-Agent mit eigener 2FA** (22.10.) statt sudoers-Allowlist.
- **Ein Repository** (21.10.): Plugins bleiben in `GoDeb`, keine Aufteilung in einzelne Repos oder eine GitHub-Organisation. Die Apps haben eigene Repos: `godeb-tunnel-android` (Apache-2.0) und `godeb-tunnel-arch` (GPL-3.0), öffentlich seit 23.10.; Windows (`godeb-wireguard-windows`) bleibt privat, bis der Produktname im Installer und in der Oberfläche umgebaut ist. Umbenannt von „GoDeb WireGuard“ wegen der Markenrichtlinie von WireGuard.
- **Öffentlich unter GPL-3.0** (23.10.): neues Repo `ZeroDos1988/GoDeb` mit frischer Historie ab v0.2.1, nur unter dem Namen ZeroDos1988 (noreply-Adresse). Die Entwicklungsgeschichte bis dahin liegt im privaten Archiv `GoDeb-private`. `main` ist gegen Löschen und Force-Push geschützt, `v*`-Tags darf nur der Admin setzen; Lücken über Private Vulnerability Reporting.
- **Beitrittscode statt Kopplungsdienst** (25.10.): Der Code enthält die Client-Datei selbst. Ein kurzer Code, mit dem die Heim-VM ihre Datei beim Hub abholt, bräuchte einen öffentlich erreichbaren Endpunkt am Hub; das widerspricht „nur über den Tunnel“. Der lange Code wird kopiert, nicht abgetippt.
- **Releases per Git-Tag** (22.10., ergänzt 23.10.): Vorabversionen bis zum Test auf echten Servern. Neue Funktionen erhöhen die mittlere Stelle (`v0.2.0`: Logging und Überwachung), Fixes nach einem Servertest die letzte (`v0.2.x`); apt erkennt jede als Update. Die Installationsanleitung steht im Text des Releases.
