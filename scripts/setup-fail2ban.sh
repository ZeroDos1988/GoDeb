#!/bin/sh
# Configures fail2ban for the GoDeb web port and the SSH daemon.
# Run as root. The Debian package calls it from postinst.
set -eu

GODEB_LOG=/var/log/godeb/auth.log
FILTER=/etc/fail2ban/filter.d/godeb.conf
JAIL=/etc/fail2ban/jail.d/godeb.local
SSHD_JAIL=/etc/fail2ban/jail.d/godeb-sshd.local

if [ "$(id -u)" -ne 0 ]; then
	echo "setup-fail2ban.sh muss als root laufen" >&2
	exit 1
fi

if ! command -v fail2ban-client >/dev/null 2>&1; then
	if [ -n "${DPKG_MAINTSCRIPT_PACKAGE:-}" ]; then
		# apt cannot run while dpkg holds its lock; Depends: fail2ban covers this case.
		echo "fail2ban fehlt. Danach erneut ausführen: apt-get install -y fail2ban && $0" >&2
		exit 0
	fi
	DEBIAN_FRONTEND=noninteractive apt-get install -y fail2ban
fi

install -d -m 0750 "$(dirname "$GODEB_LOG")"
# fail2ban refuses to start a jail whose logpath does not exist.
[ -e "$GODEB_LOG" ] || install -m 0640 /dev/null "$GODEB_LOG"
install -d -m 0755 /etc/fail2ban/filter.d /etc/fail2ban/jail.d

cat >"$FILTER" <<'EOF'
# Managed by godeb (scripts/setup-fail2ban.sh)
[Definition]
failregex = ^.* FAILED_LOGIN ip=<HOST>.*$
ignoreregex =
EOF

# backend = auto overrides a global "backend = systemd" from defaults-debian.conf,
# which would otherwise make fail2ban ignore logpath.
cat >"$JAIL" <<'EOF'
# Managed by godeb (scripts/setup-fail2ban.sh)
[godeb]
enabled = true
port = 8080,8443
filter = godeb
logpath = /var/log/godeb/auth.log
backend = auto
maxretry = 3
findtime = 600
bantime = 3600
EOF

if [ -x /usr/sbin/sshd ]; then
	{
		echo "# Managed by godeb (scripts/setup-fail2ban.sh)"
		echo "[sshd]"
		echo "enabled = true"
		# Debian 13 logs sshd to the journal only unless rsyslog is installed.
		if [ ! -e /var/log/auth.log ]; then
			echo "backend = systemd"
		fi
	} >"$SSHD_JAIL"
else
	rm -f "$SSHD_JAIL"
	echo "openssh-server ist nicht installiert, sshd-Jail bleibt aus" >&2
fi

if ! fail2ban-client -t >/dev/null 2>&1; then
	echo "fail2ban-Konfiguration ungültig:" >&2
	fail2ban-client -t >&2 || true
	exit 1
fi

systemctl enable fail2ban.service >/dev/null 2>&1 || true
systemctl restart fail2ban.service

for _ in 1 2 3 4 5 6 7 8 9 10; do
	fail2ban-client ping >/dev/null 2>&1 && break
	sleep 1
done
fail2ban-client status
