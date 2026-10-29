# GoDeb

Admin-WebUI für Debian 13 (Trixie): Go-Daemon (`backend/`) + React/Vite (`frontend/`), Plugins unter `plugins/<id>/`.
Architektur, API und Sicherheitskonzept stehen in `DOCUMENTATION.md`, offene Arbeit in `ROADMAP.md`.

## Befehle

- Backend-Tests: `go test ./...` (CI zusätzlich `go test -race`); Fuzz-Tests ausführlich: `scripts/fuzz.sh [zeit]`
- Frontend-Build (inkl. Typecheck): `cd frontend && npm run build`
- Paket: `scripts/build-deb.sh` (baut Frontend + statisches Binary, prüft `DEBIAN/control` und `postinst`)
- Gesamtdiagnose ohne Server: `scripts/diagnose.sh` (`--quick` ohne Build/Netz); der Skill `/diagnose` (`.claude/skills/diagnose`) beschreibt den ganzen Ablauf inkl. Server-Befehlsblock
- Mail-Konfiguration gegen die echten Debian-13-Werkzeuge prüfen (ohne root): `scripts/check-mail-config.sh`
- Oberfläche ohne Browser auf dem Rechner ansehen: Playwright + `chromium-headless-shell` nach `/tmp` (siehe Skill `/diagnose`), `/api` per `page.route` mit Beispieldaten
- Lokal ohne root starten: `GODEB_DB=$PWD/.dev/godeb.db GODEB_AUTH_DIR=$PWD/.dev/auth GODEB_AUTH_LOG=$PWD/.dev/auth.log GODEB_MASTER_KEY=$PWD/.dev/master.key go run ./backend`
  (fehlende Werkzeuge liefern Beispieldaten mit `mocked: true`)
- Mit Agent lokal (zwei Terminals, gleiche Variablen plus `GODEB_AGENT_SOCKET=$PWD/.dev/agent.sock GODEB_AGENT_DB=$PWD/.dev/agent.db GODEB_STATE_DIR=$PWD/.dev`): `go run ./backend agent`, dann `go run ./backend`

## Entwicklungsrechner nicht verändern

Der Entwicklungsrechner kann selbst GoDeb betreiben. Nichts unter `/etc`, `/usr`, systemd oder APT anfassen.
Pakete nie mit apt, dpkg oder sudo installieren. Fehlt ein Werkzeug: anhalten und dem Nutzer genau einen
fertigen Befehl geben, den er selbst als root ausführt. Gleiches gilt für Deploy (`dpkg -i`, `systemctl restart`).

## Sicherheitsregeln (nicht verhandelbar)

GoDeb läuft als zwei Prozesse desselben Programms: die Weboberfläche (`godeb.service`, Benutzer `godeb`, ohne Rechte) und der Agent (`godeb-agent.service`, root, nur Unix-Socket). Jede Lücke im Agenten ist root.

- Alles, was das System ändert, läuft im Agenten (`backend/agent`, `newAgentHandler`). Die Weboberfläche leitet `/api/plugins/*`, `/api/plugin-manager*` und die privilegierten Setup-Schritte weiter (`agent.Routed`). Neue privilegierte Routen gehören dorthin, nie in die Weboberfläche.
- Der Agent vertraut der Weboberfläche nicht: Er prüft 2FA für weitergeleitete Änderungen selbst (`auth.AgentGuard`), hält die 2FA-Geheimnisse und liest nichts aus `/var/lib/godeb`, wonach er handelt. Daten, nach denen der Agent handelt, liegen in `/etc/godeb` (root), über `db.AgentOpen`/`AgentGorm`; Konten und Sitzungen in `/var/lib/godeb` über `db.Open`/`Gorm`.
- Schreibt root nach `/var/lib/godeb`, dann nur über `state.WriteFile` (os.Root, kein Symlink-Ausbruch, Übergabe an `godeb`).
- Interne Agent-Routen (`/internal/…`) nur für Abfragen, die die Weboberfläche nicht selbst lesen darf. Nie eine Route, die 2FA zurücksetzt oder ohne Code etwas ändert.

- Standardmäßig nur auf Loopback und das WireGuard-Interface binden, nie Klartext-HTTP auf LAN/WAN. Ausnahmen nur per HTTPS: die Ersteinrichtung (`backend/firstrun`, Einmal-Token, schließt sich nach Abschluss) und der vom Admin gewählte LAN-Zugang (`access.json`, nur LAN-Subnetz).
- Neue `/api`-Routen sind automatisch geschützt (`auth.RequireSession`). Ausnahmen nur in `auth.protected()` und nur mit Begründung im Kommentar.
- Externe Befehle nur über `core.Run*`/`core.RunAdmin*`: kein `sh -c`, Argumente als Liste, Secrets über stdin.
- Jede Eingabe, die in einem Befehl, Pfad oder einer Konfigdatei landet, gegen eine enge Regex/Allowlist prüfen. Vorhandene Muster (`unitPattern`, `clientName`, …) wiederverwenden. Wer eine neue Konfigdatei schreibt, legt einen Fuzz-Test an (Vorbild: `plugins/websites/fuzz_test.go`).
- Dateien nur über `core.ReadAdminFile`/`WriteAdminFile` (Pfad-Allowlist) oder mit eigener Allowlist im Plugin. Secrets mit Modus `0600`.
- Sensible DB-Felder mit `vault:"secret"` (AES-256-GCM). Die Web-DB versiegelt mit `/var/lib/godeb/master.key`, die Agent-DB mit `/etc/godeb/master.key` (`db.AgentKeyPath`, auch für 2FA). Neue Secret-Spalten in der Agent-DB zusätzlich mit `db.RegisterAgentSecret` anmelden.
- Zustandsändernde Endpunkte nur mit POST/PUT, nie mit GET.
- Neue Abhängigkeiten nur mit Begründung. Danach `govulncheck ./...` bzw. `npm audit`.

## Architektur

- Backups sind Sache der Plattform (Proxmox zu Hause, Hetzner-Backups/Snapshots), nicht von GoDeb. Keine eigene Backup-Funktion bauen.
- Ersteinrichtung: Ein frisches GoDeb richtet nichts selbst ein (auch keinen WireGuard-Server). Browser-Assistent und `godeb setup` benutzen dieselben Funktionen in `backend/firstrun`.
- Das OS ist die Quelle der Wahrheit (`/etc/wireguard/wg0.conf`, `main.cf`, systemd, apt). SQLite (`/var/lib/godeb/godeb.db` für die Weboberfläche, `/etc/godeb/godeb.db` für den Agenten, GORM + `glebarez/sqlite`) hält nur Metadaten, die die UI zum Anzeigen oder Wiederanwenden braucht.
- Kein CGO: `CGO_ENABLED=0`, `-tags netgo`.
- Ein Plugin besteht aus `manifest.json` (Sidebar), `plugin.json` (APT-Pakete, systemd-Units für den Plugin-Manager), Go-Paket mit `core.Register` in `init()` plus Blank-Import in `backend/modules.go`, und optional `ui/Panel.tsx` (wird per `import.meta.glob` geladen).
- Mailserver: Plugin `mail` (Postfix 3.10, Dovecot **2.4** – neue Konfigurationssyntax, Anleitungen für 2.3 passen nicht –, Rspamd, Redis, optional ClamAV). GoDeb schreibt alle Mail-Dateien aus der Agent-Datenbank (`mailserver_*`), die ganze `dovecot.conf` inklusive. Mail-Gateway (`mailgateway`) und Mailserver schließen sich aus.
- Domains/DNS: Plugin `domains` (Manuell oder Hetzner-Cloud-API). Andere Plugins melden Einträge über `domains.RegisterNeeds` (Mail tut das); Token in der Agent-Datenbank versiegelt; nur RRSets mit Label `godeb=1` werden geändert.
- Spielserver laufen alle über das Plugin `steam` (SteamCMD). Ein neues Spiel ist eine `Game`-Beschreibung in `plugins/steam/` (plus optional ein Formular in `plugins/steam/ui/games/`), kein eigenes Plugin.
- Fehlt ein Werkzeug auf dem Host, liefert die API Beispieldaten und `mocked: true`, statt einen Fehler zu werfen.
- Zielbild Betrieb: zwei Installationen desselben Pakets. Der Hetzner-VPS (klein) ist öffentlicher Eingang und WireGuard-Hub `10.8.0.1`. Die Heim-VM hängt als Peer `10.8.0.2` daran und betreibt Enshrouded; der VPS leitet `15636–15637/udp` per DNAT/SNAT weiter. Unterschiede nur über Setup-Rolle und Plugins. Umgesetzt mit `godeb site add` (Hub), `godeb setup --join` (Heim-VM), `wireguard.DetectRole` und dem Plugin `portforward` (eigene nftables-Tabelle, nicht `before.rules`). Details: `DOCUMENTATION.md` „Zielbild: VPS und Heim-VM“, offene Punkte: `ROADMAP.md` Phase 3b.
- Gleiche Logik für CLI und HTTP: Was `godeb setup` tut, muss über dieselben Funktionen laufen wie die Web-UI, damit beides synchron bleibt.

## Konventionen

- Fehlermeldungen und UI-Texte auf Deutsch, Code-Kommentare auf Englisch.
- UI-Texte nur über i18n (`frontend/src/locales/*.json`). Neue Schlüssel mindestens in `de.json` und `en.json` (Fallback ist `en`).
- Jede Ansicht muss ab 360 px Breite bedienbar sein: keine festen Breiten, Tabellen in `DataTable` (scrollt horizontal), Formulare einspaltig unter `md`.
- Stil wie die vorhandenen Panels: `PanelFrame`, `rounded-2xl`, Glas-Optik. Kein neues UI-Framework.

## Git: zeitnah committen und pushen (feste Regel)

Ziel: GitHub (`origin`, `git@github.com:ZeroDos1988/GoDeb.git`) ist immer nah am Arbeitsstand.

- Nach jedem abgeschlossenen, getesteten Arbeitspaket sofort committen und pushen, ohne nachzufragen. Nicht erst am Ende der Sitzung sammeln.
- Vor jedem Commit: `go test ./...` und, wenn das Frontend betroffen ist, `cd frontend && npm run build`. Schlägt etwas fehl, wird nicht committet. Stattdessen den Fehler beheben oder dem Nutzer melden.
- Ein Commit pro logischer Änderung, Conventional Commits wie in der Historie (`feat(wireguard): …`, `fix(ui): …`, `build: …`).
- Gezielt stagen (`git add <pfade>`), nie `git add -A`/`.`. Vor dem Commit `git diff --cached` prüfen: keine Secrets (`master.key`, `*.db`, `.dev/`, Client-Configs, private Schlüssel), keine Build-Artefakte (`*.deb`, `dist/`).
- Gemischte Dateien (fremde und eigene Hunks in einer Datei): nachfragen, statt fremde Änderungen ungefragt mitzucommitten.
- Commit-Identität ist `ZeroDos1988 <336650567+ZeroDos1988@users.noreply.github.com>` (in `.git/config` gesetzt). Nie einen Klarnamen oder eine private Mailadresse in Commits, Dateien oder Releases.
- Gepusht wird der aktuelle Branch. Kein `--force`, kein Umschreiben gepushter Historie, keine Tags/Releases ohne Auftrag.
- Push schlägt fehl (z. B. remote ist weiter): `git pull --rebase`, Tests erneut, dann pushen. Bei Konflikten anhalten und melden.
