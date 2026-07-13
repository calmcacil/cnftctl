#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
    echo "usage: $0 VERSION ARCH OUTPUT.deb" >&2
    exit 2
fi

version=$1
arch=$2
out=$3
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || {
    echo "version must be SemVer without a leading v: $version" >&2
    exit 2
}
case $arch in amd64|arm64) ;; *) echo "architecture must be amd64 or arm64: $arch" >&2; exit 2 ;; esac
[ ! -e "$out" ] || { echo "output already exists: $out" >&2; exit 1; }

base=${version%%+*}
metadata=
case $version in *+*) metadata=+${version#*+} ;; esac
case $base in
    *-*) deb_version=${base%%-*}~${base#*-}$metadata ;;
    *) deb_version=$base$metadata ;;
esac
dpkg --validate-version "$deb_version" >/dev/null 2>&1 || { echo "invalid Debian version: $deb_version" >&2; exit 2; }

src=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/cnftctl-deb.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
bundle=$tmp/bundle
pkg=$tmp/pkg
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$src" log -1 --format=%ct)}
case $epoch in *[!0-9]*|'') echo "SOURCE_DATE_EPOCH must be a non-negative integer" >&2; exit 2 ;; esac

sh "$src/scripts/build-bundle.sh" "$version" "$arch" "$bundle"
mkdir -p "$pkg/DEBIAN" "$pkg/usr/bin" "$pkg/usr/lib/cnftctl" \
    "$pkg/usr/lib/systemd/system" "$pkg/usr/share/doc/cnftctl" \
    "$pkg/usr/share/lintian/overrides" "$pkg/usr/share/man/man1" \
    "$pkg/var/lib/cnftctl/delivery"

install -m 0755 "$bundle/bin/cnftctl" "$pkg/usr/bin/cnftctl"
install -m 0755 "$bundle/scripts/cnftctl-recover" "$pkg/usr/lib/cnftctl/cnftctl-recover"
install -m 0755 "$bundle/scripts/inspect-transaction" "$pkg/usr/lib/cnftctl/inspect-transaction"
install -m 0644 "$bundle"/systemd/* "$pkg/usr/lib/systemd/system/"
install -m 0644 "$bundle/manifest" "$pkg/var/lib/cnftctl/delivery/manifest"
install -m 0644 "$bundle/LICENSE" "$pkg/usr/share/doc/cnftctl/LICENSE"
install -m 0644 "$bundle/THIRD_PARTY_NOTICES.md" "$pkg/usr/share/doc/cnftctl/THIRD_PARTY_NOTICES.md"
install -m 0644 "$bundle"/docs/* "$pkg/usr/share/doc/cnftctl/"
install -m 0644 "$src/packaging/debian/copyright" "$pkg/usr/share/doc/cnftctl/copyright"
install -m 0644 "$src/packaging/debian/lintian-overrides" "$pkg/usr/share/lintian/overrides/cnftctl"

date=$(LC_ALL=C date -u -d "@$epoch" -R)
sed -e "s/@DEBIAN_VERSION@/$deb_version/g" -e "s/@UPSTREAM_VERSION@/$version/g" \
    -e "s/@DATE@/$date/" "$src/packaging/debian/changelog.in" >"$tmp/changelog.Debian"
gzip -9n <"$tmp/changelog.Debian" >"$pkg/usr/share/doc/cnftctl/changelog.gz"
gzip -9n <"$src/packaging/debian/cnftctl.1" >"$pkg/usr/share/man/man1/cnftctl.1.gz"
chmod 0644 "$pkg/usr/share/doc/cnftctl/changelog.gz" "$pkg/usr/share/man/man1/cnftctl.1.gz"

sed "s/@ARCH@/$arch/g" "$src/packaging/debian/preinst" >"$pkg/DEBIAN/preinst"
chmod 0755 "$pkg/DEBIAN/preinst"
for script in postinst prerm postrm; do install -m 0755 "$src/packaging/debian/$script" "$pkg/DEBIAN/$script"; done

hashes=$tmp/SHA256SUMS
hash_entry() { sha256sum "$1" | sed "s|  .*|  $2|" >>"$hashes"; }
: >"$hashes"
hash_entry "$pkg/usr/bin/cnftctl" bin/cnftctl
hash_entry "$pkg/usr/lib/cnftctl/cnftctl-recover" scripts/cnftctl-recover
hash_entry "$pkg/usr/lib/cnftctl/inspect-transaction" scripts/inspect-transaction
hash_entry "$pkg/var/lib/cnftctl/delivery/manifest" manifest
for unit in "$pkg"/usr/lib/systemd/system/*; do hash_entry "$unit" "systemd/${unit##*/}"; done
LC_ALL=C sort "$hashes" >"$pkg/var/lib/cnftctl/delivery/SHA256SUMS"
chmod 0644 "$pkg/var/lib/cnftctl/delivery/SHA256SUMS"

installed_size=$(du -sk "$pkg" | awk '{print $1}')
sed -e "s/@DEBIAN_VERSION@/$deb_version/g" -e "s/@INSTALLED_SIZE@/$installed_size/" -e "s/@ARCH@/$arch/g" \
    "$src/packaging/debian/control.in" >"$pkg/DEBIAN/control"
chmod 0644 "$pkg/DEBIAN/control"

find "$pkg" -type d -exec chmod 0755 {} +
find "$pkg" -exec touch -h -d "@$epoch" {} +
mkdir -p "$(dirname -- "$out")"
DPKG_DEB_COMPRESSOR_TYPE=xz DPKG_DEB_COMPRESSOR_LEVEL=9 \
    SOURCE_DATE_EPOCH=$epoch dpkg-deb --build --root-owner-group "$pkg" "$out" >/dev/null
echo "$out"
