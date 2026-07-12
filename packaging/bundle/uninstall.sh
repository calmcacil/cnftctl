#!/bin/sh
set -eu

root=${CNFTCTL_INSTALL_ROOT:-}
systemctl_cmd=${CNFTCTL_SYSTEMCTL:-systemctl}
bundle=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
force=0
case $#:${1:-} in
    0:) ;;
    1:--force-inactive) force=1 ;;
    *) echo "usage: uninstall.sh [--force-inactive]" >&2; exit 2 ;;
esac
dest() { printf '%s%s\n' "$root" "$1"; }
fail() { echo "cnftctl uninstall: $*" >&2; exit 1; }

[ -n "$root" ] || [ "$(id -u)" -eq 0 ] || fail "root privileges are required"
lock=$(dest /var/lock/cnftctl-delivery.lock)
lockdir=$lock.d
mkdir -p "${lock%/*}"
locked=0
trap '[ "$locked" -eq 0 ] || rmdir "$lockdir" 2>/dev/null || true' EXIT HUP INT TERM
if command -v flock >/dev/null 2>&1; then
    exec 9>"$lock"
    flock -n 9 || fail "another install or uninstall is running"
else
    mkdir "$lockdir" 2>/dev/null || fail "another install or uninstall is running"
    locked=1
fi

binary=$(dest /usr/bin/cnftctl)
[ -x "$binary" ] || fail "installed cnftctl is required to validate transaction history"
transactions=$(dest /var/lib/cnftctl/transactions)
[ ! -e "$transactions" ] || [ -d "$transactions" ] || fail "transaction history is unsafe"
if [ -d "$transactions" ]; then
    for tx in "$transactions"/*; do
        [ -e "$tx" ] || continue
        [ -d "$tx" ] && [ ! -L "$tx" ] || fail "transaction history is unsafe"
        id=${tx##*/}
        case $id in *[!0-9a-f]*) fail "transaction history is corrupt" ;; esac
        [ "${#id}" -eq 32 ] || fail "transaction history is corrupt"
        state=$tx/state.json
        [ -f "$state" ] && [ ! -L "$state" ] || fail "transaction history is unsafe"
        phase=$("$bundle/scripts/inspect-transaction" "$tx") || fail "transaction history is corrupt or unresolved"
        case $phase in confirmed|rolled-back) ;; *) fail "transaction history is corrupt or unresolved" ;; esac
    done
fi
if [ -z "$root" ]; then
    nft list table inet hostfw >/dev/null 2>&1 && fail "managed policy inet hostfw is active; keep rollback assets installed"
else
    [ "$force" -eq 1 ] || fail "staged uninstall requires --force-inactive because live policy cannot be inspected"
fi

if [ -z "$root" ] || [ "${CNFTCTL_TEST_SYSTEMD:-0}" = 1 ]; then
    "$systemctl_cmd" disable --now cnftctl-ddns-refresh.timer cnftctl-reconcile.service cnftctl-firewall.service
    for unit in cnftctl-ddns-refresh.timer cnftctl-reconcile.service cnftctl-firewall.service; do
        "$systemctl_cmd" is-enabled --quiet "$unit" && fail "failed to disable $unit"
    done
fi
rm -f "$binary" "$(dest /usr/lib/cnftctl/cnftctl-recover)"
for name in cnftctl-rollback@.service cnftctl-rollback@.timer cnftctl-reconcile.service cnftctl-firewall.service cnftctl-ddns-refresh.service cnftctl-ddns-refresh.timer; do rm -f "$(dest /usr/lib/systemd/system/$name)"; done
rm -rf "$(dest /var/lib/cnftctl/delivery)" "$(dest /usr/share/doc/cnftctl)"
if [ -z "$root" ] || [ "${CNFTCTL_TEST_SYSTEMD:-0}" = 1 ]; then
    "$systemctl_cmd" daemon-reload
fi
echo "cnftctl delivery assets removed; configuration was preserved"
