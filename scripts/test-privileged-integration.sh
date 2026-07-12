#!/bin/sh
set -eu

skip() {
    printf 'SKIP: %s\n' "$1"
    exit 0
}

[ "$(id -u)" -eq 0 ] || skip "requires root"
command -v unshare >/dev/null 2>&1 || skip "unshare is unavailable"
command -v nft >/dev/null 2>&1 || skip "nft is unavailable"
command -v systemctl >/dev/null 2>&1 || skip "systemctl is unavailable"
[ -d /run/systemd/system ] || skip "systemd is not running"
unshare --net true >/dev/null 2>&1 || skip "network namespace creation is unavailable"

tmp=${TMPDIR:-/tmp}/cnftctl-integration.$$
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir -p "$tmp"
go build -o "$tmp/cnftctl" ./cmd/cnftctl
unshare --net "$tmp/cnftctl" --version
printf 'PASS: privileged namespace prerequisites and CLI execution verified\n'
