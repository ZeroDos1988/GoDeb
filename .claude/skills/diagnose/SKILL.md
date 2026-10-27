---
name: diagnose
description: Gesamtdiagnose von GoDeb – Repo, Tests, Build, Sicherheitsregeln, Doku, CI, Releases, APT-Repo und (über einen Befehlsblock für den Nutzer) die Server. Aufrufen, wenn der Nutzer „Diagnose“, „Gesamtcheck“, „Zustand prüfen“ oder /diagnose sagt.
---

# GoDeb-Diagnose

Ziel: ein vollständiges, ehrliches Bild vom Stand des Projekts und der Server, dann ein Bericht mit Befunden nach Schwere und Vorschlägen. **Erst berichten, dann nach Rückfrage reparieren** – außer bei offensichtlichen Kleinigkeiten (Tippfehler in Doku), die nennst du und erledigst sie im selben Zug.

## 1. Kontext laden

- `CLAUDE.md` (Regeln), `ROADMAP.md` Abschnitt „Aktueller Stand“ und „Entscheidungen“, `git log --oneline -20`.
- Gedächtnis-Notizen (`project-state`, `deployment-targets`) sind Hintergrund; was davon heute noch stimmt, prüfst du.

## 2. Automatische Prüfungen

```sh
scripts/diagnose.sh
```

Das Skript ändert nichts und prüft: Git-Stand und unveröffentlichte Commits seit dem letzten Tag, `gofmt`, `go vet`, `go test`, `govulncheck`, Frontend-Build (`tsc`) und `npm audit`, de/en-Übersetzungen (Parität und benutzte Schlüssel), jede API-Route in `DOCUMENTATION.md`, Plugin-Dateien, Sicherheitsregeln aus `CLAUDE.md` (kein `sh -c`, Befehle über `core.Run*`, keine ändernden GET-Routen, keine Schlüssel oder `*.db` im Repo), die Mail-Konfiguration gegen die echten Debian-13-Werkzeuge (`scripts/check-mail-config.sh`), CI auf `main`, letztes Release und ob das APT-Repo es ausliefert.

Jede `FAIL`- und `WARN`-Zeile einzeln bewerten: echter Befund oder Grenze des Skripts? Grenzen des Skripts nennst du als solche.

## 3. Prüfungen von Hand (das Skript kann sie nicht)

- **Sicherheit:** neue Routen seit dem letzten Release lesen (`git diff <tag>..main -- backend plugins`): Eingaben gegen enge Muster geprüft? Privilegiertes nur im Agenten? Step-up-Stufe in `backend/auth/stepup.go` passend (Installationen, Firewall, Peers: frischer Code)? Secrets nur über stdin, Dateien nur über Allowlists?
- **Oberfläche:** Ansichten, die seit dem letzten Release geändert wurden, im Browser bei 360 px und am Desktop ansehen. Ohne Browser auf dem Rechner: Playwright mit Chromium nach `/tmp` installieren (`npm i playwright@1` in einem Temp-Ordner, `PLAYWRIGHT_BROWSERS_PATH=/tmp/… npx playwright install chromium-headless-shell`), `frontend/dist` über `page.route` ausliefern, `/api/*` mit Beispieldaten beantworten, Screenshots ansehen, `scrollWidth - innerWidth` muss 0 sein, keine `pageerror`. Danach den Temp-Ordner löschen.
- **Doku gegen Code:** README-Funktionstabelle, `DOCUMENTATION.md` (Plugins, Sicherheitskonzept), `ROADMAP.md` (erledigt/offen stimmt mit dem Code überein?).
- **Paket:** `scripts/build-deb.sh` nur bei Verdacht (dauert, schreibt `*.deb` ins Repo-Verzeichnis – danach löschen, nie committen).

## 4. Server (VPS und Heim-VM)

Du hast keinen Zugang zu den Servern. Bitte den Nutzer, auf jedem Server Folgendes auszuführen und die Ausgabe zu schicken (Text oder Screenshot). `godeb doctor` gibt es ab v0.2.8, die Ausgabe enthält keine Geheimnisse:

```sh
sudo godeb doctor; sudo godeb summary
```

Ältere Versionen ohne `doctor`:

```sh
godeb version; systemctl --failed --no-pager
sudo journalctl -u godeb -u godeb-agent -p warning -b --no-pager | tail -40
sudo ufw status verbose; sudo wg show all latest-handshakes
sudo ss -ltnup | grep -vE '127\.0\.0\.(1|53)|::1'; apt list --upgradable 2>/dev/null | head -20
```

Bewerten: läuft alles, was laut Rolle laufen soll (Hub: wg0 mit Handshake der Clients; Heim-VM: Peer mit Handshake)? Lauscht etwas offen, das nicht soll (Web-UI nur Loopback/wg0, 8443 nur bis zur Einrichtung)? Fehler im Journal? Version gleich dem letzten Release?

Mit Mailserver zusätzlich (Domain einsetzen): `dig +short MX <domain>`, `dig +short TXT <domain>`, `dig +short -x <vps-ip>`, `sudo postconf -n | head -60`, `sudo doveconf -n | head -40`, `sudo journalctl -u postfix@- -u dovecot -p warning -b --no-pager | tail -30`. Bei Hetzner: ausgehend 25/465 freigeschaltet? (`nc -zvw5 gmail-smtp-in.l.google.com 25`).

## 5. Bericht

Kurz und auf Deutsch, Befunde zuerst:

1. **Kritisch** (Sicherheit, Datenverlust, Server down) – mit Ort (`datei:zeile`) und Vorschlag.
2. **Wichtig** (Fehler, die Nutzer treffen; Doku falsch; Release veraltet).
3. **Kleinigkeiten.**
4. **In Ordnung** – eine Zeile, was geprüft wurde und passt.
5. **Nicht geprüft** – was ohne Server oder Werkzeug offen blieb.

Danach fragen, was repariert werden soll. Reparaturen folgen den Regeln aus `CLAUDE.md` (Tests, Commit pro Änderung, pushen). Neue Ideen aus der Diagnose kommen in `ROADMAP.md`.
