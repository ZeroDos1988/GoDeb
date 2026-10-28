#!/bin/sh
# Checks the mail configuration GoDeb renders (plugins/mail) with the real
# Debian 13 tools: postconf/postmap (Postfix 3.10), doveconf/doveadm/sievec
# (Dovecot 2.4) and rspamadm (Rspamd). Nothing is installed: the packages
# are downloaded with `apt-get download`, unpacked into a work directory and
# run in a chroot inside a user namespace (unshare -r), so no root is needed.
#
#   scripts/check-mail-config.sh [workdir]     (default /tmp/godeb-mailcheck)
#
# Needs: apt-get, dpkg-deb, unshare, a Debian 13 package list on the host.
set -eu

cd "$(dirname "$0")/.."
REPO=$(pwd)
W=${1:-/tmp/godeb-mailcheck}
R=$W/root
T=$W/t
mkdir -p "$W/debs" "$R" "$T"

if [ ! -x "$R/usr/sbin/postconf" ]; then
	echo "==> Pakete laden und entpacken"
	(cd "$W/debs" && apt-get download -q postfix dovecot-core dovecot-imapd dovecot-lmtpd dovecot-sieve rspamd \
		libexttextcat-2.0-0 libhyperscan5 liblua5.4-0 libluajit-5.1-2 libnsl2 libsodium23 libstemmer0d libtlsrpt0 libunwind8 >/dev/null)
	for f in "$W"/debs/*.deb; do dpkg-deb -x "$f" "$R"; done
	# Host libraries the binaries need (libc, OpenSSL, …).
	export LD_LIBRARY_PATH="$R/usr/lib/x86_64-linux-gnu:$R/usr/lib/dovecot:$R/usr/lib/postfix:$R/usr/lib/rspamd"
	for b in "$R/usr/bin/doveconf" "$R/usr/bin/doveadm" "$R/usr/bin/sievec" "$R/usr/sbin/postconf" "$R/usr/sbin/postmap" "$R/usr/bin/rspamadm" \
		$(find "$R/usr/lib/dovecot" "$R/usr/lib/rspamd" -name '*.so'); do
		ldd "$b" 2>/dev/null | awk '/=> \//{print $3}'
	done | sort -u | grep -v "^$R" | while read -r l; do
		mkdir -p "$R$(dirname "$l")"
		[ -e "$R$l" ] || cp -L "$l" "$R$l"
	done
	unset LD_LIBRARY_PATH
	mkdir -p "$R/lib64" "$R/dev" "$R/t" "$R/tmp" "$R/etc/dovecot" "$R/etc/rspamd/local.d" "$R/var/lib/rspamd/dkim" "$R/run" "$R/var/log"
	cp -L /lib64/ld-linux-x86-64.so.2 "$R/lib64/"
	for l in libnss_files.so.2 libnss_dns.so.2 libresolv.so.2; do
		[ -e "/lib/x86_64-linux-gnu/$l" ] && cp -L "/lib/x86_64-linux-gnu/$l" "$R/usr/lib/x86_64-linux-gnu/"
	done
	[ -e "$R/lib" ] || ln -s usr/lib "$R/lib"
	cp /etc/hosts /etc/nsswitch.conf "$R/etc/"
	cp -L /etc/resolv.conf "$R/etc/resolv.conf"
	cp /etc/passwd /etc/group "$R/etc/"
	printf 'postfix:x:120:120::/var/spool/postfix:/usr/sbin/nologin\nvmail:x:5000:5000::/var/vmail:/usr/sbin/nologin\ndovecot:x:121:121::/usr/lib/dovecot:/usr/sbin/nologin\ndovenull:x:122:122::/nonexistent:/usr/sbin/nologin\n_rspamd:x:123:123::/var/lib/rspamd:/usr/sbin/nologin\n' >>"$R/etc/passwd"
	printf 'postfix:x:120:\npostdrop:x:124:\nvmail:x:5000:\ndovecot:x:121:\ndovenull:x:122:\n_rspamd:x:123:\n' >>"$R/etc/group"
fi

run() {
	unshare -rm sh -c "mount --rbind /dev '$R/dev' && mount --bind '$T' '$R/t' && exec /usr/sbin/chroot '$R' \"\$@\"" sh "$@"
}

echo "==> Konfiguration erzeugen"
rm -rf "$T/out"
GODEB_MAIL_DUMP="$T/out" go test -count=1 -run TestDumpConfig ./plugins/mail/ >/dev/null

echo "==> Postfix"
rm -rf "$T/pf" && mkdir -p "$T/pf"
cp "$R/etc/postfix/main.cf.proto" "$T/pf/main.cf"
cp "$R/etc/postfix/master.cf.proto" "$T/pf/master.cf"
run /usr/sbin/postconf -c /t/pf -e "myhostname=debian" "compatibility_level=3.6"
while IFS= read -r l; do run /usr/sbin/postconf -c /t/pf -e "$l"; done <"$T/out/postconf-e.txt"
while IFS= read -r l; do run /usr/sbin/postconf -c /t/pf -M "$l"; done <"$T/out/postconf-M.txt"
while IFS= read -r l; do run /usr/sbin/postconf -c /t/pf -P "$l"; done <"$T/out/postconf-P.txt"
warn=$(run /usr/sbin/postconf -c /t/pf -n 2>&1 >/dev/null || true)
[ -z "$warn" ] || { echo "$warn"; exit 1; }
for m in domains mailboxes aliases senders; do run /usr/sbin/postmap -c /t/pf -s "texthash:/t/out/etc/postfix/godeb/$m" >/dev/null; done
[ "$(run /usr/sbin/postmap -c /t/pf -q @example.com texthash:/t/out/etc/postfix/godeb/aliases)" = "info@example.com" ]
echo "    main.cf/master.cf ohne Warnung, Maps lesbar"

echo "==> Dovecot"
run /usr/bin/doveconf -c /t/out/etc/dovecot/dovecot.conf -n >/dev/null
cp "$T/out/etc/dovecot/dovecot.conf" "$R/etc/dovecot/dovecot.conf"
run /usr/bin/sievec /t/out/etc/dovecot/godeb/spam.sieve
run /usr/bin/sievec /t/out/vacation.sieve
hash=$(sed -n 's/^info@example.com:\([^:]*\):.*/\1/p' "$T/out/etc/dovecot/godeb/users")
run /usr/bin/doveadm pw -t "$hash" -p 'Correct-Horse-9' >/dev/null
echo "    dovecot.conf gültig, Sieve kompiliert, Argon2id-Hash geprüft"

echo "==> Rspamd"
rm -f "$R"/etc/rspamd/local.d/*
cp "$T"/out/etc/rspamd/local.d/* "$R/etc/rspamd/local.d/"
run /usr/bin/rspamadm configtest | grep -q "syntax OK"
echo "    configtest: syntax OK"
echo "Alles in Ordnung."
