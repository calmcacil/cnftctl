#!/bin/sh
set -eu

repo=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/cnftctl-deb-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
arch=$(dpkg --print-architecture)
case $arch in amd64|arm64) ;; *) echo "tests require amd64 or arm64" >&2; exit 1 ;; esac
for candidate_arch in amd64 arm64; do
    first=$tmp/cnftctl_0.0.0-test_${candidate_arch}.deb
    second=$tmp/cnftctl_0.0.0-test-copy_${candidate_arch}.deb
    SOURCE_DATE_EPOCH=0 sh "$repo/scripts/build-deb.sh" 0.0.0-test "$candidate_arch" "$first" >/dev/null
    SOURCE_DATE_EPOCH=0 sh "$repo/scripts/build-deb.sh" 0.0.0-test "$candidate_arch" "$second" >/dev/null
    cmp -s "$first" "$second" || { echo "$candidate_arch Debian package build is not reproducible" >&2; exit 1; }
    sh "$repo/scripts/verify-deb.sh" "$first" 0.0.0-test "$candidate_arch" >/dev/null
    mkdir -p "$tmp/preinst-$candidate_arch/root" "$tmp/preinst-$candidate_arch/control"
    dpkg-deb -e "$first" "$tmp/preinst-$candidate_arch/control"
    printf '%s\n' 'ID=debian' 'VERSION_ID="13"' >"$tmp/preinst-$candidate_arch/os-release"
    CNFTCTL_MAINT_ROOT="$tmp/preinst-$candidate_arch/root" CNFTCTL_DPKG_ARCH="$candidate_arch" \
        CNFTCTL_OS_RELEASE="$tmp/preinst-$candidate_arch/os-release" \
        "$tmp/preinst-$candidate_arch/control/preinst" install 2>"$tmp/preinst-$candidate_arch/stderr"
    if [ "$candidate_arch" = arm64 ]; then
        grep -q 'EXPERIMENTAL.*use at your own risk' "$tmp/preinst-$candidate_arch/stderr"
    else
        [ ! -s "$tmp/preinst-$candidate_arch/stderr" ]
    fi
done
deb1=$tmp/cnftctl_0.0.0-test_${arch}.deb

[ "$(dpkg-deb -f "$deb1" Package)" = cnftctl ]
[ "$(dpkg-deb -f "$deb1" Version)" = 0.0.0~test ]
[ "$(dpkg-deb -f "$deb1" Architecture)" = "$arch" ]
[ "$(dpkg-deb -f "$deb1" Depends)" = "nftables, systemd" ]

root=$tmp/root
control=$tmp/control
mkdir -p "$root" "$control"
dpkg-deb -x "$deb1" "$root"
dpkg-deb -e "$deb1" "$control"
[ "$("$root/usr/bin/cnftctl" --version)" = "cnftctl 0.0.0-test" ]
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

case $arch in amd64) other_arch=arm64 ;; arm64) other_arch=amd64 ;; esac
expect_fail sh "$repo/scripts/build-deb.sh" 0.0.0-test riscv64 "$tmp/unsupported.deb"
expect_fail sh "$repo/scripts/verify-deb.sh" "$deb1" 0.0.0-test riscv64
expect_fail sh "$repo/scripts/verify-deb.sh" "$deb1" 0.0.0-test "$other_arch"

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

CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH="$arch" CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" install 2>"$tmp/install-warning"
if [ "$arch" = arm64 ]; then grep -q 'EXPERIMENTAL.*use at your own risk' "$tmp/install-warning"; else [ ! -s "$tmp/install-warning" ]; fi
expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH="$other_arch" CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" install
expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH=unknown CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" install
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
CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH="$arch" CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" upgrade 0.0.0~old >/dev/null 2>&1
[ -z "$(CNFTCTL_SYSTEMCTL="$tmp/systemctl" CNFTCTL_SYSTEMCTL_LOG="$systemctl_log" "$control/postrm" upgrade 0.0.0~old)" ] || {
    echo "package upgrade reported a removal" >&2
    exit 1
}
[ -z "$(CNFTCTL_SYSTEMCTL="$tmp/systemctl" CNFTCTL_SYSTEMCTL_LOG="$systemctl_log" "$control/postinst" abort-remove)" ] || {
    echo "aborted removal reported an installation" >&2
    exit 1
}

pending=abcdef0123456789abcdef0123456789
mkdir "$transactions/$pending"
printf '{"id":"%s","phase":"armed"}\n' "$pending" >"$transactions/$pending/state.json"
expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH="$arch" CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" upgrade 0.0.0~old
rm -rf "${transactions:?}/$pending"

expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_SYSTEMCTL="$tmp/systemctl" CNFTCTL_SYSTEMCTL_LOG="$systemctl_log" "$control/prerm" remove
CNFTCTL_MAINT_ROOT="$root" CNFTCTL_POLICY_INACTIVE=1 CNFTCTL_SYSTEMCTL="$tmp/systemctl" \
    CNFTCTL_SYSTEMCTL_LOG="$systemctl_log" CNFTCTL_SYSTEMCTL_DISABLED=1 "$control/prerm" remove
CNFTCTL_SYSTEMCTL="$tmp/systemctl" CNFTCTL_SYSTEMCTL_LOG="$systemctl_log" "$control/postrm" remove >/dev/null
[ -f "$transactions/$terminal/state.json" ] || { echo "audit state was removed" >&2; exit 1; }

# dpkg removes package-owned helpers while preserving operator and audit state.
# A later install must validate that state before the incoming payload exists.
rm "$root/usr/lib/cnftctl/inspect-transaction"
CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH="$arch" CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" install >/dev/null 2>&1
cp "$transactions/$terminal/state.json" "$tmp/terminal-state.json"
printf '{broken}\n' >"$transactions/$terminal/state.json"
expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH="$arch" CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" install
cp "$tmp/terminal-state.json" "$transactions/$terminal/state.json"
printf '{}\n' >>"$transactions/$terminal/state.json"
expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH="$arch" CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" install
cp "$tmp/terminal-state.json" "$transactions/$terminal/state.json"
mv "$transactions/$terminal/state.json" "$transactions/$terminal/state.real"
ln -s state.real "$transactions/$terminal/state.json"
expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH="$arch" CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" install
rm "$transactions/$terminal/state.json"
mv "$transactions/$terminal/state.real" "$transactions/$terminal/state.json"
mkdir "$transactions/$pending"
printf '{"id":"%s","phase":"armed"}\n' "$pending" >"$transactions/$pending/state.json"
expect_fail env CNFTCTL_MAINT_ROOT="$root" CNFTCTL_DPKG_ARCH="$arch" CNFTCTL_OS_RELEASE="$root/etc-os-release" "$control/preinst" install

echo "Debian package lifecycle tests passed"
