#!/bin/sh

set -eu

if [ ! -f go.mod ]; then
    echo "go.mod not found; skipping Go checks." >&2
    exit 0
fi

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
    printf '%s\n' "$unformatted" >&2
    echo "gofmt reported unformatted files." >&2
    exit 1
fi

go test ./...
go vet ./...
