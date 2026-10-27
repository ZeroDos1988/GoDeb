# Aufgabenliste für Coding-Agenten (Stand 27.10.2026)

Diese Liste ist zum Abarbeiten durch einen Agenten (Cursor, Claude Code) gedacht. Hintergrund und Begründungen stehen in `ROADMAP.md`, Architektur und Sicherheitskonzept in `DOCUMENTATION.md`.

## Arbeitsweise (vor jeder Aufgabe lesen)

1. **`CLAUDE.md` lesen und befolgen.** Die Sicherheitsregeln dort sind nicht verhandelbar. Die wichtigsten:
   - Alles, was das System ändert, läuft im Agenten (`backend/agent`), nie in der Weboberfläche.
   - Externe Befehle nur über `core.Run*`, kein `sh -c`. Jede Eingabe gegen eine enge Regex oder Allowlist prüfen.
   - Neue `/api`-Routen sind automatisch geschützt. Zustandsänderungen nur mit POST/PUT.
   - UI-Texte nur über i18n (`de.json` und `en.json`). Fehlermeldungen und UI auf Deutsch, Code-Kommentare auf Englisch.
   - Jede Ansicht muss ab 360 px Breite bedienbar sein.
2. **Den Entwicklungsrechner nicht verändern.** Kein `apt`, `dpkg`, `sudo` und `systemctl`, nichts unter `/etc` oder `/usr`. Fehlt ein Werkzeug: anhalten und dem Nutzer einen fertigen Befehl nennen.
3. **Eine Aufgabe nach der anderen, in der Reihenfolge unten.** Aufgaben mit **(Frage)** erst nach Rückfrage beim Nutzer beginnen.
4. **Tests vor jedem Commit.**
   - Backend: `go test ./...` und `go vet ./...`.
   - Frontend, wenn es betroffen ist: `cd frontend && npm run build`.
   - Schlägt etwas fehl, nicht committen.
   - Zu neuer Logik gehören Tests (Vorbild: die vorhandenen `*_test.go`).
5. **Committen und pushen nach jedem fertigen Teilschritt.**
   - Gezielt stagen mit `git add <pfade>`, nie `git add -A` oder `git add .`.
   - Conventional Commits wie in der Historie, z. B. `feat(auth): …`.
   - Danach `git push origin main`. Kein `--force`, keine Tags, keine Releases ohne Auftrag.
6. **Danach in `ROADMAP.md` abhaken** (`[x]` mit Datum) und den Punkt hier als erledigt markieren. Was der Nutzer wissen oder testen muss, kommt in `DOCUMENTATION.md`.
7. **Neue Abhängigkeiten nur mit Begründung im Commit.** Danach `govulncheck ./...` bzw. `npm audit`.
8. **Nie einen Klarnamen oder eine private Mailadresse** in Commits oder Dateien. Die Commit-Identität ist bereits gesetzt.

---

## A. Für den Agenten

### 1. Passwort und 2FA bei kritischen Aktionen („sudo mit 2FA“) – erledigt 27.10.

Umgesetzt: `X-GoDeb-Password` bei `stepUpFresh`, Prüfung in der Weboberfläche vor dem Code, Header wird vor dem Agenten entfernt, Dialog mit Passwort+Code, Tests und Doku.

### 2. Update-Hinweise in der Überwachung – erledigt 27.10.

Umgesetzt in `plugins/apt/metrics.go`: Warnung bei offenen Sicherheitsupdates nur wenn AutoCheck/AutoSecurity aus ist, Warnung bei fehlgeschlagenem automatischem Lauf (`status.json`), Neustart weiter über `rebootCheck`. Läuft in Ampel und Alarme.

### 3. Befunde von `godeb doctor` auf der Startseite – erledigt 27.10.

`GET /api/doctor` (Web) bzw. `GET /internal/doctor` (Agent, nur lesend); Kachel auf der Übersicht mit FAIL/WARN.

### 4. Mail-Ausbau (Plugin `mail`, Dovecot **2.4**-Syntax)

Jeden Unterpunkt einzeln committen:
- **Abwesenheitsnotiz** per Sieve (Pigeonhole) pro Postfach, in der UI ein- und ausschaltbar.
- **Autoconfig (Thunderbird) und Autodiscover (Outlook)** für die Maildomains. Braucht einen Webserver, also nach Aufgabe 6 oder als kleiner eigener Endpunkt; vorher fragen.
- **MTA-STS und TLS-RPT**: Richtlinie und DNS-Einträge, über `domains.RegisterNeeds` melden.
- **Quota-Warnungen** (Dovecot-Quota-Warning) und Belegung pro Postfach in der UI.
- **Spam lernen**: Verschieben in den Junk-Ordner bzw. aus ihm heraus trainiert Rspamd (imapsieve).
- **Mail-Queue**: Postfix-Queue anzeigen, einzelne Mails löschen oder erneut senden. Die Queue-IDs gegen eine Regex prüfen.
- Danach mit `scripts/check-mail-config.sh` gegen die echten Debian-13-Werkzeuge prüfen.

### 5. Mobil-Feinschliff (ROADMAP Phase 6)

- Kompakte Kopfzeile mit Menü-Button statt der Seitenleiste auf dem Handy.
- Touch-Ziele mindestens 44 px. Unter `sm` Tabellen als Karten (WireGuard-Peers, UFW-Regeln).
- Playwright-Smoke-Test mit iPhone- und Pixel-Viewport in der CI (`.github/workflows`): prüft `scrollWidth == innerWidth` auf allen Seiten, die API wird per `page.route` mit Beispieldaten bedient.

### 6. Webseiten und Reverse-Proxy (neues Plugin, nginx) **(Frage)**

nginx-vhosts für eigene Webseiten und Reverse-Proxy auf interne Ziele, auch hinter dem Tunnel. Zertifikate über das vorhandene Certbot-Plugin, DNS über `domains.RegisterNeeds`. Vor dem Start mit dem Nutzer klären: PHP ja oder nein, Verzeichnisstruktur, ob Port 80/443 dauerhaft offen sein darf.

### 7. Cron-Aufgaben (neues Plugin)

Geplante Aufgaben (systemd-Timer und `/etc/cron.d`) anzeigen. Neue Aufgaben nur als systemd-Timer mit einem Befehl aus einer Allowlist bzw. einem streng geprüften Pfad, kein freier Shell-Befehl.

### 8. Datenbanken (neues Plugin) **(Frage)**

MariaDB und/oder PostgreSQL installieren, Datenbanken und Benutzer anlegen, einfacher Tabellenbetrachter. Zugangsdaten versiegelt (`vault:"secret"`, Agent-DB, `db.RegisterAgentSecret`). Datenbankports nie öffentlich. Vorher fragen: welche Datenbank zuerst, ob ein Schreibzugriff im Betrachter gewünscht ist.

### 9. Kleinere offene Punkte

- WireGuard: Option „Schlüssel auf dem Gerät erzeugen“ (Public Key einfügen), Client-Dateien nach dem Download optional löschen.
- APK-Download optional mit Token.
- [x] `godeb doctor` und Ampel: SSH nur per Schlüssel und unattended-upgrades (nur melden) – 27.10.
- Paket auch für `arm64` (Hetzner CAX): `scripts/build-deb.sh`, CI und APT-Repo-Workflow.
- Steam/Enshrouded: Formular für `gameSettings` beim Preset „Custom“, zeitgesteuerte Updates und Neustarts mit Warnfenster.
- Volltunnel als bewusste Option im Setup (IP-Forwarding und NAT, eigene nftables-Tabelle wie `portforward`) **(Frage)**.

---

## B. Für den Nutzer (braucht die echten Server, nicht für den Agenten)

1. **VPS auf v0.2.7 bringen:** vorher einen Snapshot ziehen, dann APT → „Alle aktualisieren“. Prüfen: `godeb version`, Dienste-Seite, Protokoll, Plugins abschalten, automatische Updates.
2. **Mailserver auf dem VPS:**
   - Vorher: A-Eintrag `mail.<domain>`, Reverse-DNS, Freigabe für ausgehend 25/465 bei Hetzner beantragen.
   - Dann: Domain und Postfach anlegen, mit Thunderbird und Handy testen, mail-tester.com.
3. **Domains-Plugin** mit echter Zone und echtem Hetzner-Token prüfen.
4. **Release v0.2.8** (Sicherheitsfix für Agent-Secrets, Domains, `godeb doctor`, danach Aufgabe A1):
   - Tag setzen: `git tag v0.2.8 && git push origin v0.2.8`.
   - Danach den Release-Text im Stil von v0.2.7 schreiben und als Vorabversion markieren.
   - Der Agent macht das nur auf ausdrücklichen Auftrag.
5. **Heim-VM** per Beitrittscode anbinden, Enshrouded installieren, Portweiterleitung am VPS einrichten, im Spiel testen.
6. **Tunnel-App am Handy:** Auto-Disconnect im Heim-WLAN und Wiederverbinden im Mobilnetz.

## C. Andere Repos (nicht in diesem Repo bearbeiten)

- `godeb-tunnel-arch` und `godeb-wireguard-windows`: Vertrauensprüfung auf das Protokoll `godeb-trust-v1` umstellen, wie in der Android-App. Die alte Header-Prüfung funktioniert nicht mehr.
- Windows-Client umbenennen in „GoDeb Tunnel“ und veröffentlichen (ROADMAP Phase 5).
