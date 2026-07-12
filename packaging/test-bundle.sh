#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=${TMPDIR:-/tmp}/cnftctl-bundle-test-$$
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir -m 0700 "$tmp"
bundle=$tmp/bundle
sh "$repo/scripts/build-bundle.sh" 0.0.0-test "$bundle" >/dev/null
go build -o "$tmp/cnftctl-native" "$repo/cmd/cnftctl"

expect_fail() {
    "$@" >/dev/null 2>&1 && { echo "expected failure: $*" >&2; exit 1; }
    return 0
}

cp -R "$bundle" "$tmp/extra"
printf 'extra\n' >"$tmp/extra/extra"
expect_fail "$tmp/extra/scripts/verify-bundle" "$tmp/extra"
cp -R "$bundle" "$tmp/missing"
rm "$tmp/missing/LICENSE"
expect_fail "$tmp/missing/scripts/verify-bundle" "$tmp/missing"
cp -R "$bundle" "$tmp/symlink"
rm "$tmp/symlink/LICENSE"
ln -s THIRD_PARTY_NOTICES.md "$tmp/symlink/LICENSE"
expect_fail "$tmp/symlink/scripts/verify-bundle" "$tmp/symlink"
cp -R "$bundle" "$tmp/duplicate"
sed -n '1p' "$tmp/duplicate/SHA256SUMS" >>"$tmp/duplicate/SHA256SUMS"
expect_fail "$tmp/duplicate/scripts/verify-bundle" "$tmp/duplicate"

root=$tmp/root
mkdir -p "$root/var/lib/cnftctl/transactions" "$root/usr/bin"
cat >"$tmp/systemctl" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$CNFTCTL_SYSTEMCTL_LOG"
case ${CNFTCTL_SYSTEMCTL_FAIL:-}:$1 in
    daemon-reload:daemon-reload|enable:enable|is-enabled:is-enabled|disable:disable) exit 1 ;;
esac
case $1 in is-enabled) [ "${CNFTCTL_SYSTEMCTL_DISABLED:-0}" = 1 ] && exit 1 ;; esac
exit 0
EOF
chmod 0755 "$tmp/systemctl"
systemctl_log=$tmp/systemctl.log
: >"$systemctl_log"
CNFTCTL_INSTALL_ROOT=$root CNFTCTL_BUNDLE_ARCH=amd64 CNFTCTL_TEST_SYSTEMD=1 CNFTCTL_SYSTEMCTL=$tmp/systemctl CNFTCTL_SYSTEMCTL_LOG=$systemctl_log "$bundle/install.sh" >/dev/null
[ "$(sed -n '/^enable /p' "$systemctl_log")" = "enable cnftctl-reconcile.service" ] || { echo "installer enabled an unexpected service" >&2; exit 1; }
! grep -q 'cnftctl-firewall.service' "$systemctl_log" || { echo "installer touched firewall service" >&2; exit 1; }
[ -f "$root/var/lib/cnftctl/delivery/SHA256SUMS" ] || { echo "installed checksum missing" >&2; exit 1; }
[ ! -e "$root/var/lib/cnftctl/manifest" ] || { echo "legacy manifest location used" >&2; exit 1; }
cp "$tmp/cnftctl-native" "$root/usr/bin/cnftctl"
terminal=0123456789abcdef0123456789abcdef
mkdir "$root/var/lib/cnftctl/transactions/$terminal"
cp "$repo/packaging/testdata/transaction-confirmed-override.json" "$root/var/lib/cnftctl/transactions/$terminal/state.json"
[ "$("$bundle/scripts/inspect-transaction" "$root/var/lib/cnftctl/transactions/$terminal")" = confirmed ] || { echo "override transaction was not accepted" >&2; exit 1; }
CNFTCTL_INSTALL_ROOT=$root CNFTCTL_BUNDLE_ARCH=amd64 CNFTCTL_TEST_SYSTEMD=1 CNFTCTL_SYSTEMCTL=$tmp/systemctl CNFTCTL_SYSTEMCTL_LOG=$systemctl_log "$bundle/install.sh" >/dev/null
cp "$tmp/cnftctl-native" "$root/usr/bin/cnftctl"
rolled=abababababababababababababababab
mkdir "$root/var/lib/cnftctl/transactions/$rolled"
sed -e "s/$terminal/$rolled/" -e 's/"phase": "confirmed"/"phase": "rolled-back"/' -e 's/"confirmed": true/"confirmed": false/' -e 's/"rolled_back": false/"rolled_back": true/' "$repo/packaging/testdata/transaction-confirmed-override.json" >"$root/var/lib/cnftctl/transactions/$rolled/state.json"
[ "$("$bundle/scripts/inspect-transaction" "$root/var/lib/cnftctl/transactions/$rolled")" = rolled-back ] || { echo "rolled-back transaction was not accepted" >&2; exit 1; }
pending=abcdef0123456789abcdef0123456789
mkdir "$root/var/lib/cnftctl/transactions/$pending"
printf '{"id":"%s","phase":"armed"}\n' "$pending" >"$root/var/lib/cnftctl/transactions/$pending/state.json"
expect_fail env CNFTCTL_INSTALL_ROOT=$root CNFTCTL_BUNDLE_ARCH=amd64 "$bundle/install.sh"
rm -rf "$root/var/lib/cnftctl/transactions/$pending"
cp "$tmp/cnftctl-native" "$root/usr/bin/cnftctl"
corrupt=11111111111111111111111111111111
mkdir "$root/var/lib/cnftctl/transactions/$corrupt"
printf '{\n' >"$root/var/lib/cnftctl/transactions/$corrupt/state.json"
expect_fail env CNFTCTL_INSTALL_ROOT=$root CNFTCTL_BUNDLE_ARCH=amd64 "$bundle/install.sh"
rm -rf "$root/var/lib/cnftctl/transactions/$corrupt"
cp "$tmp/cnftctl-native" "$root/usr/bin/cnftctl"
malformed=13131313131313131313131313131313
mkdir "$root/var/lib/cnftctl/transactions/$malformed"
sed -e "s/$terminal/$malformed/" -e '/    "reason"/s/,$//' "$repo/packaging/testdata/transaction-confirmed-override.json" >"$root/var/lib/cnftctl/transactions/$malformed/state.json"
expect_fail "$bundle/scripts/inspect-transaction" "$root/var/lib/cnftctl/transactions/$malformed"
rm -rf "$root/var/lib/cnftctl/transactions/$malformed"
unknown=14141414141414141414141414141414
mkdir "$root/var/lib/cnftctl/transactions/$unknown"
sed -e "s/$terminal/$unknown/" -e '/  "ssh_override"/i\  "unknown": true,' "$repo/packaging/testdata/transaction-confirmed-override.json" >"$root/var/lib/cnftctl/transactions/$unknown/state.json"
expect_fail "$bundle/scripts/inspect-transaction" "$root/var/lib/cnftctl/transactions/$unknown"
rm -rf "$root/var/lib/cnftctl/transactions/$unknown"
duplicate_state=15151515151515151515151515151515
mkdir "$root/var/lib/cnftctl/transactions/$duplicate_state"
sed -e "s/$terminal/$duplicate_state/" -e '/  "phase"/a\  "phase": "confirmed",' "$repo/packaging/testdata/transaction-confirmed-override.json" >"$root/var/lib/cnftctl/transactions/$duplicate_state/state.json"
expect_fail "$bundle/scripts/inspect-transaction" "$root/var/lib/cnftctl/transactions/$duplicate_state"
rm -rf "$root/var/lib/cnftctl/transactions/$duplicate_state"
cp "$tmp/cnftctl-native" "$root/usr/bin/cnftctl"
trailing=12121212121212121212121212121212
mkdir "$root/var/lib/cnftctl/transactions/$trailing"
sed "s/$terminal/$trailing/" "$root/var/lib/cnftctl/transactions/$terminal/state.json" >"$root/var/lib/cnftctl/transactions/$trailing/state.json"
printf 'trailing garbage\n' >>"$root/var/lib/cnftctl/transactions/$trailing/state.json"
expect_fail env CNFTCTL_INSTALL_ROOT=$root CNFTCTL_BUNDLE_ARCH=amd64 "$bundle/install.sh"
rm -rf "$root/var/lib/cnftctl/transactions/$trailing"
cp "$tmp/cnftctl-native" "$root/usr/bin/cnftctl"
unsafe=22222222222222222222222222222222
mkdir "$root/var/lib/cnftctl/transactions/$unsafe"
ln -s /dev/null "$root/var/lib/cnftctl/transactions/$unsafe/state.json"
expect_fail env CNFTCTL_INSTALL_ROOT=$root CNFTCTL_BUNDLE_ARCH=amd64 "$bundle/install.sh"
rm -rf "$root/var/lib/cnftctl/transactions/$unsafe"
cp "$tmp/cnftctl-native" "$root/usr/bin/cnftctl"
oldsum=$(sha256sum "$root/usr/bin/cnftctl")
expect_fail env CNFTCTL_INSTALL_ROOT=$root CNFTCTL_BUNDLE_ARCH=amd64 CNFTCTL_TEST_SYSTEMD=1 CNFTCTL_SYSTEMCTL=$tmp/systemctl CNFTCTL_SYSTEMCTL_LOG=$systemctl_log CNFTCTL_SYSTEMCTL_FAIL=enable "$bundle/install.sh"
[ "$(sha256sum "$root/usr/bin/cnftctl")" = "$oldsum" ] || { echo "failed install did not roll back assets" >&2; exit 1; }
expect_fail env CNFTCTL_INSTALL_ROOT=$root "$bundle/uninstall.sh" --unknown
expect_fail env CNFTCTL_INSTALL_ROOT=$root CNFTCTL_TEST_SYSTEMD=1 CNFTCTL_SYSTEMCTL=$tmp/systemctl CNFTCTL_SYSTEMCTL_LOG=$systemctl_log CNFTCTL_SYSTEMCTL_FAIL=disable "$bundle/uninstall.sh" --force-inactive
[ -x "$root/usr/bin/cnftctl" ] || { echo "failed uninstall removed assets" >&2; exit 1; }
CNFTCTL_INSTALL_ROOT=$root CNFTCTL_TEST_SYSTEMD=1 CNFTCTL_SYSTEMCTL=$tmp/systemctl CNFTCTL_SYSTEMCTL_LOG=$systemctl_log CNFTCTL_SYSTEMCTL_DISABLED=1 "$bundle/uninstall.sh" --force-inactive >/dev/null
[ ! -e "$root/usr/bin/cnftctl" ] || { echo "binary survived uninstall" >&2; exit 1; }

recover_root=$tmp/recover
recover_id=33333333333333333333333333333333
mkdir -p "$recover_root/var/lib/cnftctl/transactions/$recover_id"
printf '{}\n' >"$recover_root/var/lib/cnftctl/transactions/$recover_id/state.json"
sed "s|/var/lib/cnftctl|$recover_root/var/lib/cnftctl|; s|exec /usr/bin/cnftctl|exec printf '%s\\n'|" "$bundle/scripts/cnftctl-recover" >"$tmp/recover-test"
chmod 0755 "$tmp/recover-test"
[ "$("$tmp/recover-test" "$recover_id")" = "$(printf 'rollback\n%s' "$recover_id")" ] || { echo "recovery helper used wrong transaction path" >&2; exit 1; }

echo "bundle lifecycle tests passed"
