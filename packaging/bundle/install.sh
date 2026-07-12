#!/bin/sh
set -eu

root=${CNFTCTL_INSTALL_ROOT:-}
systemctl_cmd=${CNFTCTL_SYSTEMCTL:-systemctl}
bundle=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
dest() { printf '%s%s\n' "$root" "$1"; }
fail() { echo "cnftctl install: $*" >&2; exit 1; }

lock=$(dest /var/lock/cnftctl-delivery.lock)
lockdir=$lock.d
stage=
backup=
committed=0
locked=0
cleanup() {
    status=$?
    if [ "$status" -ne 0 ] && [ "$committed" -eq 0 ] && [ -n "$backup" ] && [ -d "$backup" ]; then
        while IFS='|' read -r target saved; do
            rm -f "$target"
            [ "$saved" = - ] || { mkdir -p "${target%/*}"; mv "$saved" "$target"; }
        done <"$backup/index"
    fi
    [ -z "$stage" ] || rm -rf "$stage"
    [ -z "$backup" ] || rm -rf "$backup"
    [ "$locked" -eq 0 ] || rmdir "$lockdir" 2>/dev/null || true
    exit "$status"
}
trap cleanup EXIT HUP INT TERM

[ -n "$root" ] || [ "$(id -u)" -eq 0 ] || fail "root privileges are required"
"$bundle/scripts/verify-bundle" "$bundle"
[ "${CNFTCTL_BUNDLE_ARCH:-$(dpkg --print-architecture)}" = amd64 ] || fail "Debian amd64 is required"
if [ -z "$root" ]; then
    [ "$(. /etc/os-release; printf '%s' "${ID:-}")" = debian ] || fail "Debian is required"
    [ "$(. /etc/os-release; printf '%s' "${VERSION_ID:-}")" = 13 ] || fail "Debian 13 is required"
fi

mkdir -p "${lock%/*}"
if command -v flock >/dev/null 2>&1; then
    exec 9>"$lock"
    flock -n 9 || fail "another install or uninstall is running"
else
    mkdir "$lockdir" 2>/dev/null || fail "another install or uninstall is running"
    locked=1
fi

binary=$(dest /usr/bin/cnftctl)
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

parent=$(dest /var/lib/cnftctl)
mkdir -p "$parent"
stage=$parent/.install-stage-$$
backup=$parent/.install-backup-$$
mkdir -m 0700 "$stage" "$backup"
: >"$backup/index"

add() {
    source=$1 target=$(dest "$2") mode=$3
    staged=$stage/$2
    mkdir -p "${staged%/*}"
    install -m "$mode" "$source" "$staged"
    printf '%s|%s\n' "$target" "$staged" >>"$stage/index"
}
add "$bundle/bin/cnftctl" /usr/bin/cnftctl 0755
add "$bundle/scripts/cnftctl-recover" /usr/lib/cnftctl/cnftctl-recover 0755
for unit in "$bundle"/systemd/*; do add "$unit" "/usr/lib/systemd/system/${unit##*/}" 0644; done
add "$bundle/manifest" /var/lib/cnftctl/delivery/manifest 0644
add "$bundle/SHA256SUMS" /var/lib/cnftctl/delivery/SHA256SUMS 0644
add "$bundle/LICENSE" /usr/share/doc/cnftctl/LICENSE 0644
add "$bundle/THIRD_PARTY_NOTICES.md" /usr/share/doc/cnftctl/THIRD_PARTY_NOTICES.md 0644
for doc in "$bundle"/docs/*; do add "$doc" "/usr/share/doc/cnftctl/${doc##*/}" 0644; done

while IFS='|' read -r target staged; do
    mkdir -p "${target%/*}"
    saved=-
    if [ -e "$target" ] || [ -L "$target" ]; then
        [ ! -L "$target" ] || fail "refusing symlink destination: $target"
        saved=$backup/$(wc -l <"$backup/index")
        mv "$target" "$saved"
    fi
    printf '%s|%s\n' "$target" "$saved" >>"$backup/index"
    mv "$staged" "$target"
done <"$stage/index"

if [ -z "$root" ] || [ "${CNFTCTL_TEST_SYSTEMD:-0}" = 1 ]; then
    "$systemctl_cmd" daemon-reload
    "$systemctl_cmd" enable cnftctl-reconcile.service
    "$systemctl_cmd" is-enabled --quiet cnftctl-reconcile.service
fi
committed=1
echo "cnftctl installed; firewall policy and DDNS were not started"
