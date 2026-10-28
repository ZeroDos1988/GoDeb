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
- [x] **Abwesenheitsnotiz** per Sieve (Pigeonhole) pro Postfach, in der UI ein- und ausschaltbar – 28.10.
- [x] **Autoconfig (Thunderbird) und Autodiscover (Outlook)** für die Maildomains – 28.10. (nginx, nach Webseiten-Plugin).
- [x] **MTA-STS und TLS-RPT**: Richtlinie und DNS-Einträge, über `domains.RegisterNeeds` melden – 28.10. (HTTPS-Auslieferung der Richtlinie mit Webserver-Plugin).
- [x] **Quota-Warnungen** (Dovecot-Quota-Warning) und Belegung pro Postfach in der UI – 28.10.
- [x] **Spam lernen**: Verschieben in den Junk-Ordner bzw. aus ihm heraus trainiert Rspamd (imapsieve) – 28.10.
- [x] **Mail-Queue**: Postfix-Queue anzeigen, einzelne Mails löschen oder erneut senden. Die Queue-IDs gegen eine Regex prüfen – 28.10.
- Danach mit `scripts/check-mail-config.sh` gegen die echten Debian-13-Werkzeuge prüfen.

### 5. Mobil-Feinschliff (ROADMAP Phase 6) – erledigt 28.10.

- [x] Kompakte Kopfzeile mit Menü-Button statt der Seitenleiste auf dem Handy.
- [x] Touch-Ziele mindestens 44 px. Unter `sm` Tabellen als Karten (WireGuard-Peers, UFW-Regeln).
- [x] Playwright-Smoke-Test mit iPhone- und Pixel-Viewport in der CI (`.github/workflows`): prüft `scrollWidth == innerWidth` auf allen Seiten, die API wird per `page.route` mit Beispieldaten bedient.

### 6. Webseiten und Reverse-Proxy (neues Plugin, nginx) – erledigt 28.10.

nginx-vhosts für eigene Webseiten und Reverse-Proxy auf interne Ziele, auch hinter dem Tunnel. Zertifikate über Certbot, DNS über `domains.RegisterNeeds`. Entschieden: PHP ja, Docroot `/var/www/godeb/<name>` mit Symlink `~/www/<name>`, Ports 80/443 bei Bedarf.

### 7. Cron-Aufgaben (neues Plugin) – erledigt 28.10.

Geplante Aufgaben (systemd-Timer und `/etc/cron.d`) anzeigen. Neue Aufgaben nur als systemd-Timer mit einem Befehl aus einer Allowlist bzw. einem streng geprüften Pfad, kein freier Shell-Befehl.

### 8. Datenbanken (neues Plugin) – erledigt 28.10.

PostgreSQL (Entscheidung Nutzer). Datenbanken und Benutzer anlegen, Tabellenbetrachter nur lesend. Zugangsdaten versiegelt (`vault:"secret"`, Agent-DB). Ports nie öffentlich.

### 9. Kleinere offene Punkte

- [x] WireGuard: Option „Schlüssel auf dem Gerät erzeugen“ (Public Key einfügen), Client-Dateien nach dem Download optional löschen – 27.10.
- [x] APK-Download optional mit Token – 28.10.
- [x] `godeb doctor` und Ampel: SSH nur per Schlüssel und unattended-upgrades (nur melden) – 27.10.
- [x] Steam/Enshrouded: Formular für `gameSettings` beim Preset „Custom“, Warnfenster vor Update/Neustart – 28.10. (zeitgesteuerte Jobs über Plugin „Geplante Aufgaben“ / Skript).
- [x] Volltunnel als bewusste Option im Setup (IP-Forwarding und NAT, eigene nftables-Tabelle wie `portforward`) – 28.10.

### 10. Ganz hinten (niedrige Priorität, erst nach allem anderen)

- [x] **arm64 im APT-Repo** – 28.10. (`build-apt-repo.sh` mit beiden Architekturen, Steam auf arm64 gesperrt).

---

## B. Für den Nutzer (braucht die echten Server, nicht für den Agenten)

1. **VPS auf v0.2.10 bringen:** vorher einen Snapshot ziehen, dann APT → „Alle aktualisieren“. Prüfen: `godeb version`, Dienste-Seite, Protokoll, Plugins abschalten, automatische Updates.
2. **Mailserver auf dem VPS:**
   - Vorher: A-Eintrag `mail.<domain>`, Reverse-DNS, Freigabe für ausgehend 25/465 bei Hetzner beantragen.
   - Dann: Domain und Postfach anlegen, mit Thunderbird und Handy testen, mail-tester.com.
3. **Domains-Plugin** mit echter Zone und echtem Hetzner-Token prüfen.
4. **Release v0.2.8** – Tag und Vorabversion gesetzt 28.10. (CI baut die `.deb`-Pakete).
5. **Heim-VM** per Beitrittscode anbinden, Enshrouded installieren, Portweiterleitung am VPS einrichten, im Spiel testen.
6. **Tunnel-App am Handy:** Auto-Disconnect im Heim-WLAN und Wiederverbinden im Mobilnetz.

## C. Andere Repos (nicht in diesem Repo bearbeiten)

- `godeb-tunnel-arch` und `godeb-wireguard-windows`: Vertrauensprüfung auf das Protokoll `godeb-trust-v1` umstellen, wie in der Android-App. Die alte Header-Prüfung funktioniert nicht mehr.
- Windows-Client umbenennen in „GoDeb Tunnel“ und veröffentlichen (ROADMAP Phase 5).
