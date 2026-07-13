#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 PACKAGE.deb UPSTREAM-VERSION" >&2
    exit 2
fi
deb=$1
version=$2
[ -f "$deb" ] || { echo "package is unavailable: $deb" >&2; exit 1; }

base=${version%%+*}
metadata=
case $version in *+*) metadata=+${version#*+} ;; esac
case $base in
    *-*) deb_version=${base%%-*}~${base#*-}$metadata ;;
    *) deb_version=$base$metadata ;;
esac

[ "$(dpkg-deb -f "$deb" Package)" = cnftctl ] || { echo "wrong package name" >&2; exit 1; }
[ "$(dpkg-deb -f "$deb" Version)" = "$deb_version" ] || { echo "wrong package version" >&2; exit 1; }
[ "$(dpkg-deb -f "$deb" Architecture)" = amd64 ] || { echo "wrong package architecture" >&2; exit 1; }
[ "$(dpkg-deb -f "$deb" Depends)" = "nftables, systemd" ] || { echo "wrong package dependencies" >&2; exit 1; }

tmp=$(mktemp -d "${TMPDIR:-/tmp}/cnftctl-deb-verify.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
root=$tmp/root
control=$tmp/control
mkdir -p "$root" "$control"
dpkg-deb -x "$deb" "$root"
dpkg-deb -e "$deb" "$control"
dpkg-deb --contents "$deb" >"$tmp/contents"
awk '$2 != "root/root" { bad = 1 } END { exit bad }' "$tmp/contents" || { echo "package payload is not root-owned" >&2; exit 1; }

find "$root" -type l -print -quit | grep -q . && { echo "package contains a symlink" >&2; exit 1; }
find "$root" ! -type d ! -type f -print -quit | grep -q . && { echo "package contains a non-regular entry" >&2; exit 1; }

cat >"$tmp/expected" <<'EOF'
usr/bin/cnftctl
usr/lib/cnftctl/cnftctl-recover
usr/lib/cnftctl/inspect-transaction
usr/lib/systemd/system/cnftctl-ddns-refresh.service
usr/lib/systemd/system/cnftctl-ddns-refresh.timer
usr/lib/systemd/system/cnftctl-firewall.service
usr/lib/systemd/system/cnftctl-reconcile.service
usr/lib/systemd/system/cnftctl-rollback@.service
usr/lib/systemd/system/cnftctl-rollback@.timer
usr/share/doc/cnftctl/LICENSE
usr/share/doc/cnftctl/THIRD_PARTY_NOTICES.md
usr/share/doc/cnftctl/changelog.gz
usr/share/doc/cnftctl/copyright
usr/share/doc/cnftctl/incident-response.md
usr/share/doc/cnftctl/manual-validation.md
usr/share/doc/cnftctl/operator-guide.md
usr/share/doc/cnftctl/support-matrix.md
usr/share/lintian/overrides/cnftctl
usr/share/man/man1/cnftctl.1.gz
var/lib/cnftctl/delivery/SHA256SUMS
var/lib/cnftctl/delivery/manifest
EOF
(cd "$root" && find . -type f -print | sed 's|^./||' | LC_ALL=C sort) >"$tmp/actual"
cmp -s "$tmp/expected" "$tmp/actual" || { echo "package file inventory mismatch" >&2; diff -u "$tmp/expected" "$tmp/actual" >&2 || true; exit 1; }

[ "$(find "$control" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort)" = "$(printf '%s\n' control postinst postrm preinst prerm)" ] || {
    echo "package control inventory mismatch" >&2
    exit 1
}
for script in postinst postrm preinst prerm; do [ "$(stat -c %a "$control/$script")" = 755 ] || { echo "wrong maintainer script mode: $script" >&2; exit 1; }; done
find "$root" -type d -print | while IFS= read -r path; do
    [ "$(stat -c %a "$path")" = 755 ] || { echo "wrong package directory mode: $path" >&2; exit 1; }
done
find "$root" -type f -print | while IFS= read -r path; do
    case $path in
        "$root/usr/bin/cnftctl"|"$root/usr/lib/cnftctl/cnftctl-recover"|"$root/usr/lib/cnftctl/inspect-transaction") expected=755 ;;
        *) expected=644 ;;
    esac
    [ "$(stat -c %a "$path")" = "$expected" ] || { echo "wrong package file mode: $path" >&2; exit 1; }
done

grep -qx 'format=1' "$root/var/lib/cnftctl/delivery/manifest"
grep -qx 'product=cnftctl' "$root/var/lib/cnftctl/delivery/manifest"
grep -qx 'architecture=amd64' "$root/var/lib/cnftctl/delivery/manifest"
grep -qx 'os=debian-13' "$root/var/lib/cnftctl/delivery/manifest"
grep -qx "version=$version" "$root/var/lib/cnftctl/delivery/manifest"

while read -r expected logical; do
    case $logical in
        bin/cnftctl) installed=$root/usr/bin/cnftctl ;;
        scripts/*) installed=$root/usr/lib/cnftctl/${logical#scripts/} ;;
        systemd/*) installed=$root/usr/lib/systemd/system/${logical#systemd/} ;;
        manifest) installed=$root/var/lib/cnftctl/delivery/manifest ;;
        *) echo "unexpected installed checksum path: $logical" >&2; exit 1 ;;
    esac
    actual=$(sha256sum "$installed" | awk '{print $1}')
    [ "$actual" = "$expected" ] || { echo "installed checksum mismatch: $logical" >&2; exit 1; }
done <"$root/var/lib/cnftctl/delivery/SHA256SUMS"

if [ "$(dpkg --print-architecture)" = amd64 ]; then
    [ "$("$root/usr/bin/cnftctl" --version)" = "cnftctl $version" ] || { echo "binary version mismatch" >&2; exit 1; }
fi

echo "Debian package verified: $deb"
