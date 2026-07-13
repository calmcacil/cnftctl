#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 VERSION OUTPUT-DIRECTORY" >&2
    exit 2
fi

version=$1
out=$2
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || {
    echo "version must be SemVer without a leading v: $version" >&2
    exit 2
}

src=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
[ ! -e "$out" ] || { echo "output already exists: $out" >&2; exit 1; }
mkdir -p "$out/bin" "$out/systemd" "$out/scripts" "$out/docs"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$out/.version-check" "$src/cmd/cnftctl"
actual_version=$("$out/.version-check" --version)
rm "$out/.version-check"
[ "$actual_version" = "cnftctl $version" ] || { echo "binary version mismatch: $actual_version" >&2; exit 1; }
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$out/bin/cnftctl" "$src/cmd/cnftctl"
cp "$src"/deploy/systemd/* "$out/systemd/"
cp "$src"/packaging/bundle/scripts/* "$out/scripts/"
cp "$src/packaging/bundle/install.sh" "$src/packaging/bundle/uninstall.sh" "$out/"
cp "$src/LICENSE" "$src/THIRD_PARTY_NOTICES.md" "$out/"
cp "$src/docs/operator-guide.md" "$src/docs/manual-validation.md" "$src/docs/incident-response.md" "$src/docs/support-matrix.md" "$out/docs/"
sed "s/@VERSION@/$version/" "$src/packaging/bundle/manifest" >"$out/manifest"
chmod 0755 "$out/bin/cnftctl" "$out/install.sh" "$out/uninstall.sh" "$out/scripts/"*
chmod 0644 "$out/manifest" "$out/LICENSE" "$out/THIRD_PARTY_NOTICES.md" "$out/docs/"* "$out/systemd/"*
(cd "$out" && find . -type f ! -name SHA256SUMS -print | LC_ALL=C sort | sed 's|^./||' | xargs sha256sum) >"$out/SHA256SUMS"
chmod 0644 "$out/SHA256SUMS"
"$out/scripts/verify-bundle" "$out"
