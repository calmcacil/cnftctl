#!/bin/sh
set -eu

repo=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/cnftctl-deb-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
deb1=$tmp/cnftctl_0.0.0-test_amd64.deb
deb2=$tmp/cnftctl_0.0.0-test-copy_amd64.deb

SOURCE_DATE_EPOCH=0 sh "$repo/scripts/build-deb.sh" 0.0.0-test "$deb1" >/dev/null
SOURCE_DATE_EPOCH=0 sh "$repo/scripts/build-deb.sh" 0.0.0-test "$deb2" >/dev/null
cmp -s "$deb1" "$deb2" || { echo "Debian package build is not reproducible" >&2; exit 1; }
sh "$repo/scripts/verify-deb.sh" "$deb1" 0.0.0-test >/dev/null

[ "$(dpkg-deb -f "$deb1" Package)" = cnftctl ]
[ "$(dpkg-deb -f "$deb1" Version)" = 0.0.0~test ]
[ "$(dpkg-deb -f "$deb1" Architecture)" = amd64 ]
[ "$(dpkg-deb -f "$deb1" Depends)" = "nftables, systemd" ]

root=$tmp/root
control=$tmp/control
mkdir -p "$root" "$control"
dpkg-deb -x "$deb1" "$root"
dpkg-deb -e "$deb1" "$control"
if [ "$(dpkg --print-architecture)" = amd64 ]; then
    [ "$("$root/usr/bin/cnftctl" --version)" = "cnftctl 0.0.0-test" ]
fi
for path in \
    usr/bin/cnftctl \
    usr/lib/cnftctl/cnftctl-recover \
    usr/lib/cnftctl/inspect-transaction \
    usr/lib/systemd/system/cnftctl-firewall.service \
    var/lib/cnftctl/delivery/manifest \
    var/lib/cnftctl/delivery/SHA256SUMS; do
    [ -f "$root/$path" ] || { echo "missing package asset: $path" >&2; exit 1; }
done
[ "$(stat -c %a "$root/usr/bin")" = 755 ]
[ "$(stat -c %a "$root/usr/share/doc/cnftctl/changelog.gz")" = 644 ]

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

expect_fail() {
    "$@" >/dev/null 2>&1 && { echo "expected failure: $*" >&2; exit 1; }
    return 0
}

printf '%s\n' 'ID=debian' 'VERSION_ID="13"' >"$root/etc-os-release"
systemctl_log=$tmp/systemctl.log
: >"$systemctl_log"
cat >"$tmp/systemctl" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$CNFTCTL_SYSTEMCTL_LOG"
case $1 in is-enabled) [ "${CNFTCTL_SYSTEMCTL_DISABLED:-0}" = 1 ] && exit 1 ;; esac
exit 0
EOF
chmod 0755 "$tmp/systemctl"

CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH=amd64 CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" install
CNFTCTL_SYSTEMCTL="$tmp/systemctl" CNFTCTL_SYSTEMCTL_LOG="$systemctl_log" "$control/postinst" configure >/dev/null
grep -qx 'daemon-reload' "$systemctl_log"
grep -qx 'enable --now cnftctl-reconcile.service' "$systemctl_log"
if grep -q 'enable.*cnftctl-firewall.service' "$systemctl_log"; then
    echo "package installation enabled the firewall service" >&2
    exit 1
fi

transactions=$root/var/lib/cnftctl/transactions
terminal=0123456789abcdef0123456789abcdef
mkdir -p "$transactions/$terminal"
cp "$repo/packaging/testdata/transaction-confirmed-override.json" "$transactions/$terminal/state.json"
CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH=amd64 CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" upgrade 0.0.0~old

pending=abcdef0123456789abcdef0123456789
mkdir "$transactions/$pending"
printf '{"id":"%s","phase":"armed"}\n' "$pending" >"$transactions/$pending/state.json"
expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH=amd64 CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" upgrade 0.0.0~old
rm -rf "${transactions:?}/$pending"

expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_SYSTEMCTL="$tmp/systemctl" CNFTCTL_SYSTEMCTL_LOG="$systemctl_log" "$control/prerm" remove
CNFTCTL_MAINT_ROOT="$root" CNFTCTL_POLICY_INACTIVE=1 CNFTCTL_SYSTEMCTL="$tmp/systemctl" \
    CNFTCTL_SYSTEMCTL_LOG="$systemctl_log" CNFTCTL_SYSTEMCTL_DISABLED=1 "$control/prerm" remove
CNFTCTL_SYSTEMCTL="$tmp/systemctl" CNFTCTL_SYSTEMCTL_LOG="$systemctl_log" "$control/postrm" remove >/dev/null
[ -f "$transactions/$terminal/state.json" ] || { echo "audit state was removed" >&2; exit 1; }

echo "Debian package lifecycle tests passed"
