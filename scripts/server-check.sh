#!/bin/bash
# GoDeb server check: collects everything needed to verify that an installed
# GoDeb does what it should (package, users, permissions, services, sandbox,
# listeners, firewall, WireGuard, fail2ban, plugins, journal).
#
# Read-only: changes nothing on the system. Secrets (private keys, master
# keys, tokens, passwords) are never printed, only their permissions.
#
# Usage on the server:
#   sudo bash server-check.sh            # report to stdout and a file
#   sudo bash server-check.sh --no-net   # skip outbound tests (port 25, APT)
#
# The report lands in /root/godeb-check-<host>-<date>.txt (mode 0600).
set -u

NET=1
[ "${1:-}" = "--no-net" ] && NET=0

if [ "$(id -u)" -ne 0 ]; then
	echo "Bitte mit sudo ausführen: sudo bash $0" >&2
	exit 1
fi

export LC_ALL=C.UTF-8
REPORT="/root/godeb-check-$(hostname -s)-$(date +%Y%m%d-%H%M%S).txt"
umask 077
exec > >(tee "$REPORT") 2>&1

OK=0 WARN=0 FAIL=0
ok()   { OK=$((OK+1));     printf '  [OK]   %s\n' "$*"; }
warn() { WARN=$((WARN+1)); printf '  [WARN] %s\n' "$*"; }
fail() { FAIL=$((FAIL+1)); printf '  [FAIL] %s\n' "$*"; }
info() { printf '  [INFO] %s\n' "$*"; }
section() { printf '\n==== %s ====\n' "$*"; }
raw() { printf '  $ %s\n' "$*"; "$@" 2>&1 | sed 's/^/    /'; }
have() { command -v "$1" >/dev/null 2>&1; }

# check_perm PATH MODE OWNER:GROUP [required]
check_perm() {
	local p=$1 want_mode=$2 want_own=$3 req=${4:-required}
	if [ ! -e "$p" ] && [ ! -L "$p" ]; then
		if [ "$req" = required ]; then fail "$p fehlt"; else info "$p nicht vorhanden (optional)"; fi
		return
	fi
	if [ -L "$p" ]; then fail "$p ist ein Symlink"; return; fi
	local mode own
	mode=$(stat -c %a "$p"); own=$(stat -c %U:%G "$p")
	if [ "$mode" = "$want_mode" ] && [ "$own" = "$want_own" ]; then
		ok "$p $mode $own"
	else
		fail "$p ist $mode $own, erwartet $want_mode $want_own"
	fi
}

# Secret files: must not be readable by group/others.
check_secret() {
	local p=$1 want_own=$2
	[ -e "$p" ] || { info "$p nicht vorhanden"; return; }
	local mode own
	mode=$(stat -c %a "$p"); own=$(stat -c %U:%G "$p")
	if [ $((8#$mode & 8#077)) -eq 0 ] && [ "$own" = "$want_own" ]; then
		ok "$p $mode $own"
	else
		fail "$p ist $mode $own, erwartet 600 $want_own"
	fi
}

redact() {
	sed -E 's/^([[:space:]]*(PrivateKey|PresharedKey)[[:space:]]*=).*/\1 <entfernt>/I'
}

echo "GoDeb-Servercheck $(date -Is) auf $(hostname -f 2>/dev/null || hostname)"
echo "Bericht: $REPORT"

# ---------------------------------------------------------------------------
section "System"
raw sh -c '. /etc/os-release; echo "$PRETTY_NAME"'
raw uname -r
raw uptime
grep -q '^VERSION_CODENAME=trixie' /etc/os-release && ok "Debian 13 (trixie)" || warn "kein Debian 13 (trixie)"
[ -e /run/reboot-required ] && warn "Neustart ausstehend (/run/reboot-required)" || ok "kein Neustart ausstehend"
raw df -h / /var /etc
raw free -h

# ---------------------------------------------------------------------------
section "Paket"
if dpkg-query -W -f='${Status}' godeb 2>/dev/null | grep -q 'install ok installed'; then
	ok "Paket godeb installiert: $(dpkg-query -W -f='${Version}' godeb)"
else
	fail "Paket godeb nicht (vollständig) installiert: $(dpkg-query -W -f='${Status}' godeb 2>&1)"
fi
have godeb && raw godeb version
raw apt-cache policy godeb
if [ "$NET" = 1 ]; then
	info "APT-Quellen werden nicht aktualisiert (read-only); Kandidat oben stammt vom letzten apt update"
fi
changed=$(dpkg --verify godeb 2>/dev/null)
if [ -z "$changed" ]; then ok "dpkg --verify: keine veränderten Paketdateien"; else warn "dpkg --verify meldet Änderungen:"; echo "$changed" | sed 's/^/    /'; fi
for f in /usr/bin/godeb /usr/share/godeb/scripts/setup-fail2ban.sh \
	/lib/systemd/system/godeb.service /lib/systemd/system/godeb-agent.service \
	/lib/systemd/system/godeb-portforward.service /lib/systemd/system/godeb-volltunnel.service \
	/lib/systemd/system/godeb-gateway.service /etc/logrotate.d/godeb \
	/etc/apt/sources.list.d/godeb.sources /usr/share/keyrings/godeb-archive-keyring.asc \
	/etc/fail2ban/jail.d/godeb.local /etc/fail2ban/filter.d/godeb.conf; do
	[ -e "$f" ] && ok "$f vorhanden" || fail "$f fehlt"
done
for d in ufw certbot wireguard-tools qrencode fail2ban; do
	dpkg-query -W -f='${Status}' "$d" 2>/dev/null | grep -q 'install ok installed' && ok "Abhängigkeit $d" || fail "Abhängigkeit $d fehlt"
done
upg=$(apt list --upgradable 2>/dev/null | grep -v '^Listing' | wc -l)
[ "$upg" -eq 0 ] && ok "keine ausstehenden Updates" || warn "$upg Pakete aktualisierbar"
apt list --upgradable 2>/dev/null | grep -v '^Listing' | head -20 | sed 's/^/    /'

# ---------------------------------------------------------------------------
section "Benutzer und Rechte"
if getent passwd godeb >/dev/null; then
	pw=$(getent passwd godeb)
	ok "Benutzer godeb: $pw"
	case "$pw" in *nologin|*/false) ok "godeb hat keine Login-Shell" ;; *) fail "godeb hat eine Login-Shell" ;; esac
	[ "$(id -u godeb)" -lt 1000 ] && ok "godeb ist Systembenutzer" || warn "godeb hat UID >= 1000"
	groups_godeb=$(id -nG godeb)
	[ "$groups_godeb" = godeb ] && ok "godeb nur in Gruppe godeb" || warn "godeb in weiteren Gruppen: $groups_godeb"
else
	fail "Benutzer godeb fehlt"
fi
check_perm /var/lib/godeb 700 godeb:godeb
check_perm /etc/godeb 700 root:root
check_perm /etc/wireguard 700 root:root
check_perm /etc/wireguard/clients 700 root:root optional
check_perm /var/log/godeb 750 godeb:adm
check_perm /var/log/godeb/auth.log 640 godeb:adm
check_perm /usr/bin/godeb 755 root:root
check_secret /var/lib/godeb/master.key godeb:godeb
check_secret /etc/godeb/master.key root:root
check_secret /etc/godeb/smtp.json root:root
check_secret /var/lib/godeb/godeb.db godeb:godeb
check_secret /etc/godeb/godeb.db root:root
check_secret /etc/wireguard/wg0.conf root:root
# Nothing in /var/lib/godeb may belong to someone else, nothing may be a symlink.
foreign=$(find /var/lib/godeb \( ! -user godeb -o -type l \) 2>/dev/null)
[ -z "$foreign" ] && ok "/var/lib/godeb: alles gehört godeb, keine Symlinks" || { fail "/var/lib/godeb: fremde Besitzer oder Symlinks:"; echo "$foreign" | sed 's/^/    /'; }
loose=$(find /etc/godeb /etc/wireguard -type f -perm /077 2>/dev/null)
[ -z "$loose" ] && ok "/etc/godeb, /etc/wireguard: keine Datei für Gruppe/andere lesbar" || { fail "zu offene Dateien:"; echo "$loose" | sed 's/^/    /'; }
info "Inhalt /etc/godeb und /var/lib/godeb (nur Namen und Rechte):"
ls -la /etc/godeb /var/lib/godeb 2>&1 | sed 's/^/    /'
SOCK=/run/godeb-agent/agent.sock
if [ -S "$SOCK" ]; then
	m=$(stat -c '%a %U:%G' "$SOCK")
	[ "$m" = "660 root:godeb" ] && ok "Agent-Socket $m" || fail "Agent-Socket ist $m, erwartet 660 root:godeb"
else
	fail "Agent-Socket $SOCK fehlt"
fi

# ---------------------------------------------------------------------------
section "Dienste"
for u in godeb-agent.service godeb.service; do
	act=$(systemctl is-active "$u"); en=$(systemctl is-enabled "$u" 2>/dev/null)
	[ "$act" = active ] && ok "$u läuft" || fail "$u ist $act"
	[ "$en" = enabled ] && ok "$u aktiviert" || fail "$u ist $en"
	nr=$(systemctl show -p NRestarts --value "$u")
	[ "${nr:-0}" -eq 0 ] && ok "$u ohne Neustarts" || warn "$u wurde $nr× neu gestartet"
done
pid=$(systemctl show -p MainPID --value godeb.service)
if [ "${pid:-0}" -gt 0 ]; then
	usr=$(ps -o user= -p "$pid")
	[ "$usr" = godeb ] && ok "Weboberfläche läuft als godeb" || fail "Weboberfläche läuft als $usr"
	capeff=$(awk '/^CapEff/ {print $2}' /proc/"$pid"/status)
	[ "$capeff" = 0000000000000000 ] && ok "Weboberfläche ohne Capabilities" || fail "Weboberfläche hat Capabilities: $capeff"
fi
apid=$(systemctl show -p MainPID --value godeb-agent.service)
if [ "${apid:-0}" -gt 0 ]; then
	usr=$(ps -o user= -p "$apid")
	[ "$usr" = root ] && ok "Agent läuft als root" || fail "Agent läuft als $usr"
	if ss -Hltunp 2>/dev/null | grep -q "pid=$apid,"; then fail "Agent lauscht im Netz:"; ss -Hltunp | grep "pid=$apid," | sed 's/^/    /'; else ok "Agent lauscht nicht im Netz"; fi
fi
for u in portforward volltunnel gateway; do
	f=/etc/godeb/$u.nft
	if [ -e "$f" ]; then
		[ "$(systemctl is-active godeb-$u.service)" = active ] && ok "godeb-$u aktiv ($f vorhanden)" || fail "godeb-$u nicht aktiv, obwohl $f existiert"
		[ "$(systemctl is-enabled godeb-$u.service 2>/dev/null)" = enabled ] && ok "godeb-$u aktiviert" || warn "godeb-$u nicht aktiviert (fehlt nach Neustart)"
	else
		info "godeb-$u nicht eingerichtet"
	fi
done
failed=$(systemctl --failed --no-legend --plain | awk '{print $1}')
[ -z "$failed" ] && ok "keine fehlgeschlagenen Units" || { fail "fehlgeschlagene Units: $(echo $failed)"; }
info "Sandbox-Bewertung (systemd-analyze security, niedriger ist besser):"
for u in godeb.service godeb-agent.service; do
	printf '    %-22s %s\n' "$u" "$(systemd-analyze security "$u" 2>/dev/null | tail -1)"
done

# ---------------------------------------------------------------------------
section "Weboberfläche"
if have curl; then
	code=$(curl -s -o /tmp/.godeb-health.$$ -w '%{http_code}' --max-time 5 http://127.0.0.1:8080/api/health)
	case "$code" in
		200) ok "http://127.0.0.1:8080/api/health: $(cat /tmp/.godeb-health.$$)" ;;
		401|403) ok "Weboberfläche antwortet auf 127.0.0.1:8080 (HTTP $code)" ;;
		*) fail "Weboberfläche auf 127.0.0.1:8080 antwortet nicht (HTTP $code)" ;;
	esac
	rm -f /tmp/.godeb-health.$$
	hdr=$(curl -sI --max-time 5 http://127.0.0.1:8080/ 2>/dev/null)
	for h in content-security-policy x-frame-options x-content-type-options referrer-policy; do
		echo "$hdr" | grep -qi "^$h:" && ok "Header $h gesetzt" || warn "Header $h fehlt"
	done
else
	warn "curl fehlt, Weboberfläche nicht abgefragt"
fi

# ---------------------------------------------------------------------------
section "Offene Ports"
raw ss -Hltunp
WGIP=$(ip -4 -o addr show dev wg0 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -1)
SETUP_OPEN=1; grep -qE '"done"[[:space:]]*:[[:space:]]*true' /var/lib/godeb/setup.json 2>/dev/null && SETUP_OPEN=0
while read -r proto _ _ _ local _ proc; do
	addr=${local%:*}; port=${local##*:}; addr=${addr#[}; addr=${addr%]}
	case "$proc" in *'"godeb"'*) ;; *) continue ;; esac
	case "$addr" in
		127.0.0.1|::1) ok "godeb $proto $local (Loopback)" ;;
		"$WGIP") ok "godeb $proto $local (WireGuard)" ;;
		*)
			if [ "$port" = 8443 ]; then
				if [ "$SETUP_OPEN" = 1 ]; then warn "godeb $proto $local: Einrichtungs-Listener offen (Einrichtung noch nicht abgeschlossen)"
				elif [ -e /var/lib/godeb/access.json ] || [ -e /var/lib/godeb/remote.json ]; then info "godeb $proto $local: LAN- oder Fernzugang (HTTPS) laut Einstellung"
				else fail "godeb $proto $local: HTTPS offen ohne Einrichtung/LAN-/Fernzugang"; fi
			else
				fail "godeb $proto $local lauscht außerhalb von Loopback/wg0"
			fi ;;
	esac
done < <(ss -Hltunp 2>/dev/null)
public=$(ss -Hltun 2>/dev/null | awk '{print $5}' | grep -vE '^(127\.|\[::1\]|\[::ffff:127\.)' | grep -vE "^${WGIP:-none}:" | sort -u)
info "Dienste auf nicht-lokalen Adressen: $(echo $public)"

# ---------------------------------------------------------------------------
section "Firewall"
if have ufw; then
	st=$(ufw status verbose 2>&1)
	echo "$st" | sed 's/^/    /'
	echo "$st" | grep -q '^Status: active' && ok "ufw aktiv" || fail "ufw nicht aktiv"
	echo "$st" | grep -q 'Default: deny (incoming)' && ok "eingehend standardmäßig gesperrt" || fail "ufw: eingehend nicht standardmäßig gesperrt"
	echo "$st" | grep -qE '^8080(/tcp)?[[:space:]]+ALLOW IN' && fail "8080 aus dem Internet freigegeben" || ok "8080 nicht aus dem Internet freigegeben"
fi
if have nft; then
	raw nft list tables
	for t in portforward volltunnel gateway; do
		if [ -e /etc/godeb/$t.nft ]; then
			nft list table ip godeb_$t >/dev/null 2>&1 && ok "nftables-Tabelle godeb_$t geladen" || fail "nftables-Tabelle godeb_$t fehlt"
		fi
	done
fi
fwd=$(sysctl -n net.ipv4.ip_forward)
info "net.ipv4.ip_forward = $fwd"
if { [ -e /etc/godeb/portforward.nft ] || [ -e /etc/godeb/volltunnel.nft ] || [ -e /etc/godeb/gateway.nft ]; } && [ "$fwd" != 1 ]; then
	fail "Weiterleitung eingerichtet, aber ip_forward = 0"
fi

# ---------------------------------------------------------------------------
section "WireGuard"
if [ -e /etc/wireguard/wg0.conf ]; then
	if grep -qE '^[[:space:]]*ListenPort' /etc/wireguard/wg0.conf && ! grep -qE '^#[[:space:]]*GoDeb-Role:[[:space:]]*peer' /etc/wireguard/wg0.conf; then
		ROLE=hub
	else
		ROLE=peer
	fi
	info "Rolle laut wg0.conf: $ROLE"
	info "wg0.conf (Schlüssel entfernt):"
	redact </etc/wireguard/wg0.conf | sed 's/^/    /'
	[ "$(systemctl is-active wg-quick@wg0)" = active ] && ok "wg-quick@wg0 läuft" || fail "wg-quick@wg0 ist $(systemctl is-active wg-quick@wg0)"
	[ "$(systemctl is-enabled wg-quick@wg0 2>/dev/null)" = enabled ] && ok "wg-quick@wg0 aktiviert" || fail "wg-quick@wg0 nicht aktiviert"
	[ -n "$WGIP" ] && ok "wg0 hat Adresse $WGIP" || fail "wg0 ohne IPv4-Adresse"
	if have wg && ip link show wg0 >/dev/null 2>&1; then
		raw wg show wg0
		now=$(date +%s)
		while read -r peer ts; do
			short=${peer:0:8}…
			if [ "$ts" = 0 ]; then warn "Peer $short: noch nie verbunden"
			elif [ $((now-ts)) -gt 300 ]; then
				if [ "$ROLE" = peer ]; then fail "Peer $short (Hub): letzter Handshake vor $(( (now-ts)/60 )) min"; else info "Peer $short: letzter Handshake vor $(( (now-ts)/60 )) min"; fi
			else ok "Peer $short: Handshake vor $((now-ts)) s"; fi
		done < <(wg show wg0 latest-handshakes)
		if [ "$ROLE" = hub ]; then
			lp=$(wg show wg0 listen-port)
			ufw status 2>/dev/null | grep -qE "^$lp/udp" && ok "WireGuard-Port $lp/udp in ufw freigegeben" || fail "WireGuard-Port $lp/udp nicht in ufw freigegeben"
		fi
	fi
	nclients=$(find /etc/wireguard/clients -name '*.conf' 2>/dev/null | wc -l)
	info "$nclients Client-Konfigurationen in /etc/wireguard/clients"
else
	info "keine /etc/wireguard/wg0.conf (Einrichtung noch nicht gelaufen?)"
fi

# ---------------------------------------------------------------------------
section "fail2ban"
if have fail2ban-client; then
	[ "$(systemctl is-active fail2ban)" = active ] && ok "fail2ban läuft" || fail "fail2ban ist $(systemctl is-active fail2ban)"
	for j in godeb sshd; do
		if fail2ban-client status "$j" >/dev/null 2>&1; then
			ok "Jail $j aktiv"
			fail2ban-client status "$j" 2>&1 | sed 's/^/    /'
		else
			fail "Jail $j nicht aktiv"
		fi
	done
fi

# ---------------------------------------------------------------------------
section "GoDeb: doctor und summary"
if have godeb; then
	raw godeb doctor
	raw godeb summary
fi
if [ "$SETUP_OPEN" = 1 ]; then
	warn "Ersteinrichtung nicht abgeschlossen (setup.json ohne done); Link: sudo godeb setup-url"
else
	ok "Ersteinrichtung abgeschlossen"
fi

# ---------------------------------------------------------------------------
section "Zertifikate"
if have certbot; then raw certbot certificates; fi

# ---------------------------------------------------------------------------
section "Mail (falls installiert)"
if have postfix; then
	for u in postfix dovecot rspamd redis-server clamav-daemon; do
		systemctl list-unit-files "$u.service" >/dev/null 2>&1 || continue
		st=$(systemctl is-active "$u" 2>/dev/null)
		[ "$st" = active ] && ok "$u läuft" || warn "$u ist $st"
	done
	pc=$(postfix check 2>&1); [ -z "$pc" ] && ok "postfix check ohne Meldungen" || { warn "postfix check:"; echo "$pc" | sed 's/^/    /'; }
	if have doveconf; then doveconf -n >/dev/null 2>&1 && ok "doveconf -n ohne Fehler" || fail "doveconf meldet Fehler: $(doveconf -n 2>&1 >/dev/null | head -3)"; fi
	raw sh -c 'postconf -n | grep -vE "(password|secret)" | head -60'
	MAILDOM=$(postconf -h mydomain 2>/dev/null)
	if [ -n "$MAILDOM" ] && have dig && [ "$NET" = 1 ]; then
		raw dig +short MX "$MAILDOM"
		raw dig +short TXT "$MAILDOM"
		raw dig +short TXT "_dmarc.$MAILDOM"
		pubip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<NF;i++) if($i=="src") print $(i+1)}')
		[ -n "$pubip" ] && raw dig +short -x "$pubip"
	fi
	if [ "$NET" = 1 ]; then
		if timeout 6 bash -c 'exec 3<>/dev/tcp/gmail-smtp-in.l.google.com/25' 2>/dev/null; then ok "ausgehend Port 25 offen"; else warn "ausgehend Port 25 gesperrt (Hetzner: Freischaltung beantragen)"; fi
	fi
	raw sh -c 'journalctl -u postfix@- -u dovecot -u rspamd -p warning -b --no-pager | tail -30'
else
	info "kein Postfix installiert"
fi

# ---------------------------------------------------------------------------
section "Journal (Warnungen seit Systemstart)"
n=$(journalctl -u godeb -u godeb-agent -p warning -b --no-pager -q 2>/dev/null | wc -l)
[ "$n" -eq 0 ] && ok "keine Warnungen von godeb/godeb-agent" || warn "$n Warnungen/Fehler von godeb/godeb-agent (letzte 40 unten)"
journalctl -u godeb -u godeb-agent -p warning -b --no-pager -q 2>/dev/null | tail -40 | sed 's/^/    /'
info "Letzte 25 Zeilen godeb-agent:"
journalctl -u godeb-agent -b --no-pager -q -n 25 2>/dev/null | sed 's/^/    /'
info "Fehlgeschlagene Anmeldungen (auth.log, letzte 10):"
grep FAILED_LOGIN /var/log/godeb/auth.log 2>/dev/null | tail -10 | sed 's/^/    /'

# ---------------------------------------------------------------------------
section "Ergebnis"
printf '  OK: %d   WARN: %d   FAIL: %d\n' "$OK" "$WARN" "$FAIL"
echo "  Bericht gespeichert: $REPORT"
echo "  Enthält keine Schlüssel oder Passwörter, aber IP-Adressen und Hostnamen."
