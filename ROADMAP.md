# Fahrplan

Stand: 28.10.2026. Reihenfolge = Priorität. `[x]` erledigt, `[ ]` offen. Neue Ideen kommen hierher (passende Phase oder Phase 7), getroffene Entscheidungen unten unter „Entscheidungen“.

## Aktueller Stand (29.10., nach v0.2.14)

Für einen neuen Chat: zuerst `/diagnose` (Skill in `.claude/skills/diagnose`, Skript `scripts/diagnose.sh`), dann hier weiterlesen.

- **Veröffentlicht** (alle öffentlich, nur unter ZeroDos1988):
  - `ZeroDos1988/GoDeb`: GPL-3.0, Release **v0.2.14** (29.10., Vorabversion: aufgeräumte Seiten mit „Technische Details“, Hostname in der Kopfzeile, Fixes für Navigation und Timer-Liste) im APT-Repo `https://zerodos1988.github.io/GoDeb`. v0.2.13 (Logo, Anmeldung in drei Schritten ohne Verraten von Benutzernamen, fail2ban 5 Versuche/10 Minuten steigend). v0.2.12 (Fehlergrund und Hintergrundjobs im Journal, keine Fehlalarme bei Zustandsabfragen, go-jose 4.1.6 und weitere Abhängigkeiten). v0.2.11 (Sicherheitsupdate: Webseiten-Proxy-Pfad, `/var/vmail` per `os.Root`, PostgreSQL-2FA; dazu Datenbank-Prüfung, Hub-Überwachung von der Heim-VM, stabile und Test-Suite; amd64 und arm64, Suiten `trixie` und `trixie-testing`). v0.2.10 brachte arm64 ins APT-Repo, v0.2.9 brachte die Sicherheitsfixes für Webseiten, PostgreSQL und Cron aus v0.2.8; v0.2.8 Mail-Ausbau, nginx/Webseiten, PostgreSQL, Cron, Mobil-Feinschliff, Volltunnel. Jeder Tag `vX.Y.Z` baut Release (immer als Vorabversion, erst nach dem Update-Test) und APT-Repo automatisch; den Release-Text danach von Hand im Stil der vorigen Releases ersetzen. Ab 1.0 das Vorab-Häkchen entfernen, damit die Version in `trixie` landet.
  - `ZeroDos1988/godeb-tunnel-android` (Release 1.1.0), `ZeroDos1988/godeb-tunnel-arch` (1.0.1). Privat: `godeb-wireguard-windows`.
- **Auf den Servern:** Hetzner-VPS zuletzt mit älterer 0.2.x; Heim-VM noch nicht neu eingerichtet. **v0.2.14 auf dem VPS noch nicht eingespielt.**
- **Nächste Schritte:** der Fahrplan „Produktionsreife“ direkt unten. Alles, was ohne die Server ging, ist erledigt (28.10.); offen sind die Punkte beim Nutzer: Durchlauf auf beiden Servern, Hetzner-Grundschutz, Wiederherstellung üben, danach 1.0.

## Produktionsreife – Weg zu 1.0 (Prüfung 28.10.)

Funktional ist alles da, was für den Betrieb gewünscht ist; ab jetzt keine neuen Funktionen vor 1.0, nur Tests, Fixes und Betriebssicherheit. Stand der Prüfung: `go test` (30 Pakete), `go vet`, `gofmt`, `govulncheck`, `npm audit`, i18n, API-Doku und Sicherheitsregeln aus `scripts/diagnose.sh` alle grün; systemd-Härtung 1.3 (Weboberfläche) bzw. 7.4 (Agent); HTTP-Server mit `ReadHeaderTimeout` und sauberem Shutdown; SQLite mit `busy_timeout`; Login über fail2ban und 2FA geschützt; Notfallbefehle per SSH vorhanden (`godeb 2fa reset`, `user passwd`, `sessions revoke`). Die größte Lücke: **nichts davon ist auf den echten Servern gelaufen.**

### P0 – vor dem Produktivbetrieb (Pflicht)

- [ ] **Echter Durchlauf auf beiden Servern** (sammelt die offenen Tests aus Phase 2, 3, 3b und 4b): Update von der alten 0.2.x auf dem VPS (Umzug nach `/var/lib/godeb`), Assistent, Agent, Plugins ab-/anschalten, automatische Sicherheitsupdates und ein Selbst-Update von GoDeb über „Alle aktualisieren“; Heim-VM per Beitrittscode, Enshrouded (Wine ohne X-Server, Ports), Portweiterleitung, Handy und Spiel-Client von außen; **beide Seiten neu starten** und prüfen, dass Tunnel, nftables-Tabellen, ufw-Regeln und Dienste von selbst wiederkommen. Jeden Fehler als Fix-Release `0.2.x`.
- [ ] **Hetzner-Grundschutz** (Nutzer, Anleitung in `DOCUMENTATION.md` „Produktivbetrieb (Checkliste)“): Backups für den VPS einschalten (täglich, 7 Stände) und vor jedem Update ein Snapshot; Hetzner Cloud Firewall als zweite Schicht vor ufw; SSH nur mit Schlüssel. Für Mail vorab: Reverse-DNS, Freigabe von Port 25/465, Ergebnis bei mail-tester.com mindestens 9/10.
- [ ] **Wiederherstellung einmal üben, nicht nur aufschreiben** (Nutzer): Handy weg (`godeb 2fa reset`), VPS aus einem Snapshot zurückholen, Heim-VM neu aufsetzen und wieder beitreten. Das Runbook steht seit 28.10. in `DOCUMENTATION.md` unter „Notfall“; beim Üben Lücken dort nachtragen.
- [x] **Überwachung von außen** (28.10.): Die Heim-VM fragt jede Minute `/api/health` des Hubs über den Tunnel und meldet nach 3 Minuten „Hub nicht erreichbar“ (Alarme per ntfy). Offen beim Nutzer: ntfy auf beiden Servern einschalten, öffentliche Dienste (Mail, Webseiten) mit einem externen Uptime-Dienst prüfen.
- [x] **Sicherheitsdurchsicht der jüngsten Plugins** (28.10.): Webseiten: Konfig-Injektion über den Pfad der Proxy-URL geschlossen, Punktdateien (.git, .env) gesperrt. Mail: `/var/vmail` nur noch über `os.Root` (vorher konnte ein Symlink des Benutzers vmail den root-Agenten außerhalb löschen bzw. chown ausführen lassen), Abwesenheitstext mehrzeilig korrekt (Sieve `text:`), mit `sievec` geprüft. PostgreSQL: Tabellenansicht nur mit 2FA-Fenster, Löschen mit Passwort und frischem Code, Ausgabe als CSV. Cron, Mail-Queue, Portweiterleitung und Certbot ohne Befund. Jeder Fund hat einen Test.
- [ ] **Feature-Freeze und 1.0:** Wenn P0 erledigt ist und das System zwei Wochen ohne Eingriff gelaufen ist, `v1.0.0` als erste **nicht** vorläufige Version. Danach auf `1.0.x` nur Fixes.

### P1 – in den ersten Wochen Betrieb

- [x] **Stabile und Test-Suite im APT-Repo** (28.10.): `trixie` nur Releases, `trixie-testing` auch Vorabversionen (`godeb-testing.sources`). Tags werden immer als Vorabversion veröffentlicht; erst das Entfernen des Vorab-Häkchens nimmt eine Version in `trixie` auf. Vor 1.0 trägt `trixie` weiter die Vorabversionen.
- [x] **Update-Test in der CI** (28.10., `scripts/upgrade-test.sh`): Debian-13-Container mit systemd, neueste Version aus dem Repo, Update auf das gebaute Paket, Dienste, Version und Datenbanken (`godeb doctor`). Ein Release wartet darauf. Offen: arm64 per QEMU.
- [x] **Abhängigkeiten aktuell halten** (28.10.): Dependabot (Go wöchentlich, npm wöchentlich, Actions monatlich, gruppiert) und `security.yml` montags mit `govulncheck`, `npm audit` und Ablaufprüfung des APT-Schlüssels. Ein Fund heißt Fix-Release.
- [x] **Datenbanken prüfen** (28.10.): `PRAGMA quick_check` in `godeb doctor` und auf der Startseite (Agent-Datenbank und Messwerte im Agenten, Web-Datenbank in der Weboberfläche). Offen: Platzbedarf von `metrics.db` nach ein paar Wochen ansehen.
- [ ] **Grenzwerte der Überwachung** nach ein paar Wochen echter Daten einstellen (Grundlinie, Platte, Alarme), damit Alarme selten und dann ernst sind.
- [ ] **Fristen:** Der Ablauf des APT-Signierschlüssels (2028-10-22) wird seit 28.10. wöchentlich geprüft (90 Tage Vorlauf). Offen beim Nutzer: Android-Keystore an zweiter Stelle sichern; Let's-Encrypt-Erneuerung einmal per Ampel bestätigt sehen.
- [ ] Tunnel-Clients: Windows/Arch auf `godeb-trust-v1` (heute kein Auto-Disconnect dort), Handy-Test des Auto-Disconnect (Phase 5). Nur nötig, wenn diese Clients produktiv genutzt werden.

### P2 – danach (nice to have)

Alles aus Phase 4 (Savegame-Backup, weitere Spiele), Phase 7 (Webmail, eigener DNS), Phase 8 (Heim-VM als Gateway, gekoppelte Verwaltung, Netzwerk-Scanner), Push direkt in die App, IPv6-Weiterleitung, echte Spieler-IPs. Neue Wünsche kommen weiter in die passende Phase.

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
- [x] **Passwort und 2FA bei kritischen Aktionen („sudo mit 2FA“)** (Wunsch und Umsetzung 27.10.): Für `stepUpFresh` verlangt die Weboberfläche den Header `X-GoDeb-Password` (Prüfung gegen die Web-DB, bevor der Code geprüft wird) und entfernt ihn vor der Weiterleitung an den Agenten; der Agent prüft weiter nur den TOTP. Gewöhnliche Änderungen bleiben beim 5-Minuten-Fenster, Lesen bleibt frei.
- [x] **Logging** (23.10.): `slog` ins Journal mit Prioritäten, `GODEB_LOG_LEVEL`. Der Agent protokolliert jede Aktion mit Benutzer, Status und 2FA-Ergebnis, fehlgeschlagene Befehle kommen mit Exit-Code. Ansicht „Protokoll“ (Plugin `logs`).
- [ ] **Agent auf den echten Servern testen**: Update von der Version davor (Umzug nach `/var/lib/godeb`), Assistent, Trusted Endpoint, Steam-Installation.
- [x] **Client-Schlüssel** (27.10.): Option „Schlüssel auf dem Gerät erzeugen“ (Public Key einfügen); Client-Dateien nach dem Download optional löschen. Mailversand nur mit Warnhinweis (weiterhin).
- [x] ~~**Master-Key** getrennt von der Datenbank~~ (entschieden 21.10.): entfällt. Gesichert wird die ganze VM (Proxmox bzw. Hetzner), ein VM-Backup enthält Schlüssel und Datenbank ohnehin zusammen; ein systemd-Credential läge auf derselben Platte.
- [x] **Volltunnel** (28.10.): bewusste Option im Setup und unter WireGuard (`--full-tunnel` / Checkbox). IP-Forwarding und MASQUERADE in eigener nftables-Tabelle `godeb_volltunnel` (wie Portweiterleitung, nicht `before.rules`), dazu `ufw route allow` wg0→öffentlich; Boot-Unit `godeb-volltunnel.service`. Standard aus; Geräte-Clients im Setup bekommen bei Aktivierung `AllowedIPs 0.0.0.0/0`, Standorte nie.
- [x] **Unauthentifizierter APK-Download**: nur über den Tunnel erreichbar (Bind-Adresse); optional mit Token (28.10.). (`/api/trusted` ist seit 20.10. entfernt.)
- [x] ~~**Backups**~~ (entschieden 21.10.): nicht Aufgabe von GoDeb. Proxmox (Heim-VM) und Hetzner (VPS) sichern die ganze Maschine; vor größeren Änderungen einen Snapshot ziehen.
- [ ] VPS-Spezifika: Setup installiert SSH-nur-Schlüssel und unattended-upgrades noch nicht (steht bisher nur in der Anleitung). Doctor und Ampel melden den Zustand seit 27.10. nur lesend.
- [x] **Updates aus der Oberfläche, automatische Prüfung und Sicherheitsupdates** (Wunsch und Umsetzung 25.10.): Der Agent prüft alle 10 Minuten (einstellbar) und installiert Pakete aus `trixie-security` selbst; „Alle aktualisieren“ mit frischem 2FA-Code für den Rest, GoDeb inklusive. Jeder Lauf ist eine eigene Unit `godeb-apt.service` (`systemd-run`), damit das Selbst-Update den Neustart des Agenten überlebt.
- [x] **Update-Hinweise in der Überwachung** (Umsetzung 27.10.): Sicherheitsupdates als Warnung nur bei abgeschalteter Automatik (`plugins/apt/metrics.go`), fehlgeschlagene automatische `godeb-apt`-Läufe als Warnung, „Neustart nötig“ weiter über `rebootCheck`; beides läuft in die Ampel und optional in die Alarme.
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

- [x] Formular für `gameSettings` beim Preset „Custom“ (wichtigste Faktoren mit Schiebereglern) – 28.10.
- [ ] Savegame-Backup vor jedem Update und auf Knopfdruck, Wiederherstellen aus der UI.
- [x] Zeitgesteuerte Updates/Neustarts mit Warnfenster – 28.10. (Warnfenster in der UI, Zeitplan über das Plugin „Geplante Aufgaben“).
- [x] Spielerzahl/Status per Steam-Query anzeigen (Überwachung M3, 23.10.).
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

- [x] Kompakte Kopfzeile mit Menü-Button statt der vollen Seitenleiste auf dem Handy – 28.10.
- [x] **Benutzermenü wie bei GitHub** (Wunsch und Umsetzung 25.10.): Avatar oben rechts mit Sprache, Darstellung, Sitzungen, Einrichtungsübersicht, Abmelden und Version. Die Sprache folgt standardmäßig dem Browser („Auto“); Login und Ersteinrichtung haben eine eigene Sprachwahl.
- [x] Touch-Ziele mindestens 44 px, Tabellen als Karten unter `sm` (WireGuard-Peers, UFW-Regeln) – 28.10.
- [x] **Navigation wie bei GitHub** (Wunsch und Umsetzung 29.10.): keine Seitenleiste mehr. Kopfzeile mit Logo, Hostname, Zustand und Benutzermenü, darunter Reiter Übersicht · System · Netzwerk · Anwendungen · Einstellungen (Plugin-Manager, Einrichtungsübersicht); innerhalb eines Reiters links die Plugins (auf dem Handy als Chips). Gruppe über `group` im Manifest. Übersicht: Zustand und Diagnose in einer Karte, still bei „Alles in Ordnung“.
- [x] Playwright-Smoke-Test mit iPhone- und Pixel-Viewport in CI (prüft `scrollWidth == innerWidth`) – 28.10.

## Phase 7 – Weitere Plugins (Wunschliste, 20.10.)

- [x] **Webseiten und Reverse-Proxy** (nginx, Plugin `websites`, 28.10.): eigene Webseiten hosten und interne Dienste veröffentlichen, mit Let's-Encrypt-Zertifikaten (vorhandenes Certbot-Plugin). Auf dem VPS als Eingang, Ziele auch hinter dem Tunnel (z. B. die Heim-VM).
- [x] **Vollwertiger Mailserver nach dem Vorbild von HestiaCP** (Wunsch und erste Fassung 27.10., Plugin `mail`, auf dem VPS): Domains, Postfächer, Aliase, Weiterleitungen, Catch-all, Sperren, DKIM, DNS-Prüfung, Spamfilter, Let's Encrypt, optional ClamAV. Entschieden: Postfächer erst auf dem VPS, Rspamd statt SpamAssassin, ClamAV abschaltbar. Erledigt 28.10.: Abwesenheitsnotiz (Sieve); MTA-STS/TLS-RPT (DNS + Richtlinie); Quota-Warnungen; Spam/Ham-Lernen (imapsieve); Mail-Queue. Noch offen: Webmail (Roundcube), Autoconfig/Autodiscover, HTTPS für MTA-STS/Autoconfig (Webserver), Statistik, Befunde in der Überwachung. Danach: VPS nur als Relay (Ein- und Ausgang), Postfächer und Virenscanner auf der Heim-VM. Ursprünglicher Wunsch: Maildomains, Postfächer mit Passwort und Quota, Aliase und Weiterleitungen, Catch-all, DKIM/SPF/DMARC mit fertigen DNS-Einträgen, TLS über Let's Encrypt, IMAP/SMTP-Submission (Postfix + Dovecot), Spamfilter, optional Webmail, Autoconfig für Mailprogramme. Baut auf dem Mail-Gateway auf (Eingang und Versand über den VPS, weil Heimanschlüsse für Port 25 und PTR nicht taugen). Offene Entscheidungen: Standort (VPS oder Heim-VM), Spamfilter (Rspamd oder SpamAssassin), Virenscanner (ClamAV braucht etwa 1,5 GB RAM), Webmail (Roundcube braucht PHP und einen Webserver).
- [x] **Dienste-Seite wie bei HestiaCP** (Wunsch und Umsetzung 27.10.): statt der Liste aller systemd-Units eine kuratierte Übersicht der Serverdienste (SSH, Firewall, Fail2ban, WireGuard, Postfix, Dovecot, Spielserver, GoDeb selbst) mit Beschreibung, Status, Laufzeit, CPU und RAM sowie Start/Stopp/Neustart; die vollständige Unit-Liste bleibt als zweiter Reiter. Oben die Eckdaten des Servers (Hostname, OS, Kernel, Laufzeit, Last).
- [x] **Domains und DNS** (Wunsch 27.10., erste Fassung 27.10., Plugin `domains`): Domains einmal eintragen; Mail meldet MX, SPF, DKIM, DMARC, A und PTR. Betriebsarten **Manuell** (zeigen und prüfen) und **Hetzner Cloud-API** (Token versiegelt, nur RRSets mit Label `godeb=1`, PTR über `change_dns_ptr`). Noch offen: Anbindung weiterer Plugins (Webseiten), eigener primärer DNS auf dem VPS (Knot/PowerDNS mit Hetzner als Secondary). Erledigt 27.10.: Hinweis in der Domain-Ansicht, wenn die Zone im Hetzner-Projekt fehlt oder die API nicht lesbar ist.
- [x] **`godeb doctor`** (Idee und Umsetzung 27.10., `backend/doctor`): Server-Diagnose als ein Befehl (Dienste, offene Ports, Firewall, Tunnel-Handshakes, Zertifikate, DNS der Maildomains, Fehler im Journal, Updates, Speicher), Ausgabe ohne Geheimnisse zum Weitergeben. Ersetzt den Befehlsblock im Skill `/diagnose`. Dieselben Befunde auf der Startseite über `GET /api/doctor` (Agent: `/internal/doctor`, nur lesend).
- [x] **Cron-Aufgaben** (28.10., Plugin `cron`): systemd-Timer und `/etc/cron.d` anzeigen; neue Aufgaben nur als GoDeb-Timer mit Allowlist-Befehl oder Skript unter `/usr/local/sbin` bzw. `/etc/godeb/scripts`.
- [x] **Datenbanken** (28.10., Plugin `postgres`, entschieden: nur PostgreSQL): Datenbankserver installieren, Datenbanken und Benutzer anlegen, dazu ein eingebauter Admin-Client zum Ansehen von Tabellen und Daten. Zugangsdaten mit `security.Seal`, Datenbankports nie öffentlich.

## Phase 8 – Hub und Standorte enger koppeln (Ideen 23.10.)

- [x] **Beitritt per Datei** (23.10.): Standort anlegen lädt die `.conf` herunter, der Assistent der Heim-VM nimmt sie per Upload.
- [x] **Weitere Clients und Standorte im Assistenten, Beitritt per Code** (Wunsch und Umsetzung 25.10.): Auf dem Hub legt „+“ beliebig viele Handys/Laptops (QR und Datei) und GoDeb-Server zu Hause (Beitrittscode) an; `godeb setup` fragt dasselbe ab. Der Beitrittscode ist die Client-Datei als eine Zeile (`godeb-join:` + base64url) und wird in „Mit Hub verbinden“ oder `godeb setup --join -` eingefügt. Der erste Standort bekommt `10.8.0.2`, Geräte lassen die Adresse dafür frei.
- [x] **Druckansichten** (25.10.): Übersicht und Backup-Codes drucken als eigener Bogen (`PrintSheet`), statt der Glas-Oberfläche. Vorher kamen leere Seiten heraus, und beim 2FA-Druck stand der TOTP-Schlüssel mit auf dem Papier.
- [ ] **Gekoppelte Verwaltung**: Hub und Standort vertrauen einander (gegenseitig signierte Anfragen über den Tunnel, kein offener Port). Auf der Heim-VM eine Weiterleitung anlegen, und der Hub legt DNAT und ufw-Freigabe selbst an; Spielserver-Installation auf der Heim-VM schlägt die Weiterleitung am Hub automatisch vor.
- [ ] **Netzwerk-Scanner**: Hosts im Heimnetz bzw. Tunnel finden (ARP/Ping), offene Ports prüfen (nur TCP-Connect und bekannte UDP-Abfragen wie Steam A2S, kein aggressives Scannen), bekannte Dienste erkennen (z. B. „Enshrouded auf 192.168.1.40, 15636–15637/udp“) und per Klick „über den Hub erreichbar machen“ (Weiterleitung Hub → Standort → Zielhost). Nur auf eigenen Netzen, mit Hinweis.
- [ ] **Heim-VM als Gateway ins Tunnelnetz** (Wunsch 23.10.): Geräte im Heimnetz erreichen den Hub und alle Tunnel-Adressen (z. B. `http://10.8.0.1:8080`) ohne eigenen WireGuard-Client. Im Router wird eine statische Route `10.8.0.0/24 → <LAN-IP der Heim-VM>` eingetragen; GoDeb auf der Heim-VM schaltet IP-Forwarding ein, erlaubt Weiterleitung LAN → `wg0` (eigene nftables-Tabelle wie bei `portforward`, mit SNAT auf die Tunnel-Adresse der Heim-VM, damit der Hub ohne Rückroute antwortet) und erweitert das Tunnelnetz in den AllowedIPs bei Bedarf. Die Übersicht nennt die genaue Route für den Router (FRITZ!Box: Heimnetz → Netzwerk → Statische IPv4-Routen).
- [ ] **Ausgang ins Internet über den Hub** (Wunsch 23.10., Ausbaustufe des Gateways): Wer die Heim-VM als Standard-Gateway nutzt (einzelne Geräte per DHCP/Hand oder das ganze Heimnetz), geht über den Tunnel und den VPS ins Internet; die öffentliche IP ist dann die des VPS. Heim-VM: `AllowedIPs = 0.0.0.0/0` für den Hub-Peer mit Policy-Routing (nur weitergeleiteter LAN-Verkehr in den Tunnel, die Heim-VM selbst und der WireGuard-Handshake bleiben auf der normalen Leitung), Hub: NAT ins Internet für die Heim-Netze. Bewusst als Option mit Warnhinweis (Bandbreite und Traffic des VPS, Kill-Switch: fällt der Tunnel, kein Ausweichen auf die Heimleitung, wenn gewählt). Baut auf „Volltunnel“ (Phase 3) auf.

## Ganz hinten (niedrige Priorität)

- [x] **arm64 im APT-Repo** (Hetzner CAX, 28.10.): `build-apt-repo.sh` baut `binary-amd64` und `binary-arm64`, `godeb.sources` nennt beide. Plugin Steam ist auf arm64 gesperrt (`architectures` in `plugin.json`), weil es SteamCMD nur für x86 gibt.

## Entscheidungen

Festgehalten, damit sie nicht wieder neu diskutiert werden.

- **Logo** (29.10.): mit Gemini entworfen (Schild mit „GD“, Schriftzug „GoDeb“ in Cyan/Rot), Dateien in `docs/brand/`.
- **Anmeldung in drei Schritten, ohne Verraten von Benutzernamen** (29.10.): Schritt 1 antwortet für jeden Namen gleich; ein unbekannter Name zählt trotzdem sofort für fail2ban (5 Versuche in 10 Minuten, 10 Minuten Sperre, steigend). Kein Cookie-Banner, weil nur technisch nötige Cookies.
- **Ein Paket für alle Rollen** (20.10.): VPS und Heim-VM bekommen dasselbe GoDeb, Unterschiede nur über Setup-Rolle und eingeschaltete Plugins. Alle Plugins werden mitgeliefert und sind abschaltbar.
- **Kein WireGuard-Server ohne Auswahl** (21.10.): Die Ersteinrichtung fragt die Rolle (Hub, Peer, ohne WireGuard) im Browser-Assistenten ab.
- **Backups macht die Plattform** (21.10.): Proxmox bzw. Hetzner sichern die ganze Maschine, GoDeb hat keine eigene Backup-Funktion.
- **Spielserver nur über das Plugin `steam`** (20.10.): ein neues Spiel ist eine `Game`-Beschreibung, kein eigenes Plugin.
- **Weboberfläche ohne root, root-Agent mit eigener 2FA** (22.10.) statt sudoers-Allowlist.
- **Ein Repository** (21.10.): Plugins bleiben in `GoDeb`, keine Aufteilung in einzelne Repos oder eine GitHub-Organisation. Die Apps haben eigene Repos: `godeb-tunnel-android` (Apache-2.0) und `godeb-tunnel-arch` (GPL-3.0), öffentlich seit 23.10.; Windows (`godeb-wireguard-windows`) bleibt privat, bis der Produktname im Installer und in der Oberfläche umgebaut ist. Umbenannt von „GoDeb WireGuard“ wegen der Markenrichtlinie von WireGuard.
- **Öffentlich unter GPL-3.0** (23.10.): neues Repo `ZeroDos1988/GoDeb` mit frischer Historie ab v0.2.1, nur unter dem Namen ZeroDos1988 (noreply-Adresse). Die Entwicklungsgeschichte bis dahin liegt im privaten Archiv `GoDeb-private`. `main` ist gegen Löschen und Force-Push geschützt, `v*`-Tags darf nur der Admin setzen; Lücken über Private Vulnerability Reporting.
- **Mailserver zuerst auf dem VPS** (27.10.): Postfächer liegen vorerst auf dem VPS; Ziel bleibt VPS nur als Relay, Postfächer und Virenscanner auf der Heim-VM. Rspamd statt SpamAssassin (leichter, DKIM eingebaut), ClamAV abschaltbar (RAM). Bedienung nach dem Vorbild von HestiaCP, aber moderner. Hetzner sperrt bei Cloud-Servern ausgehend 25 und 465 standardmäßig, unabhängig von der eigenen Domain; Freigabe per Antrag, meist erst nach der ersten bezahlten Rechnung. Ohne Freigabe geht Empfang, Versand nur über einen Relay-Dienst auf Port 587.
- **Beitrittscode statt Kopplungsdienst** (25.10.): Der Code enthält die Client-Datei selbst. Ein kurzer Code, mit dem die Heim-VM ihre Datei beim Hub abholt, bräuchte einen öffentlich erreichbaren Endpunkt am Hub; das widerspricht „nur über den Tunnel“. Der lange Code wird kopiert, nicht abgetippt.
- **Zielgruppe und Umfang** (27.10.): Ein Admin, der seine eigenen Server privat betreibt. Vorbild ist HestiaCP (Bedienung) bzw. Plesk (Funktionsumfang), aber mit den aktuellen Paketen aus Debian 13 statt eigener Paketstände und ohne Mandanten: keine Rechte pro Domain, keine Reseller- oder Kundenkonten. Mehrere Admin-Konten gibt es, alle mit denselben Rechten.
- **arm64 veröffentlichen** (28.10.): zuerst nachrangig, am selben Tag doch umgesetzt (Nutzerwunsch). Spielserver bleiben amd64-only.
- **Releases per Git-Tag** (22.10., ergänzt 23.10.): Vorabversionen bis zum Test auf echten Servern. Neue Funktionen erhöhen die mittlere Stelle (`v0.2.0`: Logging und Überwachung), Fixes nach einem Servertest die letzte (`v0.2.x`); apt erkennt jede als Update. Die Installationsanleitung steht im Text des Releases.
