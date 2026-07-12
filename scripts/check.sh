#!/bin/sh

set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ ! -f "$root/go.mod" ] || [ ! -d "$root/.git" ]; then
    echo "scripts/check.sh must be run from a cnftctl repository checkout" >&2
    exit 1
fi
case $(pwd -P) in
    "$root"|"$root"/*) ;;
    *) echo "scripts/check.sh must be invoked from inside the repository" >&2; exit 1 ;;
esac
cd "$root"

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
    printf '%s\n' "$unformatted" >&2
    printf '%s\n' "gofmt reported unformatted files." >&2
    exit 1
fi

go test ./...
go vet ./...
