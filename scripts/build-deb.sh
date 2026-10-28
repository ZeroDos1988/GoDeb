#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
ROOT=$(pwd)
VERSION=$(scripts/version.sh)
# Build in a temporary copy so the version never lands in the tracked
# package/debian/DEBIAN/control.
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
STAGE="$WORK/godeb"
cp -a "$ROOT/package/debian" "$STAGE"
sed -i "s/^Version: .*/Version: $VERSION/" "$STAGE/DEBIAN/control"

# TARGET_ARCH selects the package architecture (amd64 or arm64). Cross-builds
# use GOARCH; native builds default to the host dpkg architecture.
host_arch=$(dpkg --print-architecture)
arch=${TARGET_ARCH:-$host_arch}
case "$arch" in
amd64 | arm64) ;;
*)
	echo "TARGET_ARCH muss amd64 oder arm64 sein (ist: $arch)" >&2
	exit 1
	;;
esac
goarch=$arch
sed -i "s/^Architecture: .*/Architecture: $arch/" "$STAGE/DEBIAN/control"

rm -rf "$STAGE/usr"
mkdir -p \
	"$STAGE/usr/bin" \
	"$STAGE/usr/share/godeb/frontend" \
	"$STAGE/usr/share/godeb/plugins" \
	"$STAGE/usr/share/godeb/scripts"
cp "$ROOT/scripts/setup-fail2ban.sh" "$STAGE/usr/share/godeb/scripts/setup-fail2ban.sh"
# Key of the APT repo; DEBIAN/conffiles ships the matching godeb.sources.
mkdir -p "$STAGE/usr/share/keyrings"
cp "$ROOT/package/apt/godeb-archive-keyring.asc" "$STAGE/usr/share/keyrings/godeb-archive-keyring.asc"
mkdir -p "$STAGE/usr/share/doc/godeb"
cat >"$STAGE/usr/share/doc/godeb/copyright" <<'EOF'
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: GoDeb
Source: https://github.com/ZeroDos1988/GoDeb

Files: *
Copyright: 2026 ZeroDos1988
License: GPL-3.0-only
 On Debian systems, the full text of the GNU General Public License
 version 3 can be found in /usr/share/common-licenses/GPL-3.
EOF

echo "Baue Frontend"
(
	cd "$ROOT/frontend"
	if [ -f package-lock.json ]; then
		npm ci
	else
		npm install
	fi
	npm run build
)

rm -rf "$STAGE/usr/share/godeb/frontend/dist"
cp -a "$ROOT/frontend/dist" "$STAGE/usr/share/godeb/frontend/dist"

echo "Kopiere Plugin-Manifeste"
found=0
for dir in "$ROOT"/plugins/*/; do
	[ -f "${dir}manifest.json" ] || continue
	name=$(basename "$dir")
	mkdir -p "$STAGE/usr/share/godeb/plugins/$name"
	cp "${dir}manifest.json" "$STAGE/usr/share/godeb/plugins/$name/manifest.json"
	# plugin.json drives the plugin manager (APT packages, systemd units).
	if [ -f "${dir}plugin.json" ]; then
		cp "${dir}plugin.json" "$STAGE/usr/share/godeb/plugins/$name/plugin.json"
	fi
	found=1
done
if [ "$found" -ne 1 ]; then
	echo "Keine Plugin-Manifeste gefunden" >&2
	exit 1
fi

echo "Baue Daemon ($arch)"
# go.mod pins the toolchain; `auto` downloads it when the local Go is older.
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export CGO_ENABLED=0
export GOOS=linux
export GOARCH="$goarch"
go build -trimpath -tags netgo -ldflags "-s -w -X main.version=$VERSION" -o "$STAGE/usr/bin/godeb" ./backend
chmod 0755 "$STAGE/usr/bin/godeb"

find "$STAGE" -type d -exec chmod 0755 {} +
find "$STAGE" -type f -exec chmod 0644 {} +
chmod 0755 "$STAGE/usr/bin/godeb" "$STAGE/DEBIAN/postinst" "$STAGE/DEBIAN/postrm" \
	"$STAGE/usr/share/godeb/scripts/setup-fail2ban.sh"

version=$VERSION
out="$ROOT/godeb_${version}_${arch}.deb"

echo "Packe .deb"
dpkg-deb --root-owner-group --build "$STAGE" "$out"

echo "Prüfe Paketinhalt"
contents=$(dpkg-deb -c "$out" | awk '{print $1 " " $6}')
for want in \
	"-rwxr-xr-x ./usr/bin/godeb" \
	"-rw-r--r-- ./lib/systemd/system/godeb.service" \
	"-rw-r--r-- ./lib/systemd/system/godeb-agent.service" \
	"-rw-r--r-- ./lib/systemd/system/godeb-portforward.service" \
	"-rw-r--r-- ./lib/systemd/system/godeb-volltunnel.service" \
	"-rw-r--r-- ./usr/share/godeb/plugins/portforward/plugin.json" \
	"-rw-r--r-- ./usr/share/godeb/frontend/dist/index.html" \
	"-rw-r--r-- ./usr/share/godeb/plugins/wireguard/manifest.json" \
	"-rw-r--r-- ./usr/share/godeb/plugins/mailgateway/manifest.json" \
	"-rwxr-xr-x ./usr/share/godeb/scripts/setup-fail2ban.sh" \
	"-rw-r--r-- ./usr/share/doc/godeb/copyright" \
	"-rw-r--r-- ./etc/logrotate.d/godeb" \
	"-rw-r--r-- ./etc/apt/sources.list.d/godeb.sources" \
	"-rw-r--r-- ./usr/share/keyrings/godeb-archive-keyring.asc" \
	"-rw-r--r-- ./etc/fail2ban/filter.d/godeb.conf" \
	"-rw-r--r-- ./etc/fail2ban/jail.d/godeb.local"; do
	if ! printf '%s\n' "$contents" | grep -qxF -- "$want"; then
		echo "Fehlt im Paket: $want" >&2
		exit 1
	fi
done
depends=$(dpkg-deb -f "$out" Depends)
for dep in wireguard-tools qrencode adduser fail2ban; do
	case ", $depends," in
	*", $dep,"*) ;;
	*)
		echo "Abhängigkeit fehlt in DEBIAN/control: $dep" >&2
		exit 1
		;;
	esac
done
recommends=$(dpkg-deb -f "$out" Recommends)
for dep in postfix opendkim opendkim-tools pflogsumm; do
	case ", $recommends," in
	*", $dep,"*) ;;
	*)
		echo "Empfehlung fehlt in DEBIAN/control: $dep" >&2
		exit 1
		;;
	esac
done
postinst=$(dpkg-deb --ctrl-tarfile "$out" | tar -xO ./postinst)
for dir in /etc/wireguard/clients /etc/godeb /etc/godeb/smtp.json /var/lib/godeb /var/log/godeb godeb-agent.service setup-fail2ban.sh; do
	if ! printf '%s\n' "$postinst" | grep -qF -- "$dir"; then
		echo "postinst legt $dir nicht an" >&2
		exit 1
	fi
done
echo "Fertig: $out"
