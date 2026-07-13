#!/bin/sh

set -eu

PATH=$PATH:/usr/sbin:/sbin
export PATH

tmp=${TMPDIR:-/tmp}/cnftctl-validate-$$
trap 'rm -rf "$tmp"' EXIT INT HUP TERM

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$root/go.mod" ] || { echo "repository root not found" >&2; exit 1; }

mkdir -p "$tmp/root"
go build -o "$tmp/cnftctl" "$root/cmd/cnftctl"
"$tmp/cnftctl" init --root "$tmp/root" --wan-interface eth0 --yes >/dev/null
command -v nft >/dev/null 2>&1 || { echo "nft is required for staged validation" >&2; exit 1; }
if nft list tables >/dev/null 2>&1; then
    "$tmp/cnftctl" validate --root "$tmp/root"
elif command -v unshare >/dev/null 2>&1; then
    unshare -Urn "$tmp/cnftctl" validate --root "$tmp/root"
else
    echo "nft validation requires NET_ADMIN or unshare support" >&2
    exit 1
fi
