#!/bin/sh

set -eu

tmp=${TMPDIR:-/tmp}/cnftctl-validate-$$
trap 'rm -rf "$tmp"' EXIT INT HUP TERM

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$root/go.mod" ] || { echo "repository root not found" >&2; exit 1; }

mkdir -p "$tmp"
go build -o "$tmp/cnftctl" "$root/cmd/cnftctl"
"$tmp/cnftctl" init --root "$tmp/root" --wan-interface eth0 --yes >/dev/null
command -v nft >/dev/null 2>&1 || { echo "nft is required for staged validation" >&2; exit 1; }
"$tmp/cnftctl" validate --root "$tmp/root"
