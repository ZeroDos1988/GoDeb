#!/bin/sh
# Update test for the CI: installs the latest GoDeb from the APT repo in a
# Debian 13 container with systemd, updates it to the freshly built package
# and checks that both services come back, the version matches and the
# databases pass quick_check (godeb doctor).
#
#   scripts/upgrade-test.sh <godeb_*.deb>
#
# Needs docker with --privileged (GitHub runner). Never run it on a server.
set -eu

if [ $# -ne 1 ] || [ ! -f "$1" ]; then
	echo "Aufruf: $0 <godeb_*.deb>" >&2
	exit 2
fi
cd "$(dirname "$0")/.."
DEB=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
NEW=$(dpkg-deb -f "$DEB" Version)
IMAGE=godeb-upgrade-test
NAME=godeb-upgrade-test

docker build -q -t "$IMAGE" - <<'EOF' >/dev/null
FROM debian:trixie
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends systemd systemd-sysv dbus ca-certificates curl sqv \
 && rm -rf /var/lib/apt/lists/*
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
EOF

docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" --privileged --cgroupns=host \
	-v /sys/fs/cgroup:/sys/fs/cgroup:rw --tmpfs /run --tmpfs /run/lock \
	"$IMAGE" >/dev/null
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT

ctr() { docker exec "$NAME" "$@"; }

# Wait for systemd.
i=0
until ctr systemctl is-system-running 2>/dev/null | grep -Eq 'running|degraded'; do
	i=$((i + 1))
	[ "$i" -lt 60 ] || { echo "systemd startet nicht" >&2; exit 1; }
	sleep 1
done

active() {
	i=0
	until ctr systemctl is-active -q godeb-agent.service && ctr systemctl is-active -q godeb.service &&
		ctr curl -fsS http://127.0.0.1:8080/api/health >/dev/null 2>&1; do
		i=$((i + 1))
		if [ "$i" -ge 60 ]; then
			ctr systemctl --no-pager status godeb-agent.service godeb.service || true
			ctr journalctl --no-pager -n 80 -u godeb-agent.service -u godeb.service || true
			return 1
		fi
		sleep 1
	done
}

echo "==> Vorige Version aus dem APT-Repo"
docker cp package/apt/godeb-archive-keyring.asc "$NAME":/usr/share/keyrings/godeb-archive-keyring.asc
docker cp package/debian/etc/apt/sources.list.d/godeb.sources "$NAME":/etc/apt/sources.list.d/godeb.sources
ctr apt-get update -q >/dev/null
ctr sh -c 'DEBIAN_FRONTEND=noninteractive apt-get install -y -q --no-install-recommends godeb' >/dev/null
OLD=$(ctr dpkg-query -W -f '${Version}' godeb)
echo "    installiert: $OLD"
active || { echo "Dienste der vorigen Version laufen nicht" >&2; exit 1; }

echo "==> Update auf $NEW"
docker cp "$DEB" "$NAME":/root/godeb.deb
ctr sh -c 'DEBIAN_FRONTEND=noninteractive apt-get install -y -q --no-install-recommends -o Dpkg::Options::=--force-confold /root/godeb.deb' >/dev/null
active || { echo "Dienste starten nach dem Update nicht" >&2; exit 1; }

got=$(ctr godeb version)
[ "$got" = "$NEW" ] || { echo "godeb version zeigt $got, erwartet $NEW" >&2; exit 1; }
health=$(ctr curl -fsS http://127.0.0.1:8080/api/health)
case "$health" in
*"\"version\":\"$NEW\""*) ;;
*) echo "Weboberfläche meldet falsche Version: $health" >&2; exit 1 ;;
esac

echo "==> godeb doctor"
# Containers lack ufw, wg0 and certificates, so FAIL there is expected; the
# databases must be fine.
report=$(ctr godeb doctor 2>&1 || true)
if printf '%s\n' "$report" | grep -Eq '^FAIL +(Agent-Datenbank|Messwerte)' ||
	! printf '%s\n' "$report" | grep -Eq '^OK +Agent-Datenbank in Ordnung'; then
	printf '%s\n' "$report"
	echo "Agent-Datenbank nicht in Ordnung" >&2
	exit 1
fi
printf '%s\n' "$report" | grep -E 'Agent-Datenbank|Messwerte'

echo "Update $OLD -> $NEW in Ordnung"
