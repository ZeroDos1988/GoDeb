# Aufgabenliste für Coding-Agenten (Stand 29.10.2026)

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

Alles aus der Liste vom 27.10. ist erledigt (sudo mit 2FA, Update-Hinweise, Doctor auf der Startseite, Mail-Ausbau, Mobil-Feinschliff, Webseiten, Cron, PostgreSQL, Volltunnel, arm64 im APT-Repo); dazu am 28./29.10. die Punkte aus `ROADMAP.md` „Produktionsreife“, die ohne Server gehen (Sicherheitsdurchsicht, Datenbank-Prüfung, Hub-Überwachung, stabile und Test-Suite, Update-Test in der CI, Dependabot, aussagekräftigere Logs). Einzelheiten stehen in der Git-Historie und in `ROADMAP.md`.

**Bis 1.0 gilt Feature-Freeze:** nur Fixes aus den Servertests unten, jeder als Fix-Release `0.2.x`. Offen für den Agenten:

- [ ] Update-Test in der CI auch für arm64 (QEMU).
- [ ] Fehler, die der Nutzer bei den Servertests meldet.

## B. Für den Nutzer (braucht die echten Server, nicht für den Agenten)

Reihenfolge wie in `ROADMAP.md` „Produktionsreife“, Anleitung in `DOCUMENTATION.md` unter „Produktivbetrieb (Checkliste)“ und „Notfall“.

1. **VPS vorbereiten:** Hetzner-Backups an, Snapshot, Hetzner Cloud Firewall, SSH nur mit Schlüssel.
2. **VPS auf v0.2.12 bringen:** APT → „Alle aktualisieren“. Prüfen: `godeb version`, `sudo godeb doctor` (Datenbanken OK), Dienste-Seite, Protokoll, Plugins, automatische Updates.
3. **Mailserver auf dem VPS:**
   - Vorher: A-Eintrag `mail.<domain>`, Reverse-DNS, Freigabe für ausgehend 25/465 bei Hetzner beantragen.
   - Dann: Domain und Postfach anlegen, mit Thunderbird und Handy testen, mail-tester.com (mindestens 9/10).
4. **Domains-Plugin** mit echter Zone und echtem Hetzner-Token prüfen.
5. **Heim-VM** per Beitrittscode anbinden, Enshrouded installieren, Portweiterleitung am VPS einrichten, im Spiel testen.
6. **Beide Server neu starten:** Tunnel, nftables, ufw und Dienste müssen von selbst wiederkommen.
7. **Alarme:** ntfy auf beiden Servern einschalten; einmal den Tunnel trennen und prüfen, dass die Heim-VM „Hub nicht erreichbar“ meldet.
8. **Notfall üben:** `sudo godeb 2fa reset` an einem Testkonto, einmal einen Snapshot zurückspielen.
9. **Tunnel-App am Handy:** Auto-Disconnect im Heim-WLAN und Wiederverbinden im Mobilnetz.
10. Zwei Wochen ohne Eingriff, dann **1.0**.

## C. Andere Repos (nicht in diesem Repo bearbeiten)

- `godeb-tunnel-arch` und `godeb-wireguard-windows`: Vertrauensprüfung auf das Protokoll `godeb-trust-v1` umstellen, wie in der Android-App. Die alte Header-Prüfung funktioniert nicht mehr.
- Windows-Client umbenennen in „GoDeb Tunnel“ und veröffentlichen (ROADMAP Phase 5).
