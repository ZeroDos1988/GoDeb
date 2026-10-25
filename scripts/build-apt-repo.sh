#!/bin/sh
# Builds the signed APT repository for GitHub Pages from a directory of
# released packages.
#
#   scripts/build-apt-repo.sh <dir with godeb_*.deb> <output dir> <signing key>
#
# The signing key is the OpenPGP key without the primary's secret
# (~/.config/godeb-apt/ci-signing.pgp, in the CI the secret APT_SIGNING_KEY).
# Needs apt-ftparchive (apt-utils) and sq.
set -eu

if [ $# -ne 3 ]; then
	echo "Aufruf: $0 <deb-verzeichnis> <ausgabe> <signierschlüssel>" >&2
	exit 2
fi
cd "$(dirname "$0")/.."
ROOT=$(pwd)
DEBS=$(cd "$1" && pwd)
mkdir -p "$2"
OUT=$(cd "$2" && pwd)
KEY=$3

SUITE=trixie
COMPONENT=main
ARCH=amd64

set -- "$DEBS"/godeb_*_"$ARCH".deb
if [ ! -e "$1" ]; then
	echo "Keine Pakete in $DEBS" >&2
	exit 1
fi

rm -rf "$OUT/dists" "$OUT/pool"
pool="pool/$COMPONENT/g/godeb"
mkdir -p "$OUT/$pool" "$OUT/dists/$SUITE/$COMPONENT/binary-$ARCH"
for deb in "$@"; do
	if [ "$(dpkg-deb -f "$deb" Package)" != godeb ]; then
		echo "Kein GoDeb-Paket: $deb" >&2
		exit 1
	fi
	cp "$deb" "$OUT/$pool/"
done

cd "$OUT"
packages="dists/$SUITE/$COMPONENT/binary-$ARCH/Packages"
apt-ftparchive packages "pool/$COMPONENT" >"$packages"
gzip -9nk "$packages"

apt-ftparchive \
	-o APT::FTPArchive::Release::Origin=GoDeb \
	-o APT::FTPArchive::Release::Label=GoDeb \
	-o APT::FTPArchive::Release::Suite=stable \
	-o APT::FTPArchive::Release::Codename="$SUITE" \
	-o APT::FTPArchive::Release::Architectures="$ARCH" \
	-o APT::FTPArchive::Release::Components="$COMPONENT" \
	-o APT::FTPArchive::Release::Description="GoDeb for Debian 13" \
	release "dists/$SUITE" >"$OUT/Release.tmp"
mv "$OUT/Release.tmp" "dists/$SUITE/Release"

sq sign --batch --signer-file "$KEY" --cleartext --output "dists/$SUITE/InRelease" "dists/$SUITE/Release"
sq sign --batch --signer-file "$KEY" --signature-file "dists/$SUITE/Release.gpg" "dists/$SUITE/Release"

# Verify with the shipped key, the way apt on Trixie does it (sqv).
keyring="$ROOT/package/apt/godeb-archive-keyring.asc"
sqv --keyring "$keyring" "dists/$SUITE/Release.gpg" "dists/$SUITE/Release" >/dev/null
# sqv refuses to overwrite its output, so it writes into a fresh directory.
check=$(mktemp -d)
sqv --keyring "$keyring" --message --output "$check/Release" "dists/$SUITE/InRelease" >/dev/null
cmp -s "$check/Release" "dists/$SUITE/Release"
rm -rf "$check"

# Files for the manual setup; identical to the ones in the package, so dpkg
# takes over godeb.sources without a conffile prompt.
cp "$keyring" "$OUT/godeb-archive-keyring.asc"
cp "$ROOT/package/debian/etc/apt/sources.list.d/godeb.sources" "$OUT/godeb.sources"
fingerprint=$(sed -n 's/^Comment: \([0-9A-F ]\{50\}\)$/\1/p' "$keyring")
cat >"$OUT/index.html" <<EOF
<!doctype html>
<html lang="de">
<meta charset="utf-8">
<title>GoDeb APT-Repository</title>
<h1>GoDeb APT-Repository</h1>
<p>Für Debian 13 (Trixie), amd64. Mit einem Konto mit sudo (als root ohne <code>sudo</code>):</p>
<pre>sudo wget -O /usr/share/keyrings/godeb-archive-keyring.asc https://zerodos1988.github.io/GoDeb/godeb-archive-keyring.asc
sudo wget -O /etc/apt/sources.list.d/godeb.sources https://zerodos1988.github.io/GoDeb/godeb.sources
sudo apt update
sudo apt install --no-install-recommends godeb</pre>
<p>Fingerprint des Schlüssels: <code>$fingerprint</code></p>
<p>Quellcode und Anleitung: <a href="https://github.com/ZeroDos1988/GoDeb">github.com/ZeroDos1988/GoDeb</a></p>
</html>
EOF
echo "Fertig: $OUT ($# Pakete)"
